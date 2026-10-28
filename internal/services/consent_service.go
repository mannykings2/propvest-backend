package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mannykings2/propvest-backend/internal/dto"
	"github.com/mannykings2/propvest-backend/internal/models"
)

// ═══════════════════════════════════════════════════════════════════════════
// CONSENT SERVICE (Phase 9.1 — NDPA 2023 consent rules)
// ═══════════════════════════════════════════════════════════════════════════
//
// Consent must be freely given, specific, informed, unambiguous — and
// WITHDRAWABLE. Two consequences for the data model:
//
//   1. user_consents is APPEND-ONLY evidence. A withdrawal is a new row with
//      granted=false; history is never rewritten.
//   2. Consent to the privacy policy and terms is versioned: each row pins the
//      version it was given for, so bumping a policy flags every user on an
//      older version as needing to re-consent. Marketing consent is a plain
//      opt-in/opt-out and must NOT be bundled with the processing the contract
//      actually requires (registration records it separately either way).

// DefaultPolicyVersion is used before the policy_versions table has a row for
// a policy (fresh database, or a policy added in code).
const DefaultPolicyVersion = "1.0"

// CurrentPolicyVersions returns the live {policy: version} map — the NEWEST
// version of each policy (since 000035 the table keeps the full history, so
// the iteration order must be oldest → newest for last-wins to be correct).
func CurrentPolicyVersions(ctx context.Context, db *gorm.DB) (map[string]string, error) {
	var rows []models.PolicyVersion
	if err := db.WithContext(ctx).
		Order("effective_at ASC, created_at ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("policy versions: %w", err)
	}
	out := map[string]string{}
	for _, r := range rows {
		out[r.Policy] = r.Version
	}
	// Guarantee both known policies answer even on an empty table.
	if _, ok := out[models.ConsentPurposePrivacyPolicy]; !ok {
		out[models.ConsentPurposePrivacyPolicy] = DefaultPolicyVersion
	}
	if _, ok := out[models.ConsentPurposeTerms]; !ok {
		out[models.ConsentPurposeTerms] = DefaultPolicyVersion
	}
	return out, nil
}

// RecordConsent appends one consent decision. It is designed to run INSIDE a
// caller-owned transaction (registration, POST /me/consents) so the evidence
// commits exactly when the state it describes does.
//
// version is pinned on the row for versioned purposes; ip/ua come from the
// request metadata (RequestMetadataFromContext) — the "informed + traceable"
// half of the evidence.
func RecordConsent(ctx context.Context, tx *gorm.DB, userID uuid.UUID, purpose string, granted bool, version *string, ip, ua string) error {
	row := models.UserConsent{
		UserID:        userID,
		Purpose:       purpose,
		Granted:       granted,
		PolicyVersion: version,
	}
	if ip != "" {
		row.IPAddress = &ip
	}
	if ua != "" {
		row.UserAgent = &ua
	}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("record consent %s: %w", purpose, err)
	}
	return nil
}

// ConsentService serves the user-facing consent endpoints (GET/POST /me/consents).
type ConsentService interface {
	// Status returns live policy versions, the user's latest decision per
	// purpose, and whether a policy bump now requires re-consent.
	Status(ctx context.Context, userID uuid.UUID) (*dto.ConsentStatusResponse, error)
	// Give records fresh decisions for the purposes present in the request
	// (withdrawal included — a false is a real, recorded decision).
	Give(ctx context.Context, userID uuid.UUID, in dto.GiveConsentRequest) (*dto.ConsentStatusResponse, error)
}

type consentService struct {
	db *gorm.DB
}

func NewConsentService(db *gorm.DB) ConsentService {
	return &consentService{db: db}
}

func (s *consentService) Status(ctx context.Context, userID uuid.UUID) (*dto.ConsentStatusResponse, error) {
	policies, err := CurrentPolicyVersions(ctx, s.db)
	if err != nil {
		return nil, err
	}

	var rows []models.UserConsent
	if err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("consent status: %w", err)
	}

	// Latest decision per purpose (rows are newest-first).
	latest := map[string]models.UserConsent{}
	for _, r := range rows {
		if _, seen := latest[r.Purpose]; !seen {
			latest[r.Purpose] = r
		}
	}

	resp := &dto.ConsentStatusResponse{Policies: policies}
	for _, purpose := range []string{
		models.ConsentPurposePrivacyPolicy,
		models.ConsentPurposeTerms,
		models.ConsentPurposeMarketing,
	} {
		if r, ok := latest[purpose]; ok {
			resp.Consents = append(resp.Consents, consentResponseFrom(r))
		}
	}

	// Re-consent is owed when a REQUIRED purpose has no granted row, was
	// withdrawn, or was given for an older policy version.
	needs := false
	for _, purpose := range []string{models.ConsentPurposePrivacyPolicy, models.ConsentPurposeTerms} {
		r, ok := latest[purpose]
		if !ok || !r.Granted {
			needs = true
			continue
		}
		if r.PolicyVersion == nil || *r.PolicyVersion != policies[purpose] {
			needs = true
		}
	}
	resp.NeedsReconsent = needs
	return resp, nil
}

func (s *consentService) Give(ctx context.Context, userID uuid.UUID, in dto.GiveConsentRequest) (*dto.ConsentStatusResponse, error) {
	if in.PrivacyPolicy == nil && in.Terms == nil && in.Marketing == nil {
		return nil, fmt.Errorf("consents: at least one purpose must be provided")
	}

	policies, err := CurrentPolicyVersions(ctx, s.db)
	if err != nil {
		return nil, err
	}
	md := RequestMetadataFromContext(ctx)

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		record := func(purpose string, granted bool, versioned bool) error {
			var version *string
			if versioned {
				v := policies[purpose]
				version = &v
			}
			return RecordConsent(ctx, tx, userID, purpose, granted, version, md.IPAddress, md.UserAgent)
		}
		if in.PrivacyPolicy != nil {
			if err := record(models.ConsentPurposePrivacyPolicy, *in.PrivacyPolicy, true); err != nil {
				return err
			}
		}
		if in.Terms != nil {
			if err := record(models.ConsentPurposeTerms, *in.Terms, true); err != nil {
				return err
			}
		}
		if in.Marketing != nil {
			if err := record(models.ConsentPurposeMarketing, *in.Marketing, false); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return s.Status(ctx, userID)
}

func consentResponseFrom(r models.UserConsent) dto.ConsentRecordResponse {
	return dto.ConsentRecordResponse{
		Purpose:       r.Purpose,
		Granted:       r.Granted,
		PolicyVersion: r.PolicyVersion,
		IPAddress:     r.IPAddress,
		CreatedAt:     r.CreatedAt,
	}
}

// NextPolicyVersion increments the minor of "1.2" → "1.3" (and "1" → "2") for
// the bump-without-explicit-version case.
func NextPolicyVersion(current string) string {
	parts := strings.Split(current, ".")
	if len(parts) == 1 {
		parts = append(parts, "0")
	}
	var minor int
	fmt.Sscanf(parts[len(parts)-1], "%d", &minor)
	parts[len(parts)-1] = fmt.Sprintf("%d", minor+1)
	return strings.Join(parts, ".")
}