package tests

import (
	"context"
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
		updatePayload := userHttp.UpdateUserRequest{
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
		payload := userHttp.UpdateUserRequest{IsActive: &isActive}
		wPatch := executeRequest("PATCH", invalidPath, payload, token)
		assert.Equal(t, http.StatusBadRequest, wPatch.Code, "Should return 400 for invalid UUID in PATCH")
	})
}

func TestUserOrganizationResponse(t *testing.T) {
	clearTables()

	// Setup: Create users
	// Admin user to test GetByID permissions
	adminUser := createTestUser(t, "admin@check.com", "pass", true)
	adminToken := generateToken(adminUser.ID, adminUser.Email)

	// Target user whose organizations we will verify
	targetUser := createTestUser(t, "target@check.com", "pass", false)
	targetToken := generateToken(targetUser.ID, targetUser.Email)

	// User with zero organizations (edge case)
	lonelyUser := createTestUser(t, "lonely@check.com", "pass", false)
	lonelyToken := generateToken(lonelyUser.ID, lonelyUser.Email)

	// Setup: Create organizations directly in DB
	orgA_ID := createTestOrganization(t, "Badminton Club A", true)
	orgB_ID := createTestOrganization(t, "Tennis Club B", true)
	orgInactive_ID := createTestOrganization(t, "Closed Club", false) // Inactive organization

	// Setup: Add targetUser to organizations
	// Add to active Org A
	addMemberToOrg(t, orgA_ID, targetUser.ID, "member")
	// Add to active Org B
	addMemberToOrg(t, orgB_ID, targetUser.ID, "admin")
	// Add to inactive Org (should be filtered out)
	addMemberToOrg(t, orgInactive_ID, targetUser.ID, "member")

	// Test Case: Check /me endpoint (should include multiple organizations and filter inactive ones)
	t.Run("Get Me Includes Active Organizations Only", func(t *testing.T) {
		w := executeRequest("GET", "/v1/me", nil, targetToken)
		require.Equal(t, http.StatusOK, w.Code)

		var resp userHttp.MeResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		// Verify User ID
		assert.Equal(t, targetUser.ID, resp.User.ID)

		// Verify Organization list length
		// Should have exactly 2 active organizations (OrgA, OrgB). OrgInactive must be excluded.
		require.Len(t, resp.User.Organizations, 2, "Should have exactly 2 active organizations")

		// Verify content (order is not guaranteed, so we check existence)
		orgNames := []string{}
		for _, org := range resp.User.Organizations {
			orgNames = append(orgNames, org.Name)
			assert.NotEmpty(t, org.ID)
		}
		assert.Contains(t, orgNames, "Badminton Club A")
		assert.Contains(t, orgNames, "Tennis Club B")
		assert.NotContains(t, orgNames, "Closed Club", "Inactive organization should not be listed")
	})

	// Test Case: Check Admin viewing a specific user (GET /users/:id)
	t.Run("Admin Get User Includes Organizations", func(t *testing.T) {
		path := "/v1/users/" + targetUser.ID
		w := executeRequest("GET", path, nil, adminToken)
		require.Equal(t, http.StatusOK, w.Code)

		var resp userHttp.MeResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		require.Len(t, resp.User.Organizations, 2)
		// Check if one of the expected orgs is present
		found := false
		for _, o := range resp.User.Organizations {
			if o.Name == "Badminton Club A" {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected organization not found in admin view")
	})

	// Test Case: Check User List (GET /users) with JSON Aggregation
	t.Run("List Users Includes Organizations Field", func(t *testing.T) {
		// Filter by email to isolate the target user
		url := "/v1/users?email=target@check.com"
		w := executeRequest("GET", url, nil, adminToken)
		require.Equal(t, http.StatusOK, w.Code)

		var resp response.PageResponse[userHttp.UserResponse]
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		require.Equal(t, 1, resp.Total)
		userItem := resp.Items[0]

		// Verify the list item contains the nested organization data
		require.Len(t, userItem.Organizations, 2)
		// Verify one of the organizations exists
		assert.NotEmpty(t, userItem.Organizations[0].Name)
	})

	// Test Case: Edge Case - User with no organizations
	t.Run("User With No Organizations Returns Empty Array", func(t *testing.T) {
		w := executeRequest("GET", "/v1/me", nil, lonelyToken)
		require.Equal(t, http.StatusOK, w.Code)

		var resp userHttp.MeResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		// It should not be nil; it should be an empty slice (depending on COALESCE implementation)
		assert.NotNil(t, resp.User.Organizations)
		assert.Len(t, resp.User.Organizations, 0)
	})
}

func TestDeleteUser(t *testing.T) {
	clearTables()

	// Setup: Create actors
	// 1. Admin user (Authorized to delete)
	adminUser := createTestUser(t, "admin@delete.com", "adminpass", true)
	adminToken := generateToken(adminUser.ID, adminUser.Email)

	// 2. Normal user (Unauthorized to delete)
	normalUser := createTestUser(t, "normal@delete.com", "userpass", false)
	normalToken := generateToken(normalUser.ID, normalUser.Email)

	// 3. Victim user (The one to be deleted)
	victimUser := createTestUser(t, "victim@delete.com", "victimpass", false)

	t.Run("Delete Without Auth", func(t *testing.T) {
		path := "/v1/users/" + victimUser.ID
		w := executeRequest("DELETE", path, nil, "")
		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 when no token is provided")
	})

	t.Run("Delete With Normal User Token", func(t *testing.T) {
		path := "/v1/users/" + victimUser.ID
		w := executeRequest("DELETE", path, nil, normalToken)
		assert.Equal(t, http.StatusForbidden, w.Code, "Should return 403 when a normal user tries to delete")
	})

	t.Run("Delete With Invalid UUID", func(t *testing.T) {
		w := executeRequest("DELETE", "/v1/users/invalid-uuid-string", nil, adminToken)
		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 for invalid UUID format")
	})

	t.Run("Delete Non-existent User", func(t *testing.T) {
		fakeUUID := "00000000-0000-0000-0000-000000000000"
		w := executeRequest("DELETE", "/v1/users/"+fakeUUID, nil, adminToken)
		assert.Equal(t, http.StatusNotFound, w.Code, "Should return 404 when deleting a non-existent user")
	})

	t.Run("Delete Success (Soft Delete)", func(t *testing.T) {
		path := "/v1/users/" + victimUser.ID
		w := executeRequest("DELETE", path, nil, adminToken)

		// 1. Check HTTP Status
		require.Equal(t, http.StatusNoContent, w.Code, "Should return 204 No Content on success")

		// 2. Verify Database State (via API)
		// We fetch the user again to ensure 'is_active' is now false.
		// Note: We use the admin token to fetch, as the user might be blocked from logging in.
		wGet := executeRequest("GET", path, nil, adminToken)
		require.Equal(t, http.StatusOK, wGet.Code)

		var resp userHttp.MeResponse
		err := json.Unmarshal(wGet.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Equal(t, victimUser.ID, resp.User.ID)
		assert.False(t, resp.User.IsActive, "User should be marked as inactive (soft deleted)")
	})

	t.Run("Delete Idempotency (Delete Again)", func(t *testing.T) {
		// Even if the user is already inactive, the DELETE operation should succeed (204).
		// This ensures the client doesn't get an error if they retry the request due to network issues.
		path := "/v1/users/" + victimUser.ID
		w := executeRequest("DELETE", path, nil, adminToken)
		assert.Equal(t, http.StatusNoContent, w.Code, "Repeated delete should still return 204")
	})

	t.Run("Deleted User Cannot Login", func(t *testing.T) {
		// Attempt to login with the user credentials that were just deleted
		loginPayload := userHttp.LoginRequest{
			Email:    "victim@delete.com", // The email of victimUser
			Password: "victimpass",        // The password we set during setup
		}

		w := executeRequest("POST", "/v1/auth/login", loginPayload, "")

		// Expect 401 Unauthorized because the user is inactive
		assert.Equal(t, http.StatusUnauthorized, w.Code, "Soft deleted user should not be able to login")

		// Optional: Verify the error message doesn't leak that the user exists but is inactive
		// It should be the generic "invalid email or password"
		var resp map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "invalid email or password", resp["error"])
	})
}

// -------------------------------------------------------------------
// Helper Functions
// -------------------------------------------------------------------

// createTestOrganization inserts a dummy organization directly into the database.
// Note: Ensure 'dbPool' or your global test database variable is accessible here.
func createTestOrganization(t *testing.T, name string, isActive bool) string {
	var id string
	query := `INSERT INTO organizations (name, is_active) VALUES ($1, $2) RETURNING id`

	err := testPool.QueryRow(context.Background(), query, name, isActive).Scan(&id)
	require.NoError(t, err, "Failed to create test organization")
	return id
}

// addMemberToOrg inserts a record into organization_permissions directly.
func addMemberToOrg(t *testing.T, orgID, userID, role string) {
	query := `INSERT INTO organization_permissions (organization_id, user_id, role) VALUES ($1, $2, $3)`

	_, err := testPool.Exec(context.Background(), query, orgID, userID, role)
	require.NoError(t, err, "Failed to add member to org")
}
