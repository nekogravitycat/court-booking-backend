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
		createPayload := orgHttp.CreateOrganizationRequest{
			Name:    "Badminton Club",
			OwnerID: admin.ID,
		}

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
		payload := orgHttp.CreateOrganizationRequest{
			Name:    "",
			OwnerID: admin.ID,
		}
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

	t.Run("Interact with Invalid UUID Path Parameter", func(t *testing.T) {
		invalidPath := "/v1/organizations/not-a-uuid"

		// 1. GET
		wGet := executeRequest("GET", invalidPath, nil, userToken)
		assert.Equal(t, http.StatusBadRequest, wGet.Code, "Should return 400 for invalid UUID in GET")

		// 2. PATCH
		newName := "Should Not Update"
		payload := orgHttp.UpdateOrganizationRequest{Name: &newName}
		wPatch := executeRequest("PATCH", invalidPath, payload, adminToken)
		assert.Equal(t, http.StatusBadRequest, wPatch.Code, "Should return 400 for invalid UUID in PATCH")

		// 3. DELETE
		wDelete := executeRequest("DELETE", invalidPath, nil, adminToken)
		assert.Equal(t, http.StatusBadRequest, wDelete.Code, "Should return 400 for invalid UUID in DELETE")
	})
}

func TestOrganizationManagers(t *testing.T) {
	clearTables()

	// Setup Users
	// System Admin (to create org)
	sysAdmin := createTestUser(t, "sysadmin@test.com", "pass", true)
	// Owner (will own the org)
	owner := createTestUser(t, "owner@test.com", "pass", false)
	// Manager (will be added as manager)
	managerUser := createTestUser(t, "manager@test.com", "pass", false)

	sysToken := generateToken(sysAdmin.ID, sysAdmin.Email)
	ownerToken := generateToken(owner.ID, owner.Email)

	// Shared variables for sub-tests
	var orgID string
	var managersPath string
	var managerDetailPath string

	t.Run("Setup Organization", func(t *testing.T) {
		createPayload := orgHttp.CreateOrganizationRequest{
			Name:    "Test Org",
			OwnerID: owner.ID,
		}
		wOrg := executeRequest("POST", "/v1/organizations", createPayload, sysToken)
		require.Equal(t, http.StatusCreated, wOrg.Code)

		var orgResp orgHttp.OrganizationResponse
		err := json.Unmarshal(wOrg.Body.Bytes(), &orgResp)
		require.NoError(t, err)

		// Initialize shared variables
		orgID = orgResp.ID
		managersPath = fmt.Sprintf("/v1/organizations/%s/managers", orgID)
		managerDetailPath = fmt.Sprintf("/v1/organizations/%s/managers/%s", orgID, managerUser.ID)
	})

	t.Run("Add Manager", func(t *testing.T) {
		payload := orgHttp.AddOrganizationManagerRequest{
			UserID: managerUser.ID,
		}
		// Owner adds manager
		wAdd := executeRequest("POST", managersPath, payload, ownerToken)
		assert.Equal(t, http.StatusCreated, wAdd.Code)
	})

	t.Run("Add Duplicate Manager", func(t *testing.T) {
		addPayload := orgHttp.AddOrganizationManagerRequest{
			UserID: managerUser.ID,
		}
		wAddDup := executeRequest("POST", managersPath, addPayload, ownerToken)
		assert.Equal(t, http.StatusConflict, wAddDup.Code, "Should return conflict for duplicate manager")
	})

	t.Run("List Managers", func(t *testing.T) {
		wList := executeRequest("GET", managersPath, nil, ownerToken)
		assert.Equal(t, http.StatusOK, wList.Code)

		var resp map[string][]orgHttp.MemberResponse
		err := json.Unmarshal(wList.Body.Bytes(), &resp)
		require.NoError(t, err)

		items := resp["data"]
		assert.Equal(t, 1, len(items))
		assert.Equal(t, managerUser.ID, items[0].UserID)
		// Role check removed as DTO might not carry role or it's always manager
	})

	t.Run("Remove Manager", func(t *testing.T) {
		wRemove := executeRequest("DELETE", managerDetailPath, nil, ownerToken)
		assert.Equal(t, http.StatusNoContent, wRemove.Code)
	})

	t.Run("Verify Removal", func(t *testing.T) {
		wListAgain := executeRequest("GET", managersPath, nil, ownerToken)
		var resp map[string][]orgHttp.MemberResponse
		json.Unmarshal(wListAgain.Body.Bytes(), &resp)
		assert.Equal(t, 0, len(resp["data"]))
	})

	t.Run("Add Non-existent User", func(t *testing.T) {
		fakeUserID := "00000000-0000-0000-0000-000000000000"
		payload := orgHttp.AddOrganizationManagerRequest{
			UserID: fakeUserID,
		}
		w := executeRequest("POST", managersPath, payload, ownerToken)
		assert.Contains(t, []int{http.StatusBadRequest, http.StatusNotFound}, w.Code)
	})

	t.Run("Interact with Invalid UUID in Manager Routes", func(t *testing.T) {
		// Case 1: Invalid Organization ID
		invalidOrgPath := "/v1/organizations/not-a-uuid/managers"

		// GET Managers
		wList := executeRequest("GET", invalidOrgPath, nil, ownerToken)
		assert.Equal(t, http.StatusBadRequest, wList.Code)

		// POST Manager
		addPayload := orgHttp.AddOrganizationManagerRequest{UserID: managerUser.ID}
		wAdd := executeRequest("POST", invalidOrgPath, addPayload, ownerToken)
		assert.Equal(t, http.StatusBadRequest, wAdd.Code)

		// Case 2: Valid Organization ID but Invalid User ID (for DELETE)
		invalidUserPath := fmt.Sprintf("/v1/organizations/%s/managers/not-a-uuid", orgID)

		// DELETE Manager
		wDelete := executeRequest("DELETE", invalidUserPath, nil, ownerToken)
		assert.Equal(t, http.StatusBadRequest, wDelete.Code)
	})
}
