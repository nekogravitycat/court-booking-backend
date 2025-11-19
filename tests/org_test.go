package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	orgHttp "github.com/nekogravitycat/court-booking-backend/internal/organization/http"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
)

func TestOrganizationCRUD(t *testing.T) {
	clearTables()

	admin := createTestUser(t, "admin@org.com", "pass", true)
	user := createTestUser(t, "user@org.com", "pass", false)

	adminToken := generateToken(admin.ID, admin.Email)
	userToken := generateToken(user.ID, user.Email)

	// Define orgID in the outer scope to share it between sub-tests
	var orgID string

	t.Run("Create Organization", func(t *testing.T) {
		createPayload := orgHttp.CreateOrganizationRequest{Name: "Badminton Club"}

		// Case A: Normal user -> Forbidden
		wFail := executeRequest("POST", "/v1/organizations", createPayload, userToken)
		assert.Equal(t, http.StatusForbidden, wFail.Code, "Normal user should not be able to create an organization")

		// Case B: Admin -> Created
		wCreate := executeRequest("POST", "/v1/organizations", createPayload, adminToken)
		require.Equal(t, http.StatusCreated, wCreate.Code, "Admin should be able to create an organization")

		var orgResp orgHttp.OrganizationResponse
		err := json.Unmarshal(wCreate.Body.Bytes(), &orgResp)
		require.NoError(t, err)
		assert.NotEmpty(t, orgResp.ID)
		assert.Equal(t, "Badminton Club", orgResp.Name)

		// Assign the ID to the outer variable for subsequent tests
		orgID = orgResp.ID
	})

	t.Run("List Organizations", func(t *testing.T) {
		wList := executeRequest("GET", "/v1/organizations", nil, userToken)
		assert.Equal(t, http.StatusOK, wList.Code)

		var listResp response.PageResponse[orgHttp.OrganizationResponse]
		err := json.Unmarshal(wList.Body.Bytes(), &listResp)
		require.NoError(t, err)
		assert.Equal(t, 1, listResp.Total)
		assert.Len(t, listResp.Items, 1)
	})

	// Moved before Delete/Update logic to ensure the resource exists and is active
	t.Run("Get Single Organization", func(t *testing.T) {
		path := fmt.Sprintf("/v1/organizations/%s", orgID)
		w := executeRequest("GET", path, nil, userToken) // Normal user can read
		assert.Equal(t, http.StatusOK, w.Code)

		var orgResp orgHttp.OrganizationResponse
		json.Unmarshal(w.Body.Bytes(), &orgResp)
		assert.Equal(t, orgID, orgResp.ID)
	})

	t.Run("Update Organization", func(t *testing.T) {
		newName := "Super Badminton Club"
		updatePayload := orgHttp.UpdateOrganizationRequest{Name: &newName}
		path := fmt.Sprintf("/v1/organizations/%s", orgID)

		wUpdate := executeRequest("PATCH", path, updatePayload, adminToken)
		assert.Equal(t, http.StatusOK, wUpdate.Code)
	})

	t.Run("Delete Organization", func(t *testing.T) {
		path := fmt.Sprintf("/v1/organizations/%s", orgID)
		wDelete := executeRequest("DELETE", path, nil, adminToken)
		assert.Equal(t, http.StatusNoContent, wDelete.Code)
	})

	t.Run("Verify Soft Delete", func(t *testing.T) {
		wListAfter := executeRequest("GET", "/v1/organizations", nil, adminToken)
		var listRespAfter response.PageResponse[orgHttp.OrganizationResponse]
		_ = json.Unmarshal(wListAfter.Body.Bytes(), &listRespAfter)
		assert.Equal(t, 0, listRespAfter.Total, "Should not list soft deleted organizations")
	})

	t.Run("Create Organization Validation (Empty Name)", func(t *testing.T) {
		payload := orgHttp.CreateOrganizationRequest{Name: ""}
		w := executeRequest("POST", "/v1/organizations", payload, adminToken)
		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 for empty name")
	})

	t.Run("Normal User Cannot Update Organization", func(t *testing.T) {
		newName := "Hacker Club"
		payload := orgHttp.UpdateOrganizationRequest{Name: &newName}
		path := fmt.Sprintf("/v1/organizations/%s", orgID)

		w := executeRequest("PATCH", path, payload, userToken)
		assert.Equal(t, http.StatusForbidden, w.Code, "Normal user cannot update org")
	})

	t.Run("Normal User Cannot Delete Organization", func(t *testing.T) {
		path := fmt.Sprintf("/v1/organizations/%s", orgID)
		w := executeRequest("DELETE", path, nil, userToken)
		assert.Equal(t, http.StatusForbidden, w.Code, "Normal user cannot delete org")
	})

	t.Run("Update Non-existent Organization", func(t *testing.T) {
		fakeID := "00000000-0000-0000-0000-000000000000"
		newName := "Ghost"
		payload := orgHttp.UpdateOrganizationRequest{Name: &newName}

		w := executeRequest("PATCH", "/v1/organizations/"+fakeID, payload, adminToken)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestOrganizationMembers(t *testing.T) {
	clearTables()

	// Setup Users
	admin := createTestUser(t, "admin@test.com", "pass", true)
	memberUser := createTestUser(t, "member@test.com", "pass", false)
	adminToken := generateToken(admin.ID, admin.Email)

	// Shared variables for sub-tests
	var orgID string
	var membersPath string
	var memberDetailPath string

	t.Run("Setup Organization", func(t *testing.T) {
		createPayload := orgHttp.CreateOrganizationRequest{Name: "Test Org"}
		wOrg := executeRequest("POST", "/v1/organizations", createPayload, adminToken)
		require.Equal(t, http.StatusCreated, wOrg.Code)

		var orgResp orgHttp.OrganizationResponse
		err := json.Unmarshal(wOrg.Body.Bytes(), &orgResp)
		require.NoError(t, err)

		// Initialize shared variables
		orgID = orgResp.ID
		membersPath = fmt.Sprintf("/v1/organizations/%s/members", orgID)
		memberDetailPath = fmt.Sprintf("/v1/organizations/%s/members/%s", orgID, memberUser.ID)
	})

	t.Run("Add Member", func(t *testing.T) {
		addPayload := orgHttp.AddMemberRequest{
			UserID: memberUser.ID,
			Role:   "admin",
		}
		wAdd := executeRequest("POST", membersPath, addPayload, adminToken)
		assert.Equal(t, http.StatusCreated, wAdd.Code)
	})

	t.Run("Add Duplicate Member", func(t *testing.T) {
		addPayload := orgHttp.AddMemberRequest{
			UserID: memberUser.ID,
			Role:   "admin",
		}
		wAddDup := executeRequest("POST", membersPath, addPayload, adminToken)
		assert.Equal(t, http.StatusConflict, wAddDup.Code, "Should return conflict for duplicate member")
	})

	t.Run("List Members", func(t *testing.T) {
		wList := executeRequest("GET", membersPath, nil, adminToken)
		assert.Equal(t, http.StatusOK, wList.Code)

		var membersResp response.PageResponse[orgHttp.MemberResponse]
		err := json.Unmarshal(wList.Body.Bytes(), &membersResp)
		require.NoError(t, err)

		require.Len(t, membersResp.Items, 1)
		assert.Equal(t, memberUser.ID, membersResp.Items[0].UserID)
		assert.Equal(t, "admin", membersResp.Items[0].Role)
	})

	t.Run("Update Member Role", func(t *testing.T) {
		updateRolePayload := orgHttp.UpdateMemberRequest{Role: "owner"}
		wUpdate := executeRequest("PATCH", memberDetailPath, updateRolePayload, adminToken)
		assert.Equal(t, http.StatusOK, wUpdate.Code)
	})

	t.Run("Remove Member", func(t *testing.T) {
		wRemove := executeRequest("DELETE", memberDetailPath, nil, adminToken)
		assert.Equal(t, http.StatusNoContent, wRemove.Code)
	})

	t.Run("Verify Removal", func(t *testing.T) {
		wListAgain := executeRequest("GET", membersPath, nil, adminToken)
		var membersRespAgain response.PageResponse[orgHttp.MemberResponse]
		json.Unmarshal(wListAgain.Body.Bytes(), &membersRespAgain)
		assert.Equal(t, 0, membersRespAgain.Total)
	})

	t.Run("Add Non-existent User", func(t *testing.T) {
		fakeUserID := "00000000-0000-0000-0000-000000000000"
		payload := orgHttp.AddMemberRequest{
			UserID: fakeUserID,
			Role:   "member",
		}
		w := executeRequest("POST", membersPath, payload, adminToken)
		// Should be 400 Bad Request or 404 Not Found depending on implementation
		assert.Contains(t, []int{http.StatusBadRequest, http.StatusNotFound}, w.Code)
	})

	t.Run("Add Member with Invalid Role", func(t *testing.T) {
		payload := orgHttp.AddMemberRequest{
			UserID: memberUser.ID, // Re-use existing user but bad role
			Role:   "super_admin", // Invalid role
		}
		w := executeRequest("POST", membersPath, payload, adminToken)
		assert.Equal(t, http.StatusBadRequest, w.Code, "Should validate role enum")
	})
}
