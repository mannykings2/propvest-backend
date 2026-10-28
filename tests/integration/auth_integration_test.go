package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mannykings2/propvest-backend/internal/audit"
	"github.com/mannykings2/propvest-backend/internal/dto"
	"github.com/mannykings2/propvest-backend/internal/handlers"
	"github.com/mannykings2/propvest-backend/internal/mailer"
	"github.com/mannykings2/propvest-backend/internal/middleware"
	"github.com/mannykings2/propvest-backend/internal/repositories"
	"github.com/mannykings2/propvest-backend/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegistrationCreatesWalletAtomically tests that user registration
// creates both user and wallet in a single database transaction
func TestRegistrationCreatesWalletAtomically(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup
	testDB := SetupTestDB(t)
	defer testDB.Cleanup()

	// Create repositories
	userRepo := repositories.NewUserRepository(testDB.DB)
	walletRepo := repositories.NewWalletRepository(testDB.DB)
	refreshTokenRepo := repositories.NewRefreshTokenRepository(testDB.DB)
	verificationTokenRepo := repositories.NewVerificationTokenRepository(testDB.DB)

	// Create audit recorder
	auditRecorder := audit.NewLogRecorder()

	// Create service (with real dependencies)
	authService := services.NewAuthService(
		userRepo,
		walletRepo,
		refreshTokenRepo,
		verificationTokenRepo,
		&mailer.MockSender{}, // Register always sends a verification email
		auditRecorder,
		testDB.Config,
		testDB.DB,
	)

	// Create handler
	handler := handlers.NewAuthHandler(authService)

	// Setup router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestID())
	router.POST("/register", handler.Register)

	// Prepare request
	reqBody := dto.RegisterRequest{
		Email:    "test@example.com",
		Password: "SecurePassword123!",
		FullName: "Test User",
		Phone:    "+2348012345678",
	}
	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("POST", "/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert HTTP response
	assert.Equal(t, http.StatusCreated, w.Code, "Should return 201 Created")

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.True(t, response["success"].(bool))
	assert.NotNil(t, response["data"])

	// Extract user data
	data := response["data"].(map[string]interface{})
	userID := data["user"].(map[string]interface{})["id"].(string)
	require.NotEmpty(t, userID)

	// Verify user was created in database
	ctx := context.Background()
	user, err := userRepo.FindByEmail(ctx, reqBody.Email)
	require.NoError(t, err, "User should exist in database")
	assert.Equal(t, reqBody.Email, user.Email)
	assert.Equal(t, reqBody.FullName, user.FullName)
	assert.Equal(t, reqBody.Phone, user.Phone)

	// Verify wallet was created for the user
	wallet, err := walletRepo.FindByUserID(ctx, user.ID)
	require.NoError(t, err, "Wallet should exist for user")
	assert.Equal(t, user.ID, wallet.UserID)
	assert.Equal(t, int64(0), wallet.MainBalance, "Initial balance should be zero")
	assert.Equal(t, "NGN", wallet.Currency)
}

// TestRegistrationRollbackOnError tests that if wallet creation fails,
// the entire transaction is rolled back (user is not created)
func TestRegistrationRollbackOnError(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This test would require mocking wallet repo to fail
	// Or use database constraints to force a failure
	// Marked as TODO for now
	t.Skip("TODO: Implement transaction rollback test")
}

// TestLoginWithValidCredentials tests the complete login flow
func TestLoginWithValidCredentials(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup
	testDB := SetupTestDB(t)
	defer testDB.Cleanup()

	// Create repositories
	userRepo := repositories.NewUserRepository(testDB.DB)
	walletRepo := repositories.NewWalletRepository(testDB.DB)
	refreshTokenRepo := repositories.NewRefreshTokenRepository(testDB.DB)
	verificationTokenRepo := repositories.NewVerificationTokenRepository(testDB.DB)
	// Create audit recorder
	auditRecorder := audit.NewLogRecorder()

	// Create service
	authService := services.NewAuthService(
		userRepo,
		walletRepo,
		refreshTokenRepo,
		verificationTokenRepo,
		&mailer.MockSender{},
		auditRecorder,
		testDB.Config,
		testDB.DB,
	)

	handler := handlers.NewAuthHandler(authService)

	// Setup router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestID())
	router.POST("/register", handler.Register)
	router.POST("/login", handler.Login)

	// Step 1: Register a user
	registerReq := dto.RegisterRequest{
		Email:    "login@example.com",
		Password: "SecurePassword123!",
		FullName: "Login Test",
		Phone:    "+2348098765432",
	}
	jsonBody, _ := json.Marshal(registerReq)
	req, _ := http.NewRequest("POST", "/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	// Step 2: Login with correct credentials
	loginReq := dto.LoginRequest{
		Email:    "login@example.com",
		Password: "SecurePassword123!",
	}
	jsonBody, _ = json.Marshal(loginReq)
	req, _ = http.NewRequest("POST", "/login", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code, "Should return 200 OK")

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.True(t, response["success"].(bool))

	// Verify tokens are returned
	data := response["data"].(map[string]interface{})
	assert.NotEmpty(t, data["access_token"], "Should return access token")
	assert.NotEmpty(t, data["refresh_token"], "Should return refresh token")
	assert.NotNil(t, data["user"], "Should return user data")
}

// TestLoginWithInvalidPassword tests login failure with wrong password
func TestLoginWithInvalidPassword(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup
	testDB := SetupTestDB(t)
	defer testDB.Cleanup()

	// Create repositories
	userRepo := repositories.NewUserRepository(testDB.DB)
	walletRepo := repositories.NewWalletRepository(testDB.DB)
	refreshTokenRepo := repositories.NewRefreshTokenRepository(testDB.DB)
	verificationTokenRepo := repositories.NewVerificationTokenRepository(testDB.DB)
	// Create audit recorder
	auditRecorder := audit.NewLogRecorder()

	// Create service
	authService := services.NewAuthService(
		userRepo,
		walletRepo,
		refreshTokenRepo,
		verificationTokenRepo,
		&mailer.MockSender{},
		auditRecorder,
		testDB.Config,
		testDB.DB,
	)

	handler := handlers.NewAuthHandler(authService)

	// Setup router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestID())
	router.POST("/register", handler.Register)
	router.POST("/login", handler.Login)

	// Step 1: Register a user
	registerReq := dto.RegisterRequest{
		Email:    "wrongpass@example.com",
		Password: "CorrectPassword123!",
		FullName: "Wrong Pass Test",
		Phone:    "+2348076543210",
	}
	jsonBody, _ := json.Marshal(registerReq)
	req, _ := http.NewRequest("POST", "/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	// Step 2: Login with wrong password
	loginReq := dto.LoginRequest{
		Email:    "wrongpass@example.com",
		Password: "WrongPassword123!",
	}
	jsonBody, _ = json.Marshal(loginReq)
	req, _ = http.NewRequest("POST", "/login", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.NotEqual(t, http.StatusOK, w.Code, "Should not return 200 OK")
	assert.True(t, w.Code >= 400, "Should return error status code")
}
