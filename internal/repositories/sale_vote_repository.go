package repositories

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SaleVoteRepository is the data-access contract for sale-vote governance.
type SaleVoteRepository interface {
	// CreateCampaign inserts a new open campaign. The partial unique index on
	// (property_id) WHERE status='open' makes this the concurrency-safe way to
	// guarantee one open campaign per property — a duplicate insert fails.
	CreateCampaign(ctx context.Context, c *models.SaleVoteCampaign, tx *gorm.DB) error

	// FindOpenCampaignByProperty returns the currently-open campaign, or nil
	// (gorm.ErrRecordNotFound) if none.
	FindOpenCampaignByProperty(ctx context.Context, propertyID uuid.UUID) (*models.SaleVoteCampaign, error)

	// FindCampaignByID returns a campaign by ID.
	FindCampaignByID(ctx context.Context, id uuid.UUID) (*models.SaleVoteCampaign, error)

	// FindCampaignByInvestment resolves the campaign for an investment via its
	// property, so the vote endpoint only needs the investment ID.
	FindCampaignByInvestment(ctx context.Context, investmentID uuid.UUID) (*models.SaleVoteCampaign, error)

	// CountDistinctInvestors returns the number of distinct investors in a
	// property (the elector denominator).
	CountDistinctInvestors(ctx context.Context, propertyID uuid.UUID) (int64, error)

	// HasOpenCampaign reports whether the property has an open campaign.
	HasOpenCampaign(ctx context.Context, propertyID uuid.UUID) (bool, error)

	// CreateVote inserts a vote; a duplicate (campaign_id, user_id) returns the
	// unique-violation error so the service can map it to 409.
	CreateVote(ctx context.Context, v *models.SaleVote, tx *gorm.DB) error

	// IncrementCounters atomically bumps for/against on the campaign.
	IncrementCounters(ctx context.Context, campaignID uuid.UUID, forDelta, againstDelta int, tx *gorm.DB) error

	// CloseCampaign marks a campaign passed/failed with a result and timestamp.
	CloseCampaign(ctx context.Context, campaignID uuid.UUID, result string, tx *gorm.DB) error

	// FindVoteByUserAndCampaign returns the user's vote on a campaign, or nil.
	FindVoteByUserAndCampaign(ctx context.Context, campaignID, userID uuid.UUID) (*models.SaleVote, error)

	// ListVoters returns the user IDs that already voted on a campaign (for
	// notification fan-out to non-voters on close).
	ListVoters(ctx context.Context, campaignID uuid.UUID) ([]uuid.UUID, error)
}

type saleVoteRepository struct {
	*BaseRepository
}

// NewSaleVoteRepository constructs the repository.
func NewSaleVoteRepository(db *gorm.DB) SaleVoteRepository {
	return &saleVoteRepository{BaseRepository: NewBaseRepository(db)}
}

func (r *saleVoteRepository) CreateCampaign(ctx context.Context, c *models.SaleVoteCampaign, tx *gorm.DB) error {
	db := r.WithContext(ctx)
	if tx != nil {
		db = tx.WithContext(ctx)
	}
	return db.Create(c).Error
}

func (r *saleVoteRepository) FindOpenCampaignByProperty(ctx context.Context, propertyID uuid.UUID) (*models.SaleVoteCampaign, error) {
	var c models.SaleVoteCampaign
	err := r.WithContext(ctx).
		Where("property_id = ? AND status = ?", propertyID, models.SaleVoteOpen).
		First(&c).Error
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *saleVoteRepository) FindCampaignByID(ctx context.Context, id uuid.UUID) (*models.SaleVoteCampaign, error) {
	var c models.SaleVoteCampaign
	if err := r.WithContext(ctx).Where("id = ?", id).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *saleVoteRepository) FindCampaignByInvestment(ctx context.Context, investmentID uuid.UUID) (*models.SaleVoteCampaign, error) {
	var c models.SaleVoteCampaign
	err := r.WithContext(ctx).
		Table("sale_vote_campaigns").
		Joins("JOIN investments ON investments.property_id = sale_vote_campaigns.property_id").
		Where("investments.id = ? AND sale_vote_campaigns.status = ?", investmentID, models.SaleVoteOpen).
		First(&c).Error
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *saleVoteRepository) CountDistinctInvestors(ctx context.Context, propertyID uuid.UUID) (int64, error) {
	var count int64
	err := r.WithContext(ctx).
		Model(&models.Investment{}).
		Where("property_id = ? AND status = ?", propertyID, models.InvestmentStatusActive).
		Distinct("user_id").
		Count(&count).Error
	return count, err
}

func (r *saleVoteRepository) HasOpenCampaign(ctx context.Context, propertyID uuid.UUID) (bool, error) {
	var count int64
	err := r.WithContext(ctx).
		Model(&models.SaleVoteCampaign{}).
		Where("property_id = ? AND status = ?", propertyID, models.SaleVoteOpen).
		Count(&count).Error
	return count > 0, err
}

func (r *saleVoteRepository) CreateVote(ctx context.Context, v *models.SaleVote, tx *gorm.DB) error {
	db := r.WithContext(ctx)
	if tx != nil {
		db = tx.WithContext(ctx)
	}
	return db.Create(v).Error
}

func (r *saleVoteRepository) IncrementCounters(ctx context.Context, campaignID uuid.UUID, forDelta, againstDelta int, tx *gorm.DB) error {
	db := r.WithContext(ctx)
	if tx != nil {
		db = tx.WithContext(ctx)
	}
	return db.Model(&models.SaleVoteCampaign{}).
		Where("id = ?", campaignID).
		Updates(map[string]interface{}{
			"for_count":     gorm.Expr("for_count + ?", forDelta),
			"against_count": gorm.Expr("against_count + ?", againstDelta),
		}).Error
}

func (r *saleVoteRepository) CloseCampaign(ctx context.Context, campaignID uuid.UUID, result string, tx *gorm.DB) error {
	db := r.WithContext(ctx)
	if tx != nil {
		db = tx.WithContext(ctx)
	}
	return db.Model(&models.SaleVoteCampaign{}).
		Where("id = ?", campaignID).
		Updates(map[string]interface{}{
			"status":     result,
			"result":     result,
			"closed_at":  gorm.Expr("NOW()"),
		}).Error
}

func (r *saleVoteRepository) FindVoteByUserAndCampaign(ctx context.Context, campaignID, userID uuid.UUID) (*models.SaleVote, error) {
	var v models.SaleVote
	err := r.WithContext(ctx).
		Where("campaign_id = ? AND user_id = ?", campaignID, userID).
		First(&v).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &v, nil
}

func (r *saleVoteRepository) ListVoters(ctx context.Context, campaignID uuid.UUID) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := r.WithContext(ctx).
		Model(&models.SaleVote{}).
		Where("campaign_id = ?", campaignID).
		Pluck("user_id", &ids).Error
	return ids, err
}

// Ensure clause is referenced (used by callers for locking hints).
var _ = clause.Locking{}
