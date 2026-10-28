package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mannykings2/propvest-backend/internal/dto"
	apperrors "github.com/mannykings2/propvest-backend/internal/errors"
	"github.com/mannykings2/propvest-backend/internal/logger"
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/mannykings2/propvest-backend/internal/repositories"
)

// OffplanService implements the off-plan module (Phase 2, Workstream B).
//
// Endpoints backed by this service:
//   - ListProjects / GetProject: public read of seeded projects
//   - Subscribe: instalment purchase that debits the wallet in one locked
//     transaction (down payment + 2% fee) and records the subscription
//
// Instalment auto-debit scheduling and cancellation fees are explicitly OUT of
// scope for Phase 2 (no scheduler exists).
type OffplanService interface {
	// ListProjects returns active projects in the shape OffPlan.jsx renders.
	ListProjects(ctx context.Context) ([]dto.OffplanProjectResponse, error)
	// GetProject returns one project or ErrPropertyNotFound.
	GetProject(ctx context.Context, id uuid.UUID) (*dto.OffplanProjectResponse, error)
	// Subscribe debits the wallet and creates a subscription record.
	Subscribe(ctx context.Context, userID, projectID uuid.UUID, req dto.SubscribeOffplanRequest) (*dto.OffplanSubscriptionResponse, error)
	// ListSubscriptions returns the user's subscriptions.
	ListSubscriptions(ctx context.Context, userID uuid.UUID) ([]dto.OffplanSubscriptionResponse, error)
}

type offplanService struct {
	offplanRepo repositories.OffplanRepository
	walletRepo  repositories.WalletRepository
	db          *gorm.DB
	ledger      LedgerService // Phase 7: balanced journal alongside the debit
}

// NewOffplanService constructs the service.
func NewOffplanService(
	offplanRepo repositories.OffplanRepository,
	walletRepo repositories.WalletRepository,
	db *gorm.DB,
	ledger LedgerService,
) OffplanService {
	return &offplanService{
		offplanRepo: offplanRepo,
		walletRepo:  walletRepo,
		db:          db,
		ledger:      ledger,
	}
}

// offplanProjectToResponse maps the model to the frontend's render shape.
// finish_types/plans/renders are Naira-denominated in the JSONB (matching the
// frontend constants); price_kobo is converted to Naira here for the top-level
// "price" field the UI shows.
func offplanProjectToResponse(p *models.OffplanProject) dto.OffplanProjectResponse {
	return dto.OffplanProjectResponse{
		ID:          p.ID,
		Name:        p.Name,
		Loc:         p.Location,
		Type:        p.PropertyType,
		Price:       p.PriceKobo / 100, // kobo → naira for display
		Stage:       p.Stage,
		Completion:  p.Completion,
		Developer:   p.Developer,
		DownPct:     p.DownPct,
		Img:         p.Img,
		Features:    json.RawMessage(p.Features),
		Renders:     json.RawMessage(p.Renders),
		FinishTypes: json.RawMessage(p.FinishTypes),
		Plans:       json.RawMessage(p.Plans),
	}
}

func (s *offplanService) ListProjects(ctx context.Context) ([]dto.OffplanProjectResponse, error) {
	projects, err := s.offplanRepo.ListProjects(ctx)
	if err != nil {
		logger.FromContext(ctx).Error("failed to list off-plan projects", "error", err)
		return nil, apperrors.ErrInternalServer
	}

	out := make([]dto.OffplanProjectResponse, 0, len(projects))
	for i := range projects {
		out = append(out, offplanProjectToResponse(&projects[i]))
	}
	return out, nil
}

func (s *offplanService) GetProject(ctx context.Context, id uuid.UUID) (*dto.OffplanProjectResponse, error) {
	p, err := s.offplanRepo.FindByID(ctx, id)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			return nil, apperrors.ErrPropertyNotFound
		}
		return nil, apperrors.ErrInternalServer
	}
	resp := offplanProjectToResponse(p)
	return &resp, nil
}

// finishType is the decoded shape of one entry in the project's finish_types
// JSONB array (copied verbatim from the frontend constants, which are Naira).
type finishType struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Price int64  `json:"price"` // NAIRA
}

// Subscribe debits the wallet (down payment + 2% fee) and records the
// subscription atomically.
//
// FINANCIAL SAFETY (same rules as the investment module):
//   - Single DB transaction; wallet row locked with SELECT ... FOR UPDATE
//   - Amounts derived server-side from the project's finish type (never trust
//     the client's price)
//   - Exactly one ledger row for the debit
//   - Rejects with insufficient_funds when the main balance can't cover it
func (s *offplanService) Subscribe(ctx context.Context, userID, projectID uuid.UUID, req dto.SubscribeOffplanRequest) (*dto.OffplanSubscriptionResponse, error) {
	project, err := s.offplanRepo.FindByID(ctx, projectID)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			return nil, apperrors.ErrPropertyNotFound
		}
		return nil, apperrors.ErrInternalServer
	}

	// Resolve the chosen finish type from the JSONB array.
	priceKobo, ferr := resolveFinishTypePrice(json.RawMessage(project.FinishTypes), req.FinishTypeID)
	if ferr != nil {
		return nil, ferr
	}

	// Validate the plan id exists in the project's plans array.
	if ok := planExists(json.RawMessage(project.Plans), req.PlanID); !ok {
		return nil, apperrors.NewAppError("Invalid instalment plan", "invalid_plan")
	}

	// Down payment = price × down_pct/100. Fee = 2% of the down payment.
	downKobo := priceKobo * int64(project.DownPct) / 100
	feeKobo := downKobo * 2 / 100
	totalDebit := downKobo + feeKobo

	reference := "OSP-" + strings.ToUpper(uuid.NewString()[:12])

	var created *models.OffplanSubscription

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		wallet, werr := s.walletRepo.FindByUserIDForUpdate(ctx, userID, tx)
		if werr != nil {
			if repositories.IsErrRecordNotFound(werr) {
				return apperrors.ErrWalletNotFound
			}
			return werr
		}

		if wallet.AvailableBalance() < totalDebit {
			return apperrors.NewAppError("Insufficient wallet balance to cover the down payment", "insufficient_funds")
		}

		before := wallet.MainBalance
		afterMain := before - totalDebit

		if uerr := tx.Model(&models.Wallet{}).
			Where("id = ?", wallet.ID).
			Update("main_balance", afterMain).Error; uerr != nil {
			return uerr
		}

		ledger := &models.WalletTransaction{
			WalletID:      wallet.ID,
			UserID:        userID,
			Type:          "investment",
			Amount:        totalDebit,
			BalanceBefore: before,
			BalanceAfter:  afterMain,
			Reference:     reference,
			Description:   fmt.Sprintf("Off-plan subscription: %s (%s finish, %s plan)", project.Name, req.FinishTypeID, req.PlanID),
			Status:        "completed",
		}
		if cerr := s.walletRepo.CreateTransaction(ctx, tx, ledger); cerr != nil {
			return cerr
		}

		// Phase 7: DR 2010 user wallet for the whole debit, then the split —
		// CR 2200 holdings (the down payment) and CR 3000 fee revenue (the
		// existing 2% subscription fee, recorded in the books like every fee).
		if lerr := s.ledger.Investment(ctx, tx, userID, &ledger.ID, downKobo, reference); lerr != nil {
			return lerr
		}
		if feeKobo > 0 {
			if _, lerr := s.ledger.Post(ctx, tx,
				LedgerLine{AccountCode: models.AccountFeeRevenueTransaction, Credit: feeKobo,
					Description: "Off-plan subscription fee (2%)", Ref: reference},
				LedgerLine{AccountCode: models.AccountUserWallets, UserID: &userID, WalletTxnID: &ledger.ID,
					Debit: feeKobo, Description: "Off-plan subscription fee", Ref: reference},
			); lerr != nil {
				return lerr
			}
		}

		sub := &models.OffplanSubscription{
			UserID:          userID,
			ProjectID:       projectID,
			PlanID:          req.PlanID,
			FinishTypeID:    req.FinishTypeID,
			PriceKobo:       priceKobo,
			DownPaymentKobo: downKobo,
			FeeKobo:         feeKobo,
			Reference:       reference,
			Status:          models.OffplanSubActive,
		}
		if serr := s.offplanRepo.CreateSubscription(ctx, sub, tx); serr != nil {
			return serr
		}

		created = sub
		return nil
	})

	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrWalletNotFound),
			errors.Is(err, apperrors.ErrPropertyNotFound),
			errors.Is(err, apperrors.ErrInvalidAmount):
			return nil, err
		}
		// Insufficient funds / invalid plan bubble up with their own message.
		var appErr *apperrors.AppError
		if errors.As(err, &appErr) {
			return nil, appErr
		}
		logger.FromContext(ctx).Error("off-plan subscribe failed", "user_id", userID, "error", err)
		return nil, apperrors.ErrInternalServer
	}

	logger.FromContext(ctx).Info("off-plan subscription created",
		"user_id", userID, "project_id", projectID, "reference", reference, "total_debit_kobo", totalDebit)

	return &dto.OffplanSubscriptionResponse{
		SubscriptionID:   created.ID,
		ProjectID:        projectID,
		ProjectName:      project.Name,
		Reference:        reference,
		PlanID:           req.PlanID,
		FinishTypeID:     req.FinishTypeID,
		PriceKobo:        priceKobo,
		DownPaymentKobo:  downKobo,
		FeeKobo:          feeKobo,
		TotalDebitedKobo: totalDebit,
		Status:           created.Status,
		CreatedAt:        created.CreatedAt,
	}, nil
}

func (s *offplanService) ListSubscriptions(ctx context.Context, userID uuid.UUID) ([]dto.OffplanSubscriptionResponse, error) {
	subs, err := s.offplanRepo.ListSubscriptions(ctx, userID)
	if err != nil {
		return nil, apperrors.ErrInternalServer
	}
	out := make([]dto.OffplanSubscriptionResponse, 0, len(subs))
	for _, sub := range subs {
		out = append(out, dto.OffplanSubscriptionResponse{
			SubscriptionID:   sub.ID,
			ProjectID:        sub.ProjectID,
			Reference:        sub.Reference,
			PlanID:           sub.PlanID,
			FinishTypeID:     sub.FinishTypeID,
			PriceKobo:        sub.PriceKobo,
			DownPaymentKobo:  sub.DownPaymentKobo,
			FeeKobo:          sub.FeeKobo,
			TotalDebitedKobo: sub.DownPaymentKobo + sub.FeeKobo,
			Status:           sub.Status,
			CreatedAt:        sub.CreatedAt,
		})
	}
	return out, nil
}

// resolveFinishTypePrice finds the finish type by id and returns its price in
// KOBO (the JSONB stores Naira, so ×100). Returns ErrInvalidAmount when the id
// doesn't match any entry.
func resolveFinishTypePrice(raw json.RawMessage, finishTypeID string) (int64, error) {
	var types []finishType
	if err := json.Unmarshal(raw, &types); err != nil {
		return 0, apperrors.ErrInternalServer
	}
	for _, ft := range types {
		if ft.ID == finishTypeID {
			if ft.Price <= 0 {
				return 0, apperrors.ErrInvalidAmount
			}
			return ft.Price * 100, nil
		}
	}
	return 0, apperrors.NewAppError("Invalid finish type", "invalid_finish_type")
}

// planExists reports whether the plan id is present in the plans JSONB array.
func planExists(raw json.RawMessage, planID string) bool {
	var plans []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &plans); err != nil {
		return false
	}
	for _, p := range plans {
		if p.ID == planID {
			return true
		}
	}
	return false
}

var _ = time.Now
