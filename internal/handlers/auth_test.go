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
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock AuthService for testing
type MockAuthService struct {
	mock.Mock
}

func (m *MockAuthService) Register(ctx context.Context, req dto.RegisterRequest) (*dto.RegisterResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.RegisterResponse), args.Error(1)
}

func (m *MockAuthService) Login(ctx context.Context, req dto.LoginRequest) (*dto.LoginResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.LoginResponse), args.Error(1)
}

func (m *MockAuthService) RefreshAccessToken(ctx context.Context, req dto.RefreshRequest) (*dto.RefreshResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.RefreshResponse), args.Error(1)
}

func (m *MockAuthService) Logout(ctx context.Context, refreshToken string) error {
	args := m.Called(ctx, refreshToken)
	return args.Error(0)
}

func (m *MockAuthService) LogoutAllDevices(ctx context.Context, userID uuid.UUID) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

func (m *MockAuthService) VerifyEmail(ctx context.Context, token string) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

func (m *MockAuthService) ResendVerification(ctx context.Context, email string) error {
	args := m.Called(ctx, email)
	return args.Error(0)
}

func (m *MockAuthService) ForgotPassword(ctx context.Context, email string) error {
	args := m.Called(ctx, email)
	return args.Error(0)
}

func (m *MockAuthService) ResetPassword(ctx context.Context, token, newPassword string) error {
	args := m.Called(ctx, token, newPassword)
	return args.Error(0)
}

func (m *MockAuthService) LoginHistory(ctx context.Context, userID uuid.UUID, limit int) ([]*models.LoginHistory, error) {
	args := m.Called(ctx, userID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.LoginHistory), args.Error(1)
}

// Helper functions

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

func createRegisterResponse() *dto.RegisterResponse {
	userID := uuid.New()
	return &dto.RegisterResponse{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		TokenType:    "Bearer",
		User: dto.UserResponse{
			ID:            userID,
			UserCode:      "USR-" + userID.String()[:8],
			FullName:      "Test User",
			Email:         "test@example.com",
			Phone:         "+2348012345678",
			EmailVerified: false,
			KYCStatus:     "pending",
			Role:          models.RoleInvestor,
			IsActive:      true,
			CreatedAt:     time.Now(),
		},
	}
}

func createLoginResponse() *dto.LoginResponse {
	userID := uuid.New()
	return &dto.LoginResponse{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		TokenType:    "Bearer",
		User: dto.UserResponse{
			ID:            userID,
			UserCode:      "USR-" + userID.String()[:8],
			FullName:      "Test User",
			Email:         "test@example.com",
			Phone:         "+2348012345678",
			EmailVerified: true,
			KYCStatus:     "verified",
			Role:          models.RoleInvestor,
			IsActive:      true,
			CreatedAt:     time.Now(),
		},
	}
}

func createRefreshResponse() *dto.RefreshResponse {
	return &dto.RefreshResponse{
		AccessToken:  "new-access-token",
		RefreshToken: "new-refresh-token",
		TokenType:    "Bearer",
	}
}

// Tests

func TestAuthHandler_Register_Success(t *testing.T) {
	// Arrange
	mockService := new(MockAuthService)
	handler := NewAuthHandler(mockService)
	router := setupTestRouter()
	router.POST("/register", handler.Register)

	registerResp := createRegisterResponse()
	mockService.On("Register", mock.Anything, mock.AnythingOfType("dto.RegisterRequest")).
		Return(registerResp, nil)

	reqBody := dto.RegisterRequest{
		Email:    "test@example.com",
		Password: "SecurePass123!",
		FullName: "Test User",
		Phone:    "+2348012345678",
	}
	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("POST", "/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.True(t, response["success"].(bool))
	assert.NotNil(t, response["data"])

	mockService.AssertExpectations(t)
}

func TestAuthHandler_Register_ValidationError_InvalidEmail(t *testing.T) {
	// Arrange
	mockService := new(MockAuthService)
	handler := NewAuthHandler(mockService)
	router := setupTestRouter()
	router.POST("/register", handler.Register)

	reqBody := dto.RegisterRequest{
		Email:    "invalid-email", // Invalid email format
		Password: "SecurePass123!",
		FullName: "Test User",
		Phone:    "+2348012345678",
	}
	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("POST", "/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.False(t, response["success"].(bool))

	// Service should not be called due to validation failure
	mockService.AssertNotCalled(t, "Register")
}

func TestAuthHandler_Register_ValidationError_ShortPassword(t *testing.T) {
	// Arrange
	mockService := new(MockAuthService)
	handler := NewAuthHandler(mockService)
	router := setupTestRouter()
	router.POST("/register", handler.Register)

	reqBody := dto.RegisterRequest{
		Email:    "test@example.com",
		Password: "short", // Too short
		FullName: "Test User",
		Phone:    "+2348012345678",
	}
	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("POST", "/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	mockService.AssertNotCalled(t, "Register")
}

func TestAuthHandler_Register_DuplicateEmail(t *testing.T) {
	// Arrange
	mockService := new(MockAuthService)
	handler := NewAuthHandler(mockService)
	router := setupTestRouter()
	router.POST("/register", handler.Register)

	mockService.On("Register", mock.Anything, mock.AnythingOfType("dto.RegisterRequest")).
		Return(nil, errors.New("email already exists"))

	reqBody := dto.RegisterRequest{
		Email:    "existing@example.com",
		Password: "SecurePass123!",
		FullName: "Test User",
		Phone:    "+2348012345678",
	}
	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("POST", "/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockService.AssertExpectations(t)
}

func TestAuthHandler_Register_InvalidJSON(t *testing.T) {
	// Arrange
	mockService := new(MockAuthService)
	handler := NewAuthHandler(mockService)
	router := setupTestRouter()
	router.POST("/register", handler.Register)

	invalidJSON := []byte(`{"email": "test@example.com", "password":`)

	// Act
	req, _ := http.NewRequest("POST", "/register", bytes.NewBuffer(invalidJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusBadRequest, w.Code) // Malformed JSON returns 400, not 422
	mockService.AssertNotCalled(t, "Register")
}

func TestAuthHandler_Login_Success(t *testing.T) {
	// Arrange
	mockService := new(MockAuthService)
	handler := NewAuthHandler(mockService)
	router := setupTestRouter()
	router.POST("/login", handler.Login)

	loginResp := createLoginResponse()
	mockService.On("Login", mock.Anything, mock.AnythingOfType("dto.LoginRequest")).
		Return(loginResp, nil)

	reqBody := dto.LoginRequest{
		Email:    "test@example.com",
		Password: "SecurePass123!",
	}
	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("POST", "/login", bytes.NewBuffer(jsonBody))
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

func TestAuthHandler_Login_InvalidCredentials(t *testing.T) {
	// Arrange
	mockService := new(MockAuthService)
	handler := NewAuthHandler(mockService)
	router := setupTestRouter()
	router.POST("/login", handler.Login)

	mockService.On("Login", mock.Anything, mock.AnythingOfType("dto.LoginRequest")).
		Return(nil, errors.New("invalid credentials"))

	reqBody := dto.LoginRequest{
		Email:    "test@example.com",
		Password: "WrongPassword",
	}
	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("POST", "/login", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockService.AssertExpectations(t)
}

func TestAuthHandler_Login_MissingEmail(t *testing.T) {
	// Arrange
	mockService := new(MockAuthService)
	handler := NewAuthHandler(mockService)
	router := setupTestRouter()
	router.POST("/login", handler.Login)

	reqBody := dto.LoginRequest{
		Email:    "", // Missing email
		Password: "SecurePass123!",
	}
	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("POST", "/login", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	mockService.AssertNotCalled(t, "Login")
}

func TestAuthHandler_RefreshToken_Success(t *testing.T) {
	// Arrange
	mockService := new(MockAuthService)
	handler := NewAuthHandler(mockService)
	router := setupTestRouter()
	router.POST("/refresh", handler.RefreshToken)

	refreshResp := createRefreshResponse()
	mockService.On("RefreshAccessToken", mock.Anything, mock.AnythingOfType("dto.RefreshRequest")).
		Return(refreshResp, nil)

	reqBody := dto.RefreshRequest{
		RefreshToken: "valid-refresh-token",
	}
	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("POST", "/refresh", bytes.NewBuffer(jsonBody))
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

func TestAuthHandler_RefreshToken_InvalidToken(t *testing.T) {
	// Arrange
	mockService := new(MockAuthService)
	handler := NewAuthHandler(mockService)
	router := setupTestRouter()
	router.POST("/refresh", handler.RefreshToken)

	mockService.On("RefreshAccessToken", mock.Anything, mock.AnythingOfType("dto.RefreshRequest")).
		Return(nil, errors.New("invalid refresh token"))

	reqBody := dto.RefreshRequest{
		RefreshToken: "invalid-token",
	}
	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("POST", "/refresh", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockService.AssertExpectations(t)
}

func TestAuthHandler_RefreshToken_MissingToken(t *testing.T) {
	// Arrange
	mockService := new(MockAuthService)
	handler := NewAuthHandler(mockService)
	router := setupTestRouter()
	router.POST("/refresh", handler.RefreshToken)

	reqBody := dto.RefreshRequest{
		RefreshToken: "", // Missing token
	}
	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("POST", "/refresh", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	mockService.AssertNotCalled(t, "RefreshAccessToken")
}

func TestAuthHandler_Logout_Success(t *testing.T) {
	// Arrange
	mockService := new(MockAuthService)
	handler := NewAuthHandler(mockService)
	router := setupTestRouter()
	router.POST("/logout", handler.Logout)

	mockService.On("Logout", mock.Anything, mock.AnythingOfType("string")).
		Return(nil)

	reqBody := dto.RefreshRequest{
		RefreshToken: "valid-refresh-token",
	}
	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("POST", "/logout", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
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

func TestAuthHandler_Logout_MissingRefreshToken(t *testing.T) {
	// Arrange
	mockService := new(MockAuthService)
	handler := NewAuthHandler(mockService)
	router := setupTestRouter()
	router.POST("/logout", handler.Logout)

	reqBody := dto.RefreshRequest{
		RefreshToken: "", // Missing refresh token
	}
	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("POST", "/logout", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	mockService.AssertNotCalled(t, "Logout")
}

func TestAuthHandler_LogoutAll_Success(t *testing.T) {
	// Arrange
	mockService := new(MockAuthService)
	handler := NewAuthHandler(mockService)
	router := setupTestRouter()

	// Simulate authenticated user - middleware sets user_id as string
	testUserID := uuid.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID.String()) // Handler expects string, not UUID
		c.Next()
	})
	router.POST("/logout-all", handler.LogoutAll)

	mockService.On("LogoutAllDevices", mock.Anything, testUserID).
		Return(nil)

	// Act
	req, _ := http.NewRequest("POST", "/logout-all", nil)
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

func TestAuthHandler_LogoutAll_ServiceError(t *testing.T) {
	// Arrange
	mockService := new(MockAuthService)
	handler := NewAuthHandler(mockService)
	router := setupTestRouter()

	testUserID := uuid.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID.String())
		c.Next()
	})
	router.POST("/logout-all", handler.LogoutAll)

	mockService.On("LogoutAllDevices", mock.Anything, testUserID).
		Return(errors.New("database error"))

	// Act
	req, _ := http.NewRequest("POST", "/logout-all", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockService.AssertExpectations(t)
}
