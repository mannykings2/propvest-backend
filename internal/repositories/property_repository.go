package repositories

import (
	"context"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PropertyFilter holds the optional query filters for listing properties.
type PropertyFilter struct {
	Status      string
	State       string
	Location    string
	Type        string
	DeveloperID *uuid.UUID
	Sort        string // column name, defaults to created_at
	Order       string // asc|desc, defaults to desc
}

// PropertyRepository is the data-access contract for properties.
type PropertyRepository interface {
	Create(ctx context.Context, p *models.Property) error
	FindByID(ctx context.Context, id uuid.UUID) (*models.Property, error)
	FindByIDForUpdate(ctx context.Context, id uuid.UUID, tx *gorm.DB) (*models.Property, error)
	Update(ctx context.Context, p *models.Property) error
	UpdatePartial(ctx context.Context, id uuid.UUID, updates map[string]any) error
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context, f PropertyFilter, limit, offset int) ([]models.Property, int64, error)
	AddDocument(ctx context.Context, doc *models.PropertyDocument) error
	CountByStatus(ctx context.Context, status string) (int64, error)
}

type propertyRepository struct {
	*BaseRepository
}

// NewPropertyRepository constructs the repository.
func NewPropertyRepository(db *gorm.DB) PropertyRepository {
	return &propertyRepository{BaseRepository: NewBaseRepository(db)}
}

func (r *propertyRepository) Create(ctx context.Context, p *models.Property) error {
	return r.WithContext(ctx).Create(p).Error
}

func (r *propertyRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Property, error) {
	var p models.Property
	if err := r.WithContext(ctx).Preload("Documents").Where("id = ?", id).First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// FindByIDForUpdate locks the property row inside a transaction so concurrent
// investments can't oversell slots.
func (r *propertyRepository) FindByIDForUpdate(ctx context.Context, id uuid.UUID, tx *gorm.DB) (*models.Property, error) {
	var p models.Property
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", id).
		First(&p).Error
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *propertyRepository) Update(ctx context.Context, p *models.Property) error {
	return r.WithContext(ctx).Save(p).Error
}

func (r *propertyRepository) UpdatePartial(ctx context.Context, id uuid.UUID, updates map[string]any) error {
	return r.WithContext(ctx).Model(&models.Property{}).Where("id = ?", id).Updates(updates).Error
}

func (r *propertyRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.WithContext(ctx).Delete(&models.Property{}, id).Error
}

// List applies filters + pagination and returns rows plus the total count.
func (r *propertyRepository) List(ctx context.Context, f PropertyFilter, limit, offset int) ([]models.Property, int64, error) {
	q := r.WithContext(ctx).Model(&models.Property{})

	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.State != "" {
		q = q.Where("state = ?", f.State)
	}
	if f.Location != "" {
		q = q.Where("location ILIKE ?", "%"+f.Location+"%")
	}
	if f.Type != "" {
		q = q.Where("type = ?", f.Type)
	}
	if f.DeveloperID != nil {
		q = q.Where("developer_id = ?", *f.DeveloperID)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Whitelist sortable columns to prevent SQL injection via the sort param.
	sortColumn := "created_at"
	switch f.Sort {
	case "created_at", "slot_price", "yield_pct", "funded_pct", "name":
		sortColumn = f.Sort
	}
	order := "DESC"
	if f.Order == "asc" {
		order = "ASC"
	}

	var props []models.Property
	if err := q.Order(sortColumn + " " + order).Limit(limit).Offset(offset).Find(&props).Error; err != nil {
		return nil, 0, err
	}
	return props, total, nil
}

func (r *propertyRepository) AddDocument(ctx context.Context, doc *models.PropertyDocument) error {
	return r.WithContext(ctx).Create(doc).Error
}

func (r *propertyRepository) CountByStatus(ctx context.Context, status string) (int64, error) {
	var count int64
	err := r.WithContext(ctx).Model(&models.Property{}).Where("status = ?", status).Count(&count).Error
	return count, err
}
