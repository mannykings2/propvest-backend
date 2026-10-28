package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockWalletService is a mock implementation of services.WalletService
type MockWalletService struct {
	mock.Mock
}

func (m *MockWalletService) GetWallet(ctx context.Context, userID uuid.UUID) (*dto.WalletResponse, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.WalletResponse), args.Error(1)
}

func (m *MockWalletService) InitiateDeposit(ctx context.Context, userID uuid.UUID, amountKobo int64, idempotencyKey string) (*dto.DepositResponse, error) {
	args := m.Called(ctx, userID, amountKobo, idempotencyKey)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.DepositResponse), args.Error(1)
}

func (m *MockWalletService) CreditFromPaymentReference(ctx context.Context, reference string, rawPayload []byte) error {
	args := m.Called(ctx, reference, rawPayload)
	return args.Error(0)
}

func (m *MockWalletService) InitiateWithdrawal(ctx context.Context, userID uuid.UUID, req dto.WithdrawRequest) (*dto.TransactionResponse, error) {
	args := m.Called(ctx, userID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.TransactionResponse), args.Error(1)
}

func (m *MockWalletService) FinalizeWithdrawal(ctx context.Context, transactionID uuid.UUID, success bool, providerReference, failureReason string) error {
	args := m.Called(ctx, transactionID, success, providerReference, failureReason)
	return args.Error(0)
}

func (m *MockWalletService) GetWithdrawalByReference(ctx context.Context, reference string) (uuid.UUID, error) {
	args := m.Called(ctx, reference)
	return args.Get(0).(uuid.UUID), args.Error(1)
}

func (m *MockWalletService) GetTransactionHistory(ctx context.Context, userID uuid.UUID, txType, status string, page, limit int) ([]dto.TransactionResponse, int64, error) {
	args := m.Called(ctx, userID, txType, status, page, limit)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]dto.TransactionResponse), args.Get(1).(int64), args.Error(2)
}

func (m *MockWalletService) ListBanks() []dto.BankResponse {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).([]dto.BankResponse)
}

func (m *MockWalletService) VerifyAccount(ctx context.Context, req dto.VerifyAccountRequest) (*dto.VerifyAccountResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.VerifyAccountResponse), args.Error(1)
}

func (m *MockWalletService) TransferEarnings(ctx context.Context, userID uuid.UUID, amountKobo int64) (*dto.WalletResponse, error) {
	args := m.Called(ctx, userID, amountKobo)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.WalletResponse), args.Error(1)
}

func (m *MockWalletService) VerifyWebhookSignature(signature string, body []byte) bool {
	args := m.Called(signature, body)
	return args.Bool(0)
}

// Helper function to create test wallet response
func createTestWalletResponse() *dto.WalletResponse {
	virtualAcct := "9876543210"
	virtualBank := "Wema Bank"
	return &dto.WalletResponse{
		ID:              uuid.New(),
		UserID:          uuid.New(),
		MainBalance:     1000000, // 10,000 NGN in kobo
		LockedBalance:   50000,   // 500 NGN in kobo
		EarningsBalance: 25000,   // 250 NGN in kobo
		Currency:        "NGN",
		VirtualAcctNo:   &virtualAcct,
		VirtualBank:     &virtualBank,
		CreatedAt:       time.Now(),
	}
}

// ============================================================================
// GetWallet Tests
// ============================================================================

func TestWalletHandler_GetWallet_Success(t *testing.T) {
	// Arrange
	mockService := new(MockWalletService)
	handler := NewWalletHandler(mockService)
	router := setupTestRouter()

	testUserID := uuid.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID.String())
		c.Next()
	})
	router.GET("/wallet", handler.GetWallet)

	walletResp := createTestWalletResponse()
	mockService.On("GetWallet", mock.Anything, testUserID).
		Return(walletResp, nil)

	// Act
	req, _ := http.NewRequest("GET", "/wallet", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.True(t, response["success"].(bool))
	assert.NotNil(t, response["data"])

	mockService.AssertExpectations(t)
}

func TestWalletHandler_GetWallet_Unauthorized(t *testing.T) {
	// Arrange
	mockService := new(MockWalletService)
	handler := NewWalletHandler(mockService)
	router := setupTestRouter()

	// No authentication middleware - user_id not set
	router.GET("/wallet", handler.GetWallet)

	// Act
	req, _ := http.NewRequest("GET", "/wallet", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.False(t, response["success"].(bool))

	mockService.AssertNotCalled(t, "GetWallet")
}

func TestWalletHandler_GetWallet_WalletNotFound(t *testing.T) {
	// Arrange
	mockService := new(MockWalletService)
	handler := NewWalletHandler(mockService)
	router := setupTestRouter()

	testUserID := uuid.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID.String())
		c.Next()
	})
	router.GET("/wallet", handler.GetWallet)

	mockService.On("GetWallet", mock.Anything, testUserID).
		Return(nil, errors.New("wallet not found"))

	// Act
	req, _ := http.NewRequest("GET", "/wallet", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockService.AssertExpectations(t)
}

// ============================================================================
// InitiateDeposit Tests
// ============================================================================

func TestWalletHandler_InitiateDeposit_Success(t *testing.T) {
	// Arrange
	mockService := new(MockWalletService)
	handler := NewWalletHandler(mockService)
	router := setupTestRouter()

	testUserID := uuid.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID.String())
		c.Next()
	})
	router.POST("/deposit", handler.InitiateDeposit)

	reqBody := dto.DepositRequest{
		Amount: 1000000, // 10,000 NGN in kobo
	}

	depositResp := &dto.DepositResponse{
		AuthorizationURL: "https://paystack.com/pay/abc123",
		Reference:        "DEP-123456",
	}

	mockService.On("InitiateDeposit", mock.Anything, testUserID, int64(1000000), "").
		Return(depositResp, nil)

	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("POST", "/deposit", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.True(t, response["success"].(bool))
	assert.NotNil(t, response["data"])

	mockService.AssertExpectations(t)
}

func TestWalletHandler_InitiateDeposit_WithIdempotencyKey(t *testing.T) {
	// Arrange
	mockService := new(MockWalletService)
	handler := NewWalletHandler(mockService)
	router := setupTestRouter()

	testUserID := uuid.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID.String())
		c.Next()
	})
	router.POST("/deposit", handler.InitiateDeposit)

	reqBody := dto.DepositRequest{
		Amount: 1000000,
	}

	idempotencyKey := "idempotency-key-12345"

	depositResp := &dto.DepositResponse{
		AuthorizationURL: "https://paystack.com/pay/abc123",
		Reference:        "DEP-123456",
	}

	mockService.On("InitiateDeposit", mock.Anything, testUserID, int64(1000000), idempotencyKey).
		Return(depositResp, nil)

	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("POST", "/deposit", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempotencyKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)
	mockService.AssertExpectations(t)
}

func TestWalletHandler_InitiateDeposit_ValidationError_ZeroAmount(t *testing.T) {
	// Arrange
	mockService := new(MockWalletService)
	handler := NewWalletHandler(mockService)
	router := setupTestRouter()

	testUserID := uuid.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID.String())
		c.Next()
	})
	router.POST("/deposit", handler.InitiateDeposit)

	reqBody := dto.DepositRequest{
		Amount: 0, // Invalid: must be > 0
	}
	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("POST", "/deposit", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	mockService.AssertNotCalled(t, "InitiateDeposit")
}

func TestWalletHandler_InitiateDeposit_ValidationError_NegativeAmount(t *testing.T) {
	// Arrange
	mockService := new(MockWalletService)
	handler := NewWalletHandler(mockService)
	router := setupTestRouter()

	testUserID := uuid.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID.String())
		c.Next()
	})
	router.POST("/deposit", handler.InitiateDeposit)

	reqBody := dto.DepositRequest{
		Amount: -1000, // Invalid: must be > 0
	}
	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("POST", "/deposit", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	mockService.AssertNotCalled(t, "InitiateDeposit")
}

func TestWalletHandler_InitiateDeposit_Unauthorized(t *testing.T) {
	// Arrange
	mockService := new(MockWalletService)
	handler := NewWalletHandler(mockService)
	router := setupTestRouter()

	// No authentication middleware
	router.POST("/deposit", handler.InitiateDeposit)

	reqBody := dto.DepositRequest{
		Amount: 1000000,
	}
	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("POST", "/deposit", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	mockService.AssertNotCalled(t, "InitiateDeposit")
}

// ============================================================================
// RequestWithdrawal Tests
// ============================================================================

func TestWalletHandler_RequestWithdrawal_Success(t *testing.T) {
	// Arrange
	mockService := new(MockWalletService)
	handler := NewWalletHandler(mockService)
	router := setupTestRouter()

	testUserID := uuid.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID.String())
		c.Next()
	})
	router.POST("/withdraw", handler.RequestWithdrawal)

	reqBody := dto.WithdrawRequest{
		Amount:        500000, // 5,000 NGN in kobo
		BankCode:      "058",
		AccountNumber: "0123456789",
		AccountName:   "John Doe",
	}

	txnResp := &dto.TransactionResponse{
		ID:            uuid.New(),
		Type:          "withdrawal",
		Amount:        500000,
		BalanceBefore: 1000000,
		BalanceAfter:  500000,
		Reference:     "WDL-123456",
		Description:   "Withdrawal to 0123456789",
		Status:        "pending",
		CreatedAt:     time.Now(),
	}

	mockService.On("InitiateWithdrawal", mock.Anything, testUserID, reqBody).
		Return(txnResp, nil)

	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("POST", "/withdraw", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.True(t, response["success"].(bool))
	assert.NotNil(t, response["data"])

	mockService.AssertExpectations(t)
}

func TestWalletHandler_RequestWithdrawal_ValidationError_InvalidAccountNumber(t *testing.T) {
	// Arrange
	mockService := new(MockWalletService)
	handler := NewWalletHandler(mockService)
	router := setupTestRouter()

	testUserID := uuid.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID.String())
		c.Next()
	})
	router.POST("/withdraw", handler.RequestWithdrawal)

	reqBody := dto.WithdrawRequest{
		Amount:        500000,
		BankCode:      "058",
		AccountNumber: "12345", // Too short (must be exactly 10)
		AccountName:   "John Doe",
	}
	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("POST", "/withdraw", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	mockService.AssertNotCalled(t, "InitiateWithdrawal")
}

func TestWalletHandler_RequestWithdrawal_InsufficientBalance(t *testing.T) {
	// Arrange
	mockService := new(MockWalletService)
	handler := NewWalletHandler(mockService)
	router := setupTestRouter()

	testUserID := uuid.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID.String())
		c.Next()
	})
	router.POST("/withdraw", handler.RequestWithdrawal)

	reqBody := dto.WithdrawRequest{
		Amount:        10000000, // More than balance
		BankCode:      "058",
		AccountNumber: "0123456789",
		AccountName:   "John Doe",
	}

	mockService.On("InitiateWithdrawal", mock.Anything, testUserID, reqBody).
		Return(nil, errors.New("insufficient balance"))

	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("POST", "/withdraw", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockService.AssertExpectations(t)
}

// ============================================================================
// GetTransactions Tests
// ============================================================================

func TestWalletHandler_GetTransactions_Success(t *testing.T) {
	// Arrange
	mockService := new(MockWalletService)
	handler := NewWalletHandler(mockService)
	router := setupTestRouter()

	testUserID := uuid.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID.String())
		c.Next()
	})
	router.GET("/transactions", handler.GetTransactions)

	transactions := []dto.TransactionResponse{
		{
			ID:            uuid.New(),
			Type:          "deposit",
			Amount:        1000000,
			BalanceBefore: 0,
			BalanceAfter:  1000000,
			Reference:     "DEP-123",
			Description:   "Wallet deposit",
			Status:        "completed",
			CreatedAt:     time.Now(),
		},
	}

	mockService.On("GetTransactionHistory", mock.Anything, testUserID, "", "", 1, 20).
		Return(transactions, int64(1), nil)

	// Act
	req, _ := http.NewRequest("GET", "/transactions?page=1&limit=20", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.True(t, response["success"].(bool))
	assert.NotNil(t, response["data"])

	mockService.AssertExpectations(t)
}

func TestWalletHandler_GetTransactions_WithTypeFilter(t *testing.T) {
	// Arrange
	mockService := new(MockWalletService)
	handler := NewWalletHandler(mockService)
	router := setupTestRouter()

	testUserID := uuid.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID.String())
		c.Next()
	})
	router.GET("/transactions", handler.GetTransactions)

	transactions := []dto.TransactionResponse{
		{
			ID:            uuid.New(),
			Type:          "deposit",
			Amount:        1000000,
			BalanceBefore: 0,
			BalanceAfter:  1000000,
			Reference:     "DEP-123",
			Description:   "Wallet deposit",
			Status:        "completed",
			CreatedAt:     time.Now(),
		},
	}

	mockService.On("GetTransactionHistory", mock.Anything, testUserID, "deposit", "", 1, 20).
		Return(transactions, int64(1), nil)

	// Act
	req, _ := http.NewRequest("GET", "/transactions?page=1&limit=20&type=deposit", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)
	mockService.AssertExpectations(t)
}

func TestWalletHandler_GetTransactions_EmptyResult(t *testing.T) {
	// Arrange
	mockService := new(MockWalletService)
	handler := NewWalletHandler(mockService)
	router := setupTestRouter()

	testUserID := uuid.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID.String())
		c.Next()
	})
	router.GET("/transactions", handler.GetTransactions)

	transactions := []dto.TransactionResponse{}

	mockService.On("GetTransactionHistory", mock.Anything, testUserID, "", "", 1, 20).
		Return(transactions, int64(0), nil)

	// Act
	req, _ := http.NewRequest("GET", "/transactions?page=1&limit=20", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.True(t, response["success"].(bool))

	mockService.AssertExpectations(t)
}

func TestWalletHandler_GetTransactions_Unauthorized(t *testing.T) {
	// Arrange
	mockService := new(MockWalletService)
	handler := NewWalletHandler(mockService)
	router := setupTestRouter()

	// No authentication middleware
	router.GET("/transactions", handler.GetTransactions)

	// Act
	req, _ := http.NewRequest("GET", "/transactions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	mockService.AssertNotCalled(t, "GetTransactionHistory")
}
