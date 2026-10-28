package repositories

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/logger"
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/mannykings2/propvest-backend/internal/utils/piienc"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// KYCSubmissionRepository is the data-access contract for the KYC review queue.
//
// ENCRYPTION (Phase 6.1): BVN/NIN leave this package as plaintext strings in
// the model (callers need them for last-4 display and the admin detail view),
// but what reaches the database is AES-GCM ciphertext. Rows written before
// encryption (legacy 11-digit plaintext) are transparently re-encrypted on
// first read — lazy backfill, no data migration script.
type KYCSubmissionRepository interface {
	// Upsert inserts or updates the user's single submission row (UNIQUE user_id).
	Upsert(ctx context.Context, s *models.KYCSubmission) error
	FindByUserID(ctx context.Context, userID uuid.UUID) (*models.KYCSubmission, error)

	// List returns submissions with optional status filter plus a total count.
	// Joined with users so the admin queue can show name/email without N+1.
	List(ctx context.Context, status string, limit, offset int) ([]models.KYCSubmission, int64, error)

	// ApplyDecision updates status/reason/decided_at/decided_by on the user's
	// row. Pass the caller's transaction as tx to commit the decision and its
	// audit row together (Phase 6.2 — no more best-effort sync).
	ApplyDecision(ctx context.Context, userID uuid.UUID, status, reason string, decidedBy uuid.UUID, tx ...*gorm.DB) error

	// RecordScreening stores the admin's manual sanctions/PEP check
	// (source/result/notes, who and when). Pass tx to run atomically with an
	// audit row (Phase 6.3).
	RecordScreening(ctx context.Context, userID uuid.UUID, source, result, notes string, screenedBy uuid.UUID, tx ...*gorm.DB) error
}

type kycSubmissionRepository struct {
	*BaseRepository
	cipher *piienc.Cipher
}

// NewKYCSubmissionRepository constructs the repository with the PII cipher
// used to encrypt BVN/NIN at rest.
func NewKYCSubmissionRepository(db *gorm.DB, cipher *piienc.Cipher) KYCSubmissionRepository {
	return &kycSubmissionRepository{BaseRepository: NewBaseRepository(db), cipher: cipher}
}

// encryptFields swaps the model's BVN/NIN for ciphertext in place. The caller
// gets its plaintext back after the write, so last-4 display keeps working.
func (r *kycSubmissionRepository) encryptFields(s *models.KYCSubmission) (restore func(), err error) {
	plainBVN, plainNIN := s.BVN, s.NIN
	encBVN, err := r.cipher.Encrypt(plainBVN)
	if err != nil {
		return nil, fmt.Errorf("kyc: encrypt bvn: %w", err)
	}
	encNIN, err := r.cipher.Encrypt(plainNIN)
	if err != nil {
		return nil, fmt.Errorf("kyc: encrypt nin: %w", err)
	}
	s.BVN, s.NIN = encBVN, encNIN
	return func() { s.BVN, s.NIN = plainBVN, plainNIN }, nil
}

// hydrate decrypts a loaded row into plaintext model fields. A legacy
// plaintext value is re-encrypted in place (lazy backfill, best-effort: a
// failed re-encrypt is logged, never breaks the read).
func (r *kycSubmissionRepository) hydrate(s *models.KYCSubmission) {
	legacy := !piienc.IsEncrypted(s.BVN) || !piienc.IsEncrypted(s.NIN)

	plainBVN, err := r.cipher.Decrypt(s.BVN)
	if err != nil {
		logger.Error("kyc: decrypt bvn failed", "user_id", s.UserID, "error", err)
		return
	}
	plainNIN, err := r.cipher.Decrypt(s.NIN)
	if err != nil {
		logger.Error("kyc: decrypt nin failed", "user_id", s.UserID, "error", err)
		return
	}
	s.BVN, s.NIN = plainBVN, plainNIN

	if legacy {
		encBVN, err1 := r.cipher.Encrypt(plainBVN)
		encNIN, err2 := r.cipher.Encrypt(plainNIN)
		if err1 == nil && err2 == nil {
			if uerr := r.WithContext(context.Background()).
				Model(&models.KYCSubmission{}).
				Where("user_id = ?", s.UserID).
				Updates(map[string]interface{}{"bvn": encBVN, "nin": encNIN}).Error; uerr != nil {
				logger.Error("kyc: lazy re-encrypt failed", "user_id", s.UserID, "error", uerr)
			}
		}
	}
}

func (r *kycSubmissionRepository) Upsert(ctx context.Context, s *models.KYCSubmission) error {
	restore, err := r.encryptFields(s)
	if err != nil {
		return err
	}
	defer restore()

	// One row per user: on conflict with the unique user_id index, replace the
	// existing submission so a re-submit updates rather than duplicates.
	return r.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"bvn", "nin", "doc_type", "status", "submitted_at", "screening_source", "screening_result", "screening_notes", "screening_at", "screened_by", "updated_at"}),
		}).
		Create(s).Error
}

func (r *kycSubmissionRepository) FindByUserID(ctx context.Context, userID uuid.UUID) (*models.KYCSubmission, error) {
	var s models.KYCSubmission
	if err := r.WithContext(ctx).Where("user_id = ?", userID).First(&s).Error; err != nil {
		return nil, err
	}
	r.hydrate(&s)
	return &s, nil
}

func (r *kycSubmissionRepository) List(ctx context.Context, status string, limit, offset int) ([]models.KYCSubmission, int64, error) {
	q := r.WithContext(ctx).Model(&models.KYCSubmission{})
	if status != "" {
		q = q.Where("status = ?", status)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []models.KYCSubmission
	// Session(clone) so Count's mutation of the shared chain can't clobber the
	// Select/Order below (the same GORM trap Phase 4 hit with reply_count).
	if err := q.Session(&gorm.Session{}).Order("submitted_at DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	for i := range rows {
		r.hydrate(&rows[i])
	}
	return rows, total, nil
}

// pickDB returns the tx when given, otherwise the pool — lets both the plain
// and transactional paths share one implementation.
func pickDB(tx []*gorm.DB, fallback *gorm.DB) *gorm.DB {
	if len(tx) > 0 && tx[0] != nil {
		return tx[0]
	}
	return fallback
}

func (r *kycSubmissionRepository) ApplyDecision(ctx context.Context, userID uuid.UUID, status, reason string, decidedBy uuid.UUID, tx ...*gorm.DB) error {
	updates := map[string]interface{}{
		"status":     status,
		"decided_at": gorm.Expr("NOW()"),
		"decided_by": decidedBy,
	}
	if reason != "" {
		updates["reason"] = reason
	}
	return pickDB(tx, r.GetDB()).WithContext(ctx).
		Model(&models.KYCSubmission{}).
		Where("user_id = ?", userID).
		Updates(updates).Error
}

func (r *kycSubmissionRepository) RecordScreening(ctx context.Context, userID uuid.UUID, source, result, notes string, screenedBy uuid.UUID, tx ...*gorm.DB) error {
	updates := map[string]interface{}{
		"screening_source": source,
		"screening_result": result,
		"screening_at":     gorm.Expr("NOW()"),
		"screened_by":      screenedBy,
		"updated_at":       gorm.Expr("NOW()"),
	}
	if notes != "" {
		updates["screening_notes"] = notes
	}
	return pickDB(tx, r.GetDB()).WithContext(ctx).
		Model(&models.KYCSubmission{}).
		Where("user_id = ?", userID).
		Updates(updates).Error
}
