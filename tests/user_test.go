package tests

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	userHttp "github.com/nekogravitycat/court-booking-backend/internal/user/http"
)

func TestAuthFlow(t *testing.T) {
	clearTables()

	// 1. Test Register (POST /auth/register)
	registerPayload := userHttp.RegisterRequest{
		Email:       "test@example.com",
		Password:    "password123",
		DisplayName: "Tester",
	}
	w := executeRequest("POST", "/v1/auth/register", registerPayload, "")

	assert.Equal(t, http.StatusCreated, w.Code, "Register should succeed")

	// 2. Test Duplicate Register (Should Fail)
	wDuplicate := executeRequest("POST", "/v1/auth/register", registerPayload, "")
	assert.Equal(t, http.StatusConflict, wDuplicate.Code, "Duplicate email should return 409")

	// 3. Test Login (POST /auth/login)
	loginPayload := userHttp.LoginRequest{
		Email:    "test@example.com",
		Password: "password123",
	}
	wLogin := executeRequest("POST", "/v1/auth/login", loginPayload, "")

	// Use require here because if login fails, the rest of the test is meaningless
	require.Equal(t, http.StatusOK, wLogin.Code, "Login should succeed")

	var loginResp userHttp.LoginResponse
	err := json.Unmarshal(wLogin.Body.Bytes(), &loginResp)
	require.NoError(t, err, "Should parse login response")
	assert.NotEmpty(t, loginResp.AccessToken, "Access token should not be empty")

	// 4. Test Me (GET /me)
	wMe := executeRequest("GET", "/v1/me", nil, loginResp.AccessToken)
	assert.Equal(t, http.StatusOK, wMe.Code, "Get Me should succeed")
}

func TestUserManagement_Permissions(t *testing.T) {
	clearTables()

	// Setup: Create one admin and one normal user
	adminUser := createTestUser(t, "admin@example.com", "adminpass", true)
	normalUser := createTestUser(t, "normal@example.com", "userpass", false)

	adminToken := generateTokenHelper(adminUser.ID, adminUser.Email)
	normalToken := generateTokenHelper(normalUser.ID, normalUser.Email)

	// 1. Test List Users (GET /users)
	t.Run("Admin can list users", func(t *testing.T) {
		w := executeRequest("GET", "/v1/users", nil, adminToken)
		assert.Equal(t, http.StatusOK, w.Code)

		// Optional: Check if response structure is correct
		var resp map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.NotNil(t, resp["items"])
	})

	t.Run("Normal user cannot list users", func(t *testing.T) {
		w := executeRequest("GET", "/v1/users", nil, normalToken)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	// 2. Test Get User (GET /users/:id)
	t.Run("Admin can get specific user", func(t *testing.T) {
		path := "/v1/users/" + normalUser.ID
		w := executeRequest("GET", path, nil, adminToken)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	// 3. Test Update User (PATCH /users/:id)
	t.Run("Admin can update user status", func(t *testing.T) {
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
}

func TestUser_NotFound_And_Invalid(t *testing.T) {
	clearTables()
	adminUser := createTestUser(t, "admin@sys.com", "pass", true)
	token := generateTokenHelper(adminUser.ID, adminUser.Email)

	// Test Get Non-existent User
	fakeUUID := "00000000-0000-0000-0000-000000000000"
	w := executeRequest("GET", "/v1/users/"+fakeUUID, nil, token)

	assert.Equal(t, http.StatusNotFound, w.Code, "Should return 404 for non-existent user")

	// Test Invalid UUID format (Assuming API returns 400 or 500 based on parsing logic)
	// Since gin param binding might not catch uuid strict format unless validation is set,
	// we just ensure it doesn't crash (200).
	wInvalid := executeRequest("GET", "/v1/users/not-a-uuid", nil, token)
	assert.NotEqual(t, http.StatusOK, wInvalid.Code, "Should not return 200 for invalid ID")
}
