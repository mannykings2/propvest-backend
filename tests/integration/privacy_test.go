package integration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/dto"
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/mannykings2/propvest-backend/internal/repositories"
	"github.com/mannykings2/propvest-backend/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ═══════════════════════════════════════════════════════════════════════════
// PRIVACY / DATA-RIGHTS INTEGRATION (Phase 9.1 – 9.5)
//
// Real Postgres, because the guarantees under test are DB guarantees:
// append-only consent rows pinned to policy versions, the partial unique index
// that allows one LIVE deletion request per user, the export bundle living in
// the row (wiped on expiry), the anonymise-vs-retain split in erasure, and the
// suppression list the bulk sender must read.
// ═══════════════════════════════════════════════════════════════════════════

func privacyDeps(t *testing.T, tdb *TestDB) (services.PrivacyService, services.ConsentService) {
	t.Helper()
	audit := services.NewAuditService(repositories.NewAuditRepository(tdb.DB))
	privacy := services.NewPrivacyService(tdb.DB, nil, audit, "test-unsub-secret").
		WithKYCCipher("test-pii-secret")
	consents := services.NewConsentService(tdb.DB)
	return privacy, consents
}

// boolPtr is defined in risk_test.go (shared package helper).

// ── 9.1: consent evidence + withdrawal + re-consent on policy bump ──────────

func TestPrivacy_ConsentLifecycleAndReconsent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	tdb := SetupTestDB(t)
	defer tdb.Cleanup()
	ctx := context.Background()
	_, consents := privacyDeps(t, tdb)

	userID := seedUser(t, tdb, "privacy.consent@example.com")

	// Registration-style give: all three purposes granted.
	status, err := consents.Give(ctx, userID, dto.GiveConsentRequest{
		PrivacyPolicy: boolPtr(true),
		Terms:         boolPtr(true),
		Marketing:     boolPtr(true),
	})
	require.NoError(t, err)
	require.False(t, status.NeedsReconsent, "granted consents on the live version need no re-consent")
	require.Len(t, status.Consents, 3)
	versionBefore := status.Policies[models.ConsentPurposePrivacyPolicy]
	assert.NotEmpty(t, versionBefore, "a live policy version must exist (000034 seeds v1.0)")

	// Append-only: withdrawing marketing adds a row, never rewrites the grant.
	_, err = consents.Give(ctx, userID, dto.GiveConsentRequest{Marketing: boolPtr(false)})
	require.NoError(t, err)
	marketingRows := countRows(t, tdb,
		`SELECT COUNT(*) FROM user_consents WHERE user_id = ? AND purpose = 'marketing'`, userID)
	assert.EqualValues(t, 2, marketingRows, "9.1: withdrawal is a NEW row, not an UPDATE")
	latest := countRows(t, tdb, `
		SELECT COUNT(*) FROM user_consents
		WHERE user_id = ? AND purpose = 'marketing' AND granted = false
		  AND created_at = (SELECT MAX(created_at) FROM user_consents
		                    WHERE user_id = ? AND purpose = 'marketing')`, userID, userID)
	assert.EqualValues(t, 1, latest, "the newest marketing row is the withdrawal")

	// A policy bump obliges re-consent for the versioned purposes.
	_, err = consents.Give(ctx, userID, dto.GiveConsentRequest{}) // no purposes → error
	require.Error(t, err, "an empty give must be rejected")

	privacy, _ := privacyDeps(t, tdb)
	// Empty version auto-increments, so repeated runs against the shared test
	// DB never collide on the (policy, version) unique key.
	_, err = privacy.BumpPolicyVersion(ctx, userID, models.ConsentPurposePrivacyPolicy, "")
	require.NoError(t, err)
	status, err = consents.Status(ctx, userID)
	require.NoError(t, err)
	assert.True(t, status.NeedsReconsent, "9.1: a bumped policy version triggers re-consent")
	assert.NotEqual(t, versionBefore, status.Policies[models.ConsentPurposePrivacyPolicy],
		"the bump must publish a NEW version")

	// Re-consent at the new version clears the flag.
	_, err = consents.Give(ctx, userID, dto.GiveConsentRequest{PrivacyPolicy: boolPtr(true)})
	require.NoError(t, err)
	status, err = consents.Status(ctx, userID)
	require.NoError(t, err)
	assert.False(t, status.NeedsReconsent)
}

// ── 9.2: async export job → bearer download → rotation → expiry wipe ────────

func TestPrivacy_ExportJobLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	tdb := SetupTestDB(t)
	defer tdb.Cleanup()
	ctx := context.Background()
	privacy, _ := privacyDeps(t, tdb)

	userID := seedUser(t, tdb, "privacy.export@example.com")
	seedWallet(t, tdb, userID, 250000)

	// Request → queued job, assembly runs async (never blocks the caller).
	res, err := privacy.RequestDataExport(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, models.ExportStatusQueued, res.Job.Status)
	require.Empty(t, res.DownloadToken, "a queued job hands out no link yet")

	// Poll until the worker goroutine finalises the job.
	var token string
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		jobs, err := privacy.ListDataExports(ctx, userID)
		require.NoError(t, err)
		require.NotEmpty(t, jobs)
		if jobs[0].Status == models.ExportStatusReady {
			// Second request on a ready job returns a FRESH link.
			again, err := privacy.RequestDataExport(ctx, userID)
			require.NoError(t, err)
			token = again.DownloadToken
			break
		}
		require.Equal(t, models.ExportStatusQueued, jobs[0].Status,
			"the job must not fail: %v", jobs[0].Error)
		time.Sleep(100 * time.Millisecond)
	}
	require.NotEmpty(t, token, "export job must become ready within 10s")

	// The link returns the owner's bundle.
	bundleJSON, err := privacy.DownloadDataExport(ctx, token)
	require.NoError(t, err)
	var bundle map[string]interface{}
	require.NoError(t, json.Unmarshal(bundleJSON, &bundle))
	assert.Equal(t, userID.String(), bundle["user_id"], "bundle belongs to the requester")
	for _, category := range []string{"profile", "wallet", "wallet_transactions", "consents"} {
		assert.Contains(t, bundle, category, "bundle must carry the %s category", category)
	}

	// Random tokens are rejected (hash lookup, raw token never stored).
	_, err = privacy.DownloadDataExport(ctx, uuid.NewString())
	require.Error(t, err, "an unknown token must not resolve a bundle")
	var storedHash *string
	require.NoError(t, tdb.DB.Raw(`SELECT download_token_hash FROM data_export_jobs WHERE id = ?`,
		res.Job.ID).Scan(&storedHash).Error)
	require.NotNil(t, storedHash)
	assert.NotEqual(t, token, *storedHash, "the stored value must be the HASH, not the token")

	// Re-issue rotates: the OLD link dies the moment a new one is minted.
	res2, err := privacy.RequestDataExport(ctx, userID)
	require.NoError(t, err)
	require.NotEmpty(t, res2.DownloadToken)
	_, err = privacy.DownloadDataExport(ctx, token)
	require.Error(t, err, "the superseded link must stop working")
	_, err = privacy.DownloadDataExport(ctx, res2.DownloadToken)
	require.NoError(t, err)

	// Expiry wipes the PII bundle out of the row (retention minimalism).
	require.NoError(t, tdb.DB.Exec(`UPDATE data_export_jobs
		SET expires_at = now() - interval '1 minute' WHERE id = ?`, res.Job.ID).Error)
	n, err := privacy.ExpireExportBundles(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, 1, "the expired job must be swept")
	var bundleAfter *string
	require.NoError(t, tdb.DB.Raw(`SELECT bundle FROM data_export_jobs WHERE id = ?`,
		res.Job.ID).Scan(&bundleAfter).Error)
	assert.Nil(t, bundleAfter, "9.2: expiry must WIPE the bundle, not just 404")
	_, err = privacy.DownloadDataExport(ctx, res2.DownloadToken)
	require.Error(t, err, "an expired job serves nothing")
}

// ── 9.3: erasure — grace period, cancel, anonymise, financial rows retained ─

func TestPrivacy_DeletionGraceAnonymisesButKeepsLedger(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	tdb := SetupTestDB(t)
	defer tdb.Cleanup()
	ctx := context.Background()
	privacy, _ := privacyDeps(t, tdb)
	userRepo := repositories.NewUserRepository(tdb.DB)

	userID := seedUser(t, tdb, "privacy.delete@example.com")
	seedWallet(t, tdb, userID, 100000)
	var walletIDStr string
	require.NoError(t, tdb.DB.Raw(`SELECT id::text FROM wallets WHERE user_id = ?`, userID).
		Scan(&walletIDStr).Error)
	walletID, err := uuid.Parse(walletIDStr)
	require.NoError(t, err)
	seedRiskTxn(t, tdb, walletID, userID, "deposit", 100000, time.Now(), "")

	// Request → 30-day grace; idempotent on repeat.
	req, err := privacy.RequestAccountDeletion(ctx, userID, "moving abroad")
	require.NoError(t, err)
	assert.Equal(t, models.DeletionStatusRequested, req.Status)
	assert.WithinDuration(t, time.Now().AddDate(0, 0, models.DeletionGraceDays),
		req.ExecuteAfter, time.Minute, "9.3: a 30-day grace window")
	again, err := privacy.RequestAccountDeletion(ctx, userID, "second ask")
	require.NoError(t, err)
	assert.Equal(t, req.ID, again.ID, "a second request returns the LIVE row, not a duplicate")
	live := countRows(t, tdb,
		`SELECT COUNT(*) FROM account_deletions WHERE user_id = ? AND status = 'requested'`, userID)
	assert.EqualValues(t, 1, live, "the partial unique index allows exactly one live request")

	// Money-moving is blocked while the request is live.
	pending, err := userRepo.HasPendingDeletion(ctx, userID)
	require.NoError(t, err)
	assert.True(t, pending, "launch policy must block deposits/withdrawals during the grace")

	// The user can cancel within the grace.
	cancelled, err := privacy.CancelAccountDeletion(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, models.DeletionStatusCancelled, cancelled.Status)
	pending, err = userRepo.HasPendingDeletion(ctx, userID)
	require.NoError(t, err)
	assert.False(t, pending, "a cancelled request lifts the money-move block")

	// Re-request; the fresh request carries the full 30-day grace again.
	_, err = privacy.RequestAccountDeletion(ctx, userID, "")
	require.NoError(t, err)

	// Nothing executes early.
	n, err := privacy.ExecuteDueDeletions(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 0, n, "the sweep must not execute before execute_after")

	require.NoError(t, tdb.DB.Exec(`UPDATE account_deletions
		SET execute_after = now() - interval '1 minute'
		WHERE user_id = ? AND status = 'requested'`, userID).Error)
	n, err = privacy.ExecuteDueDeletions(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n, "the due request must execute")

	// Identity anonymised…
	var user models.User
	require.NoError(t, tdb.DB.First(&user, "id = ?", userID).Error)
	assert.Equal(t, "Deleted User", user.FullName)
	assert.Contains(t, user.Email, "@deleted.invalid", "email is replaced by a non-routable placeholder")
	assert.NotEqual(t, "privacy.delete@example.com", user.Email)

	// …but the money trail survives (statutory AML/tax retention).
	assert.EqualValues(t, 1, countRows(t, tdb,
		`SELECT COUNT(*) FROM wallet_transactions WHERE user_id = ?`, userID),
		"9.3: financial records are RETAINED after erasure")
	assert.EqualValues(t, 1, countRows(t, tdb,
		`SELECT COUNT(*) FROM wallet_transactions WHERE user_id = ? AND type = 'deposit'`, userID))

	// The execution is audited.
	assert.GreaterOrEqual(t, auditCount(t, tdb, "privacy.account_deletion_executed"), int64(1),
		"erasure execution must be audited")

	// And the block is gone once the request is executed.
	pending, err = userRepo.HasPendingDeletion(ctx, userID)
	require.NoError(t, err)
	assert.False(t, pending)
}

// ── 9.5: admin list endpoints — the joined Scan needs an explicit Table ─────
//
// Regression guard: GORM's Scan without Model/Table returns
// "unsupported data type: <nil>" — this failed live in the admin UI before
// the .Table() source was added to both queries.
func TestPrivacy_AdminListExportsAndDeletions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	tdb := SetupTestDB(t)
	defer tdb.Cleanup()
	ctx := context.Background()
	privacy, _ := privacyDeps(t, tdb)

	userID := seedUser(t, tdb, "adminlist@example.com")
	require.NoError(t, tdb.DB.Create(&models.DataExportJob{
		UserID: userID, Status: models.ExportStatusReady,
	}).Error)
	require.NoError(t, tdb.DB.Exec(`INSERT INTO account_deletions
		(id, user_id, status, execute_after) VALUES (?, ?, 'requested', now() + interval '30 days')`,
		uuid.New(), userID).Error)

	exports, total, err := privacy.AdminListExports(ctx, 1, 20)
	require.NoError(t, err, "joined export scan must not fail")
	require.Len(t, exports, 1)
	assert.Equal(t, "adminlist@example.com", exports[0].UserEmail)
	assert.EqualValues(t, 1, total)

	deletions, dtotal, err := privacy.AdminListDeletions(ctx, 1, 20)
	require.NoError(t, err, "joined deletion scan must not fail")
	require.Len(t, deletions, 1)
	assert.Equal(t, "adminlist@example.com", deletions[0].UserEmail)
	assert.EqualValues(t, 1, dtotal)
	assert.Greater(t, deletions[0].DaysLeft, 0, "the 30-day countdown is shown to the admin")
}

// ── 9.4: unsubscribe token → suppression list the bulk sender reads ─────────

func TestPrivacy_UnsubscribeSuppressesMarketing(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	tdb := SetupTestDB(t)
	defer tdb.Cleanup()
	ctx := context.Background()
	privacy, _ := privacyDeps(t, tdb)

	userID := seedUser(t, tdb, "privacy.unsub@example.com")

	// A forged token resolves nothing.
	_, err := privacy.Unsubscribe(ctx, "forged-token")
	require.Error(t, err)

	token := privacy.UnsubscribeToken(userID)
	require.NotEmpty(t, token)
	email, err := privacy.Unsubscribe(ctx, token)
	require.NoError(t, err)
	assert.Equal(t, "privacy.unsub@example.com", email, "the resolved address is returned to the page")

	// Both marketing channels are suppressed, keyed the way the bulk sender
	// reads them (`channel|address`, lowercased).
	set, err := privacy.SuppressedAddresses(ctx)
	require.NoError(t, err)
	assert.True(t, set["email|privacy.unsub@example.com"], "email must be suppressed")
	var phone string
	require.NoError(t, tdb.DB.Raw(`SELECT COALESCE(phone, '') FROM users WHERE id = ?`, userID).
		Scan(&phone).Error)
	if phone != "" {
		// The bulk sender lowercases before comparing, so accept either key
		// form — what matters is that this phone cannot receive marketing.
		assert.True(t, set["sms|"+phone] || set["sms|"+strings.ToLower(phone)],
			"the phone must be suppressed too")
	}

	// Repeating the link is idempotent (unique channel+address).
	_, err = privacy.Unsubscribe(ctx, token)
	require.NoError(t, err)
	assert.EqualValues(t, 1, countRows(t, tdb,
		`SELECT COUNT(*) FROM marketing_suppressions WHERE user_id = ?`, userID))

	// The withdrawal is consent evidence (append-only: repeating the link may
	// add another withdrawal row, but never rewrites the grant).
	assert.GreaterOrEqual(t, countRows(t, tdb, `
		SELECT COUNT(*) FROM user_consents
		WHERE user_id = ? AND purpose = 'marketing' AND granted = false`, userID), int64(1),
		"9.4: an unsubscribe records a marketing-consent withdrawal row")
}