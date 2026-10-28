package dto

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
)

// Phase 6.5: full BVN/NIN may appear in exactly ONE response shape — the admin
// KYCDetailResponse. Everything else (the model itself, the user status, the
// admin queue) must serialise without the numbers ever reaching the wire.
// These tests pin that contract so a later `json:"bvn"` addition fails loudly.
const (
	testBVN = "12345678901"
	testNIN = "98765432109"
)

func containsFullPII(payload []byte) (string, bool) {
	s := string(payload)
	if strings.Contains(s, testBVN) {
		return "bvn", true
	}
	if strings.Contains(s, testNIN) {
		return "nin", true
	}
	return "", false
}

// The model is the source of truth: json:"-" on BVN/NIN means no JSON encoder,
// logger payload or admin list built from the struct can leak them.
func TestKYCSubmissionModelNeverSerialisesFullPII(t *testing.T) {
	sub := models.KYCSubmission{
		ID:       uuid.New(),
		UserID:   uuid.New(),
		BVN:      testBVN,
		NIN:      testNIN,
		DocType:  "national_id",
		Status:   models.KYCStatusPending,
		SubmittedAt: time.Now(),
	}

	out, err := json.Marshal(sub)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if which, found := containsFullPII(out); found {
		t.Errorf("models.KYCSubmission leaked the full %s into JSON: %s", which, out)
	}
	lower := strings.ToLower(string(out))
	for _, key := range []string{`"bvn"`, `"nin"`} {
		if strings.Contains(lower, key) {
			t.Errorf("models.KYCSubmission serialised the key %s (must be json:\"-\"): %s", key, out)
		}
	}
	// The full BVN/NIN stay reachable in-process for the admin detail view.
	if sub.BVN != testBVN || sub.NIN != testNIN {
		t.Errorf("model should still hold plaintext in memory; got BVN=%q NIN=%q", sub.BVN, sub.NIN)
	}
}

// The user-facing status and the admin queue only ever carry last-4.
func TestKYCStatusAndQueueExposeLast4Only(t *testing.T) {
	status := KYCStatusResponse{
		KYCStatus:  "verified",
		Status:     models.KYCStatusVerified,
		DocType:    "national_id",
		BVNLast4:   "8901",
		NINLast4:   "3210",
		SubmittedAt: time.Now(),
	}
	out, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("marshal status: %v", err)
	}
	if which, found := containsFullPII(out); found {
		t.Errorf("KYCStatusResponse leaked full %s: %s", which, out)
	}
	if !strings.Contains(string(out), "8901") {
		t.Errorf("last-4 must still be visible for the user's own status: %s", out)
	}

	queue := KYCQueueResponse{
		Submissions: []KYCQueueItem{{
			SubmissionID: uuid.New(),
			UserID:       uuid.New(),
			BVNLast4:     "8901",
			NINLast4:     "3210",
			Status:       models.KYCStatusPending,
			SubmittedAt:  time.Now(),
		}},
		Total: 1, Page: 1, Limit: 20,
	}
	out, err = json.Marshal(queue)
	if err != nil {
		t.Fatalf("marshal queue: %v", err)
	}
	if which, found := containsFullPII(out); found {
		t.Errorf("admin queue leaked full %s — the list view is last-4 only: %s", which, out)
	}
}

// The one deliberate exception, asserted so the exception stays singular.
func TestKYCDetailIsTheOnlyFullPIIResponse(t *testing.T) {
	detail := KYCDetailResponse{
		UserID: uuid.New(),
		BVN:    testBVN,
		NIN:    testNIN,
		Status: models.KYCStatusPending,
	}
	out, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("marshal detail: %v", err)
	}
	if which, found := containsFullPII(out); !found {
		t.Errorf("admin detail must carry the full %s for manual review (got %s)", which, out)
	}
}
