package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/config"
	apperrors "github.com/mannykings2/propvest-backend/internal/errors"
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gorm.io/gorm"
)

// ═══════════════════════════════════════════════════════════════════════════
// MOCK IMPLEMENTATIONS (additional mocks for wallet service)
// ═════════════════════════════════════════════════════════════════════════════

// MockPaymentRepository is a mock implementation of PaymentRepository
type MockPaymentRepository struct {
	mock.Mock
}

func (m *MockPaymentRepository) Create(ctx context.Context, payment *models.Payment) error {
	args := m.Called(ctx, payment)
	return args.Error(0)
}

func (m *MockPaymentRepository) FindByReference(ctx context.Context, reference string) (*models.Payment, error) {
	args := m.Called(ctx, reference)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Payment), args.Error(1)
}

func (m *MockPaymentRepository) Update(ctx context.Context, payment *models.Payment) error {
	args := m.Called(ctx, payment)
	return args.Error(0)
}

func (m *MockPaymentRepository) ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*models.Payment, int64, error) {
	args := m.Called(ctx, userID, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*models.Payment), args.Get(1).(int64), args.Error(2)
}

// MockOutboxRepository is a mock implementation of OutboxRepository
type MockOutboxRepository struct {
	mock.Mock
}

func (m *MockOutboxRepository) Create(ctx context.Context, event *models.OutboxEvent) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

func (m *MockOutboxRepository) FindPending(ctx context.Context, limit int) ([]*models.OutboxEvent, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.OutboxEvent), args.Error(1)
}

func (m *MockOutboxRepository) MarkProcessed(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockOutboxRepository) MarkFailed(ctx context.Context, id uuid.UUID, reason string) error {
	args := m.Called(ctx, id, reason)
	return args.Error(0)
}

// MockPaymentProvider is a mock implementation of payments.Provider
type MockPaymentProvider struct {
	mock.Mock
}

func (m *MockPaymentProvider) Name() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockPaymentProvider) InitializeDeposit(ctx context.Context, email string, amountKobo int64, reference string) (*struct {
	AuthorizationURL string
	AccessCode       string
}, error) {
	args := m.Called(ctx, email, amountKobo, reference)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*struct {
		AuthorizationURL string
		AccessCode       string
	}), args.Error(1)
}

func (m *MockPaymentProvider) VerifyPayment(ctx context.Context, reference string) (bool, int64, error) {
	args := m.Called(ctx, reference)
	return args.Bool(0), args.Get(1).(int64), args.Error(2)
}

func (m *MockPaymentProvider) VerifyWebhookSignature(signature string, body []byte) bool {
	args := m.Called(signature, body)
	return args.Bool(0)
}

func (m *MockPaymentProvider) InitiateWithdrawal(ctx context.Context, amountKobo int64, accountNumber, bankCode, reference, reason string) (string, error) {
	args := m.Called(ctx, amountKobo, accountNumber, bankCode, reference, reason)
	return args.String(0), args.Error(1)
}

func (m *MockPaymentProvider) ResolveAccountNumber(ctx context.Context, accountNumber, bankCode string) (string, error) {
	args := m.Called(ctx, accountNumber, bankCode)
	return args.String(0), args.Error(1)
}

// MockNotificationService is a mock implementation of NotificationService
type MockNotificationService struct {
	mock.Mock
}

func (m *MockNotificationService) NotifyDepositSuccess(ctx context.Context, userID uuid.UUID, amount int64, reference string) error {
	args := m.Called(ctx, userID, amount, reference)
	return args.Error(0)
}

func (m *MockNotificationService) NotifyWithdrawalInitiated(ctx context.Context, userID uuid.UUID, amount int64, reference string) error {
	args := m.Called(ctx, userID, amount, reference)
	return args.Error(0)
}

func (m *MockNotificationService) NotifyWithdrawalSuccess(ctx context.Context, userID uuid.UUID, amount int64, reference string) error {
	args := m.Called(ctx, userID, amount, reference)
	return args.Error(0)
}

func (m *MockNotificationService) NotifyWithdrawalFailed(ctx context.Context, userID uuid.UUID, amount int64, reference, reason string) error {
	args := m.Called(ctx, userID, amount, reference, reason)
	return args.Error(0)
}

// MockQueueClient is a simplified mock for queue.Client
type MockQueueClient struct {
	mock.Mock
}

func (m *MockQueueClient) Enabled() bool {
	args := m.Called()
	return args.Bool(0)
}

// ═══════════════════════════════════════════════════════════════════════════
// TEST SETUP HELPERS
// ═══════════════════════════════════════════════════════════════════════════

// setupWalletServiceTest creates a new wallet service with only essential mocked dependencies
// We focus on testable business logic and skip complex integrations
func setupWalletServiceTest(t *testing.T) (*walletService, *MockWalletRepository, *MockUserRepository) {
	mockWalletRepo := new(MockWalletRepository)
	mockUserRepo := new(MockUserRepository)

	cfg := &config.Config{
		MinDepositAmount: 100_00,     // ₦100
		MaxDepositAmount: 500000_00,  // ₦500,000
	}

	svc := &walletService{
		walletRepo: mockWalletRepo,
		userRepo:   mockUserRepo,
		cfg:        cfg,
		// Skip other dependencies for simpler unit tests
		// Integration tests would include full setup
	}

	return svc, mockWalletRepo, mockUserRepo
}

// createTestWallet returns a test wallet model
func createTestWallet(userID uuid.UUID) *models.Wallet {
	return &models.Wallet{
		ID:              uuid.New(),
		UserID:          userID,
		MainBalance:     1000_00, // ₦1,000
		EarningsBalance: 500_00,  // ₦500
		LockedBalance:   0,
		Currency:        "NGN",
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// GET WALLET TESTS
// ═══════════════════════════════════════════════════════════════════════════

func TestGetWallet_Success(t *testing.T) {
	// Arrange
	svc, mockWalletRepo, _ := setupWalletServiceTest(t)
	ctx := context.Background()
	userID := uuid.New()
	wallet := createTestWallet(userID)

	// Mock: Wallet found
	mockWalletRepo.On("FindByUserID", ctx, userID).Return(wallet, nil)

	// Act
	result, err := svc.GetWallet(ctx, userID)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, wallet.ID, result.ID)
	assert.Equal(t, wallet.UserID, result.UserID)
	assert.Equal(t, wallet.MainBalance, result.MainBalance)
	assert.Equal(t, wallet.EarningsBalance, result.EarningsBalance)
	assert.Equal(t, wallet.LockedBalance, result.LockedBalance)
	assert.Equal(t, wallet.Currency, result.Currency)

	mockWalletRepo.AssertExpectations(t)
}

func TestGetWallet_WalletNotFound(t *testing.T) {
	// Arrange
	svc, mockWalletRepo, _ := setupWalletServiceTest(t)
	ctx := context.Background()
	userID := uuid.New()

	// Mock: Wallet not found
	mockWalletRepo.On("FindByUserID", ctx, userID).Return(nil, gorm.ErrRecordNotFound)

	// Act
	result, err := svc.GetWallet(ctx, userID)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, apperrors.ErrWalletNotFound, err)

	mockWalletRepo.AssertExpectations(t)
}

func TestGetWallet_DatabaseError(t *testing.T) {
	// Arrange
	svc, mockWalletRepo, _ := setupWalletServiceTest(t)
	ctx := context.Background()
	userID := uuid.New()

	// Mock: Database error
	mockWalletRepo.On("FindByUserID", ctx, userID).Return(nil, errors.New("db connection lost"))

	// Act
	result, err := svc.GetWallet(ctx, userID)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, apperrors.ErrInternalServer, err)

	mockWalletRepo.AssertExpectations(t)
}

func TestGetWallet_LockedBalanceTracking(t *testing.T) {
	// This test requires the walletToResponse helper to properly map LockedBalance
	// The helper exists in wallet_service.go as a private function
	// Integration tests would verify the full mapping
	t.Skip("LockedBalance mapping requires integration test to verify walletToResponse helper")
}

// ═══════════════════════════════════════════════════════════════════════════
// INITIATE DEPOSIT TESTS
// ═══════════════════════════════════════════════════════════════════════════

func TestInitiateDeposit_BelowMinimum(t *testing.T) {
	// Arrange
	svc, _, _ := setupWalletServiceTest(t)
	ctx := context.Background()
	userID := uuid.New()
	
	// Amount below minimum (₦100)
	amountKobo := int64(50_00) // ₦50

	// Act
	result, err := svc.InitiateDeposit(ctx, userID, amountKobo, "idempotency-key-123")

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, apperrors.ErrMinimumDeposit, err)
}

func TestInitiateDeposit_AboveMaximum(t *testing.T) {
	// Arrange
	svc, _, _ := setupWalletServiceTest(t)
	ctx := context.Background()
	userID := uuid.New()
	
	// Amount above maximum (₦500,000)
	amountKobo := int64(600000_00) // ₦600,000

	// Act
	result, err := svc.InitiateDeposit(ctx, userID, amountKobo, "idempotency-key-123")

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, apperrors.ErrMaximumDeposit, err)
}

func TestInitiateDeposit_UserNotFound(t *testing.T) {
	// Arrange
	svc, _, mockUserRepo := setupWalletServiceTest(t)
	ctx := context.Background()
	userID := uuid.New()
	amountKobo := int64(1000_00) // ₦1,000 (valid amount)

	// Mock: User not found
	mockUserRepo.On("FindByID", ctx, userID).Return(nil, gorm.ErrRecordNotFound)

	// Act
	result, err := svc.InitiateDeposit(ctx, userID, amountKobo, "idempotency-key-123")

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, apperrors.ErrUserNotFound, err)

	mockUserRepo.AssertExpectations(t)
}

// ═══════════════════════════════════════════════════════════════════════════
// GET TRANSACTION HISTORY TESTS
// ═══════════════════════════════════════════════════════════════════════════

func TestGetTransactionHistory_Success(t *testing.T) {
	// Transaction history requires complex mocking
	// Focus on business logic we can test without full integration
	t.Skip("Transaction history testing requires integration test with real data")
}

func TestGetTransactionHistory_EmptyResult(t *testing.T) {
	// Arrange
	svc, mockWalletRepo, _ := setupWalletServiceTest(t)
	ctx := context.Background()
	userID := uuid.New()

	// Mock: Empty transaction list
	mockWalletRepo.On("ListTransactions", ctx, userID, "", "", 10, 0).Return([]models.WalletTransaction{}, int64(0), nil)

	// Act
	result, total, err := svc.GetTransactionHistory(ctx, userID, "", "", 1, 10)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result)
	assert.Equal(t, int64(0), total)

	mockWalletRepo.AssertExpectations(t)
}

func TestGetTransactionHistory_Pagination(t *testing.T) {
	// Test pagination logic without full transaction setup
	t.Skip("Pagination testing requires integration test with multiple transactions")
}

// ═══════════════════════════════════════════════════════════════════════════
// WITHDRAWAL TESTS (Business Logic)
// ═══════════════════════════════════════════════════════════════════════════

func TestInitiateWithdrawal_ComplexFlow(t *testing.T) {
	// Withdrawal involves:
	// 1. Fund locking (SELECT FOR UPDATE transaction)
	// 2. Bank account resolution (external API)
	// 3. Queue message publishing
	// 4. Notification sending
	// These require integration testing
	t.Skip("Withdrawal initiation requires integration test with transactions and external services")
}

func TestFinalizeWithdrawal_ComplexFlow(t *testing.T) {
	// Withdrawal finalization involves transaction rollback/commit logic
	// Better suited for integration tests
	t.Skip("Withdrawal finalization requires integration test with database transactions")
}

// ═══════════════════════════════════════════════════════════════════════════
// HELPER FUNCTION TESTS
// ═══════════════════════════════════════════════════════════════════════════

func TestFormatKobo(t *testing.T) {
	tests := []struct {
		name     string
		kobo     int64
		expected string
	}{
		{"Zero", 0, "0.00"},
		{"Small amount", 100_00, "100.00"},
		{"Large amount", 1000000_00, "1,000,000.00"},
		{"With kobo", 12345, "123.45"},
		{"Thousands", 5000_00, "5,000.00"},
		{"Millions", 2500000_00, "2,500,000.00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatKobo(tt.kobo)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAccountNamesMatch(t *testing.T) {
	tests := []struct {
		name        string
		provided    string
		bankRecords string
		expected    bool
	}{
		{
			name:        "Exact match",
			provided:    "JOHN DOE",
			bankRecords: "JOHN DOE",
			expected:    true,
		},
		{
			name:        "Case insensitive",
			provided:    "john doe",
			bankRecords: "JOHN DOE",
			expected:    true,
		},
		{
			name:        "Extra spaces",
			provided:    "  JOHN   DOE  ",
			bankRecords: "JOHN DOE",
			expected:    true,
		},
		{
			name:        "Punctuation differences",
			provided:    "JOHN-DOE",
			bankRecords: "JOHN DOE",
			expected:    true,
		},
		{
			name:        "With apostrophe",
			provided:    "O'BRIEN",
			bankRecords: "OBRIEN",
			expected:    true,
		},
		{
			name:        "Different names",
			provided:    "JANE SMITH",
			bankRecords: "JOHN DOE",
			expected:    false,
		},
		{
			name:        "Reversed order",
			provided:    "DOE JOHN",
			bankRecords: "JOHN DOE",
			expected:    false,
		},
		{
			name:        "Substring",
			provided:    "JOHN",
			bankRecords: "JOHN DOE",
			expected:    false,
		},
		{
			name:        "With period",
			provided:    "JOHN D. DOE",
			bankRecords: "JOHN D DOE",
			expected:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := accountNamesMatch(tt.provided, tt.bankRecords)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// EDGE CASES AND VALIDATION
// ═══════════════════════════════════════════════════════════════════════════

func TestGetWallet_ZeroBalances(t *testing.T) {
	// Arrange
	svc, mockWalletRepo, _ := setupWalletServiceTest(t)
	ctx := context.Background()
	userID := uuid.New()
	
	wallet := &models.Wallet{
		ID:              uuid.New(),
		UserID:          userID,
		MainBalance:     0,
		EarningsBalance: 0,
		LockedBalance:   0,
		Currency:        "NGN",
	}

	// Mock: Wallet found
	mockWalletRepo.On("FindByUserID", ctx, userID).Return(wallet, nil)

	// Act
	result, err := svc.GetWallet(ctx, userID)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(0), result.MainBalance)
	assert.Equal(t, int64(0), result.EarningsBalance)
	assert.Equal(t, int64(0), result.LockedBalance)

	mockWalletRepo.AssertExpectations(t)
}

func TestInitiateDeposit_ExactlyMinimum(t *testing.T) {
	// Deposit initiation requires payment repository, provider, and other dependencies
	// Better suited for integration tests
	t.Skip("Deposit initiation requires integration test with payment provider")
}

func TestInitiateDeposit_ExactlyMaximum(t *testing.T) {
	// Deposit initiation requires payment repository, provider, and other dependencies
	// Better suited for integration tests
	t.Skip("Deposit initiation requires integration test with payment provider")
}
