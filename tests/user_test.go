package tests

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
	userHttp "github.com/nekogravitycat/court-booking-backend/internal/user/http"
)

func TestAuthFlow(t *testing.T) {
	clearTables()

	// Variable shared between sub-tests
	var accessToken string

	t.Run("Register User", func(t *testing.T) {
		registerPayload := userHttp.RegisterRequest{
			Email:       "test@example.com",
			Password:    "password123",
			DisplayName: "Tester",
		}
		w := executeRequest("POST", "/v1/auth/register", registerPayload, "")
		assert.Equal(t, http.StatusCreated, w.Code, "Register should succeed")
	})

	t.Run("Duplicate Register", func(t *testing.T) {
		registerPayload := userHttp.RegisterRequest{
			Email:       "test@example.com",
			Password:    "password123",
			DisplayName: "Tester",
		}
		wDuplicate := executeRequest("POST", "/v1/auth/register", registerPayload, "")
		assert.Equal(t, http.StatusConflict, wDuplicate.Code, "Duplicate email should return 409")
	})

	t.Run("Login", func(t *testing.T) {
		loginPayload := userHttp.LoginRequest{
			Email:    "test@example.com",
			Password: "password123",
		}
		wLogin := executeRequest("POST", "/v1/auth/login", loginPayload, "")

		// Use require because we need the token for the next step
		require.Equal(t, http.StatusOK, wLogin.Code, "Login should succeed")

		var loginResp userHttp.LoginResponse
		err := json.Unmarshal(wLogin.Body.Bytes(), &loginResp)
		require.NoError(t, err, "Should parse login response")
		assert.NotEmpty(t, loginResp.AccessToken, "Access token should not be empty")

		// Save token for next step
		accessToken = loginResp.AccessToken
	})

	t.Run("Get Current User", func(t *testing.T) {
		wMe := executeRequest("GET", "/v1/me", nil, accessToken)
		assert.Equal(t, http.StatusOK, wMe.Code, "Get Me should succeed")
	})

	t.Run("Login with Wrong Password", func(t *testing.T) {
		payload := userHttp.LoginRequest{
			Email:    "test@example.com",
			Password: "wrongpassword",
		}
		w := executeRequest("POST", "/v1/auth/login", payload, "")
		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 for wrong password")
	})

	t.Run("Login with Non-existent Email", func(t *testing.T) {
		payload := userHttp.LoginRequest{
			Email:    "ghost@example.com",
			Password: "password123",
		}
		w := executeRequest("POST", "/v1/auth/login", payload, "")
		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 for non-existent user")
	})

	t.Run("Get Me with Invalid Token", func(t *testing.T) {
		w := executeRequest("GET", "/v1/me", nil, "invalid-token-string")
		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 for invalid token")
	})
}

func TestUserManagementPermissions(t *testing.T) {
	clearTables()

	// Setup: Create one admin and one normal user
	adminUser := createTestUser(t, "admin@example.com", "adminpass", true)
	normalUser := createTestUser(t, "normal@example.com", "userpass", false)

	adminToken := generateToken(adminUser.ID, adminUser.Email)
	normalToken := generateToken(normalUser.ID, normalUser.Email)

	t.Run("Admin List Users", func(t *testing.T) {
		w := executeRequest("GET", "/v1/users", nil, adminToken)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.NotNil(t, resp["items"])
	})

	t.Run("Normal User List Users", func(t *testing.T) {
		w := executeRequest("GET", "/v1/users", nil, normalToken)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("Admin Get Specific User", func(t *testing.T) {
		path := "/v1/users/" + normalUser.ID
		w := executeRequest("GET", path, nil, adminToken)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Admin Update User Status", func(t *testing.T) {
		path := "/v1/users/" + normalUser.ID
		isActive := false
		updatePayload := userHttp.UpdateUserBody{
			IsActive: &isActive,
		}
		w := executeRequest("PATCH", path, updatePayload, adminToken)
		require.Equal(t, http.StatusOK, w.Code)

		var resp userHttp.MeResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		// Verify the logic
		assert.False(t, resp.User.IsActive, "User should be inactive after update")
	})

	t.Run("Admin List Users Filtered", func(t *testing.T) {
		url := "/v1/users?email=admin@example.com"
		w := executeRequest("GET", url, nil, adminToken)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp response.PageResponse[userHttp.UserResponse]
		json.Unmarshal(w.Body.Bytes(), &resp)

		// Should only return 1 admin user
		assert.Equal(t, 1, resp.Total)
		assert.Equal(t, "admin@example.com", resp.Items[0].Email)
	})
}

func TestUserNotFoundAndInvalidInput(t *testing.T) {
	clearTables()
	adminUser := createTestUser(t, "admin@sys.com", "pass", true)
	token := generateToken(adminUser.ID, adminUser.Email)

	t.Run("Get Non-existent User", func(t *testing.T) {
		fakeUUID := "00000000-0000-0000-0000-000000000000"
		w := executeRequest("GET", "/v1/users/"+fakeUUID, nil, token)
		assert.Equal(t, http.StatusNotFound, w.Code, "Should return 404 for non-existent user")
	})

	t.Run("Interact with Invalid UUID", func(t *testing.T) {
		invalidPath := "/v1/users/not-a-uuid"

		// 1. GET
		wGet := executeRequest("GET", invalidPath, nil, token)
		assert.Equal(t, http.StatusBadRequest, wGet.Code, "Should return 400 for invalid UUID in GET")

		// 2. PATCH
		isActive := false
		payload := userHttp.UpdateUserBody{IsActive: &isActive}
		wPatch := executeRequest("PATCH", invalidPath, payload, token)
		assert.Equal(t, http.StatusBadRequest, wPatch.Code, "Should return 400 for invalid UUID in PATCH")
	})
}
