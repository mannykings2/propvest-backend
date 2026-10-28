package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
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

// MockUserService is a mock implementation of services.UserService
type MockUserService struct {
	mock.Mock
}

func (m *MockUserService) GetProfile(ctx context.Context, userID uuid.UUID) (*dto.UserResponse, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.UserResponse), args.Error(1)
}

func (m *MockUserService) UpdateProfile(ctx context.Context, userID uuid.UUID, req dto.UpdateProfileRequest) (*dto.UserResponse, error) {
	args := m.Called(ctx, userID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.UserResponse), args.Error(1)
}

func (m *MockUserService) UploadAvatar(ctx context.Context, userID uuid.UUID, file *multipart.FileHeader) (*dto.UserResponse, error) {
	args := m.Called(ctx, userID, file)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.UserResponse), args.Error(1)
}

func (m *MockUserService) ChangePassword(ctx context.Context, userID uuid.UUID, req dto.ChangePasswordRequest) error {
	args := m.Called(ctx, userID, req)
	return args.Error(0)
}

func (m *MockUserService) RequestPhoneChange(ctx context.Context, userID uuid.UUID, req dto.RequestPhoneChangeRequest) (*dto.PhoneChangeResponse, error) {
	args := m.Called(ctx, userID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.PhoneChangeResponse), args.Error(1)
}

func (m *MockUserService) VerifyPhoneChange(ctx context.Context, userID uuid.UUID, req dto.VerifyPhoneChangeRequest) (*dto.UserResponse, error) {
	args := m.Called(ctx, userID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.UserResponse), args.Error(1)
}

func (m *MockUserService) SubmitKYC(ctx context.Context, userID uuid.UUID, req dto.SubmitKYCRequest) (*dto.KYCStatusResponse, error) {
	args := m.Called(ctx, userID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.KYCStatusResponse), args.Error(1)
}

func (m *MockUserService) GetKYCStatus(ctx context.Context, userID uuid.UUID) (*dto.KYCStatusResponse, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.KYCStatusResponse), args.Error(1)
}

// Helper function to create test user response
func createTestUserResponse() *dto.UserResponse {
	avatarURL := "https://example.com/avatar.jpg"
	return &dto.UserResponse{
		ID:            uuid.New(),
		UserCode:      "USR-12345",
		FullName:      "Test User",
		Email:         "test@example.com",
		Phone:         "+2348012345678",
		AvatarURL:     &avatarURL,
		EmailVerified: true,
		KYCStatus:     "verified",
		Role:          "user",
		IsActive:      true,
		CreatedAt:     time.Now(),
	}
}

// ============================================================================
// GetProfile Tests
// ============================================================================

func TestUserHandler_GetProfile_Success(t *testing.T) {
	// Arrange
	mockService := new(MockUserService)
	handler := NewUserHandler(mockService)
	router := setupTestRouter()

	testUserID := uuid.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID.String())
		c.Next()
	})
	router.GET("/profile", handler.GetProfile)

	userResp := createTestUserResponse()
	mockService.On("GetProfile", mock.Anything, testUserID).
		Return(userResp, nil)

	// Act
	req, _ := http.NewRequest("GET", "/profile", nil)
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

func TestUserHandler_GetProfile_Unauthorized(t *testing.T) {
	// Arrange
	mockService := new(MockUserService)
	handler := NewUserHandler(mockService)
	router := setupTestRouter()

	// No authentication middleware - user_id not set
	router.GET("/profile", handler.GetProfile)

	// Act
	req, _ := http.NewRequest("GET", "/profile", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.False(t, response["success"].(bool))

	mockService.AssertNotCalled(t, "GetProfile")
}

func TestUserHandler_GetProfile_UserNotFound(t *testing.T) {
	// Arrange
	mockService := new(MockUserService)
	handler := NewUserHandler(mockService)
	router := setupTestRouter()

	testUserID := uuid.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID.String())
		c.Next()
	})
	router.GET("/profile", handler.GetProfile)

	mockService.On("GetProfile", mock.Anything, testUserID).
		Return(nil, errors.New("user not found"))

	// Act
	req, _ := http.NewRequest("GET", "/profile", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockService.AssertExpectations(t)
}

// ============================================================================
// UpdateProfile Tests
// ============================================================================

func TestUserHandler_UpdateProfile_Success(t *testing.T) {
	// Arrange
	mockService := new(MockUserService)
	handler := NewUserHandler(mockService)
	router := setupTestRouter()

	testUserID := uuid.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID.String())
		c.Next()
	})
	router.PATCH("/profile", handler.UpdateProfile)

	userResp := createTestUserResponse()
	userResp.FullName = "Updated Name"
	
	reqBody := dto.UpdateProfileRequest{
		FullName: "Updated Name",
	}
	
	mockService.On("UpdateProfile", mock.Anything, testUserID, reqBody).
		Return(userResp, nil)

	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("PATCH", "/profile", bytes.NewBuffer(jsonBody))
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

func TestUserHandler_UpdateProfile_ValidationError_EmptyName(t *testing.T) {
	// Arrange
	mockService := new(MockUserService)
	handler := NewUserHandler(mockService)
	router := setupTestRouter()

	testUserID := uuid.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID.String())
		c.Next()
	})
	router.PATCH("/profile", handler.UpdateProfile)

	reqBody := dto.UpdateProfileRequest{
		FullName: "", // Empty name
	}
	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("PATCH", "/profile", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	mockService.AssertNotCalled(t, "UpdateProfile")
}

func TestUserHandler_UpdateProfile_ValidationError_NameTooShort(t *testing.T) {
	// Arrange
	mockService := new(MockUserService)
	handler := NewUserHandler(mockService)
	router := setupTestRouter()

	testUserID := uuid.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID.String())
		c.Next()
	})
	router.PATCH("/profile", handler.UpdateProfile)

	reqBody := dto.UpdateProfileRequest{
		FullName: "A", // Too short (min 2)
	}
	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("PATCH", "/profile", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	mockService.AssertNotCalled(t, "UpdateProfile")
}

func TestUserHandler_UpdateProfile_Unauthorized(t *testing.T) {
	// Arrange
	mockService := new(MockUserService)
	handler := NewUserHandler(mockService)
	router := setupTestRouter()

	// No authentication middleware
	router.PATCH("/profile", handler.UpdateProfile)

	reqBody := dto.UpdateProfileRequest{
		FullName: "New Name",
	}
	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("PATCH", "/profile", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	mockService.AssertNotCalled(t, "UpdateProfile")
}

// ============================================================================
// ChangePassword Tests
// ============================================================================

func TestUserHandler_ChangePassword_Success(t *testing.T) {
	// Arrange
	mockService := new(MockUserService)
	handler := NewUserHandler(mockService)
	router := setupTestRouter()

	testUserID := uuid.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID.String())
		c.Next()
	})
	router.PATCH("/password", handler.ChangePassword)

	reqBody := dto.ChangePasswordRequest{
		CurrentPassword: "OldPassword123!",
		NewPassword:     "NewSecurePassword123!",
	}

	mockService.On("ChangePassword", mock.Anything, testUserID, reqBody).
		Return(nil)

	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("PATCH", "/password", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.True(t, response["success"].(bool))
	assert.Contains(t, response["message"].(string), "Password changed successfully")

	mockService.AssertExpectations(t)
}

func TestUserHandler_ChangePassword_ValidationError_MissingCurrentPassword(t *testing.T) {
	// Arrange
	mockService := new(MockUserService)
	handler := NewUserHandler(mockService)
	router := setupTestRouter()

	testUserID := uuid.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID.String())
		c.Next()
	})
	router.PATCH("/password", handler.ChangePassword)

	reqBody := dto.ChangePasswordRequest{
		CurrentPassword: "", // Missing
		NewPassword:     "NewSecurePassword123!",
	}
	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("PATCH", "/password", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	mockService.AssertNotCalled(t, "ChangePassword")
}

func TestUserHandler_ChangePassword_ValidationError_NewPasswordTooShort(t *testing.T) {
	// Arrange
	mockService := new(MockUserService)
	handler := NewUserHandler(mockService)
	router := setupTestRouter()

	testUserID := uuid.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID.String())
		c.Next()
	})
	router.PATCH("/password", handler.ChangePassword)

	reqBody := dto.ChangePasswordRequest{
		CurrentPassword: "OldPassword123!",
		NewPassword:     "short", // Too short (min 12)
	}
	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("PATCH", "/password", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	mockService.AssertNotCalled(t, "ChangePassword")
}

func TestUserHandler_ChangePassword_WrongCurrentPassword(t *testing.T) {
	// Arrange
	mockService := new(MockUserService)
	handler := NewUserHandler(mockService)
	router := setupTestRouter()

	testUserID := uuid.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID.String())
		c.Next()
	})
	router.PATCH("/password", handler.ChangePassword)

	reqBody := dto.ChangePasswordRequest{
		CurrentPassword: "WrongPassword123!",
		NewPassword:     "NewSecurePassword123!",
	}

	mockService.On("ChangePassword", mock.Anything, testUserID, reqBody).
		Return(errors.New("incorrect current password"))

	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("PATCH", "/password", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockService.AssertExpectations(t)
}

func TestUserHandler_ChangePassword_Unauthorized(t *testing.T) {
	// Arrange
	mockService := new(MockUserService)
	handler := NewUserHandler(mockService)
	router := setupTestRouter()

	// No authentication middleware
	router.PATCH("/password", handler.ChangePassword)

	reqBody := dto.ChangePasswordRequest{
		CurrentPassword: "OldPassword123!",
		NewPassword:     "NewSecurePassword123!",
	}
	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("PATCH", "/password", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	mockService.AssertNotCalled(t, "ChangePassword")
}

// ============================================================================
// UploadAvatar Tests
// ============================================================================

func TestUserHandler_UploadAvatar_NoFile(t *testing.T) {
	// Arrange
	mockService := new(MockUserService)
	handler := NewUserHandler(mockService)
	router := setupTestRouter()

	testUserID := uuid.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID.String())
		c.Next()
	})
	router.PATCH("/avatar", handler.UploadAvatar)

	// Act - No multipart form
	req, _ := http.NewRequest("PATCH", "/avatar", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.False(t, response["success"].(bool))
	assert.Contains(t, response["message"].(string), "No file uploaded")

	mockService.AssertNotCalled(t, "UploadAvatar")
}

func TestUserHandler_UploadAvatar_Success(t *testing.T) {
	// Arrange
	mockService := new(MockUserService)
	handler := NewUserHandler(mockService)
	router := setupTestRouter()

	testUserID := uuid.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID.String())
		c.Next()
	})
	router.PATCH("/avatar", handler.UploadAvatar)

	userResp := createTestUserResponse()
	newAvatarURL := "https://example.com/new-avatar.jpg"
	userResp.AvatarURL = &newAvatarURL

	mockService.On("UploadAvatar", mock.Anything, testUserID, mock.AnythingOfType("*multipart.FileHeader")).
		Return(userResp, nil)

	// Create multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("avatar", "test.jpg")
	part.Write([]byte("fake image data"))
	writer.Close()

	// Act
	req, _ := http.NewRequest("PATCH", "/avatar", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
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

func TestUserHandler_UploadAvatar_Unauthorized(t *testing.T) {
	// Arrange
	mockService := new(MockUserService)
	handler := NewUserHandler(mockService)
	router := setupTestRouter()

	// No authentication middleware
	router.PATCH("/avatar", handler.UploadAvatar)

	// Create multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("avatar", "test.jpg")
	part.Write([]byte("fake image data"))
	writer.Close()

	// Act
	req, _ := http.NewRequest("PATCH", "/avatar", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	mockService.AssertNotCalled(t, "UploadAvatar")
}

// ============================================================================
// RequestPhoneChange Tests
// ============================================================================

func TestUserHandler_RequestPhoneChange_Success(t *testing.T) {
	// Arrange
	mockService := new(MockUserService)
	handler := NewUserHandler(mockService)
	router := setupTestRouter()

	testUserID := uuid.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID.String())
		c.Next()
	})
	router.POST("/phone/request", handler.RequestPhoneChange)

	reqBody := dto.RequestPhoneChangeRequest{
		NewPhone: "+2348098765432",
	}

	phoneResp := &dto.PhoneChangeResponse{
		Message:        "OTP sent to new phone number",
		ExpiresIn:      300,
		CanResendAfter: 60,
	}

	mockService.On("RequestPhoneChange", mock.Anything, testUserID, reqBody).
		Return(phoneResp, nil)

	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("POST", "/phone/request", bytes.NewBuffer(jsonBody))
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

func TestUserHandler_RequestPhoneChange_ValidationError_MissingPhone(t *testing.T) {
	// Arrange
	mockService := new(MockUserService)
	handler := NewUserHandler(mockService)
	router := setupTestRouter()

	testUserID := uuid.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID.String())
		c.Next()
	})
	router.POST("/phone/request", handler.RequestPhoneChange)

	reqBody := dto.RequestPhoneChangeRequest{
		NewPhone: "", // Missing
	}
	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("POST", "/phone/request", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	mockService.AssertNotCalled(t, "RequestPhoneChange")
}

// ============================================================================
// VerifyPhoneChange Tests
// ============================================================================

func TestUserHandler_VerifyPhoneChange_Success(t *testing.T) {
	// Arrange
	mockService := new(MockUserService)
	handler := NewUserHandler(mockService)
	router := setupTestRouter()

	testUserID := uuid.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID.String())
		c.Next()
	})
	router.POST("/phone/verify", handler.VerifyPhoneChange)

	reqBody := dto.VerifyPhoneChangeRequest{
		NewPhone: "+2348098765432",
		OTPCode:  "123456",
	}

	userResp := createTestUserResponse()
	userResp.Phone = "+2348098765432"

	mockService.On("VerifyPhoneChange", mock.Anything, testUserID, reqBody).
		Return(userResp, nil)

	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("POST", "/phone/verify", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.True(t, response["success"].(bool))
	assert.Contains(t, response["message"].(string), "Phone number updated successfully")
	assert.NotNil(t, response["data"])

	mockService.AssertExpectations(t)
}

func TestUserHandler_VerifyPhoneChange_ValidationError_InvalidOTPLength(t *testing.T) {
	// Arrange
	mockService := new(MockUserService)
	handler := NewUserHandler(mockService)
	router := setupTestRouter()

	testUserID := uuid.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID.String())
		c.Next()
	})
	router.POST("/phone/verify", handler.VerifyPhoneChange)

	reqBody := dto.VerifyPhoneChangeRequest{
		NewPhone: "+2348098765432",
		OTPCode:  "123", // Wrong length (must be 6)
	}
	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("POST", "/phone/verify", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	mockService.AssertNotCalled(t, "VerifyPhoneChange")
}

func TestUserHandler_VerifyPhoneChange_InvalidOTPCode(t *testing.T) {
	// Arrange
	mockService := new(MockUserService)
	handler := NewUserHandler(mockService)
	router := setupTestRouter()

	testUserID := uuid.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID.String())
		c.Next()
	})
	router.POST("/phone/verify", handler.VerifyPhoneChange)

	reqBody := dto.VerifyPhoneChangeRequest{
		NewPhone: "+2348098765432",
		OTPCode:  "999999", // Wrong OTP
	}

	mockService.On("VerifyPhoneChange", mock.Anything, testUserID, reqBody).
		Return(nil, errors.New("invalid OTP code"))

	jsonBody, _ := json.Marshal(reqBody)

	// Act
	req, _ := http.NewRequest("POST", "/phone/verify", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockService.AssertExpectations(t)
}
