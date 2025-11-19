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

	adminToken := generateTokenHelper(admin.ID, admin.Email)
	userToken := generateTokenHelper(user.ID, user.Email)

	// 1. Create Organization (POST /organizations)
	createPayload := orgHttp.CreateOrganizationRequest{Name: "Badminton Club"}

	// Normal user -> Forbidden
	wFail := executeRequest("POST", "/v1/organizations", createPayload, userToken)
	assert.Equal(t, http.StatusForbidden, wFail.Code, "Normal user should not create org")

	// Admin -> Created
	wCreate := executeRequest("POST", "/v1/organizations", createPayload, adminToken)
	require.Equal(t, http.StatusCreated, wCreate.Code, "Admin should create org")

	var orgResp orgHttp.OrganizationResponse
	err := json.Unmarshal(wCreate.Body.Bytes(), &orgResp)
	require.NoError(t, err)
	assert.NotEmpty(t, orgResp.ID)
	assert.Equal(t, "Badminton Club", orgResp.Name)

	orgID := orgResp.ID

	// 2. List Organizations (GET /organizations)
	wList := executeRequest("GET", "/v1/organizations", nil, userToken)
	assert.Equal(t, http.StatusOK, wList.Code)

	var listResp response.PageResponse[orgHttp.OrganizationResponse]
	err = json.Unmarshal(wList.Body.Bytes(), &listResp)
	require.NoError(t, err)
	assert.Equal(t, 1, listResp.Total)
	assert.Len(t, listResp.Items, 1)

	// 3. Update Organization (PATCH /organizations/:id)
	newName := "Super Badminton Club"
	updatePayload := orgHttp.UpdateOrganizationRequest{Name: &newName}
	path := fmt.Sprintf("/v1/organizations/%d", orgID)

	wUpdate := executeRequest("PATCH", path, updatePayload, adminToken)
	assert.Equal(t, http.StatusOK, wUpdate.Code)

	// 4. Delete Organization (DELETE /organizations/:id)
	wDelete := executeRequest("DELETE", path, nil, adminToken)
	assert.Equal(t, http.StatusNoContent, wDelete.Code)

	// Verify Soft Delete
	wListAfter := executeRequest("GET", "/v1/organizations", nil, adminToken)
	var listRespAfter response.PageResponse[orgHttp.OrganizationResponse]
	_ = json.Unmarshal(wListAfter.Body.Bytes(), &listRespAfter)
	assert.Equal(t, 0, listRespAfter.Total, "Should not list soft deleted orgs")
}

func TestOrganizationMembers(t *testing.T) {
	clearTables()

	// Setup
	admin := createTestUser(t, "admin@test.com", "pass", true)
	memberUser := createTestUser(t, "member@test.com", "pass", false)
	adminToken := generateTokenHelper(admin.ID, admin.Email)

	// Create Org
	createPayload := orgHttp.CreateOrganizationRequest{Name: "Test Org"}
	wOrg := executeRequest("POST", "/v1/organizations", createPayload, adminToken)

	var orgResp orgHttp.OrganizationResponse
	json.Unmarshal(wOrg.Body.Bytes(), &orgResp)
	orgID := orgResp.ID

	// 1. Add Member
	addPayload := orgHttp.AddMemberRequest{
		UserID: memberUser.ID,
		Role:   "admin",
	}
	addPath := fmt.Sprintf("/v1/organizations/%d/members", orgID)

	wAdd := executeRequest("POST", addPath, addPayload, adminToken)
	assert.Equal(t, http.StatusCreated, wAdd.Code)

	// 2. Add Duplicate Member
	wAddDup := executeRequest("POST", addPath, addPayload, adminToken)
	assert.Equal(t, http.StatusConflict, wAddDup.Code, "Should return conflict for duplicate member")

	// 3. List Members
	wList := executeRequest("GET", addPath, nil, adminToken)
	assert.Equal(t, http.StatusOK, wList.Code)

	var membersResp response.PageResponse[orgHttp.MemberResponse]
	err := json.Unmarshal(wList.Body.Bytes(), &membersResp)
	require.NoError(t, err)

	require.Len(t, membersResp.Items, 1)
	assert.Equal(t, memberUser.ID, membersResp.Items[0].UserID)
	assert.Equal(t, "admin", membersResp.Items[0].Role)

	// 4. Update Member Role
	updateRolePath := fmt.Sprintf("/v1/organizations/%d/members/%s", orgID, memberUser.ID)
	updateRolePayload := orgHttp.UpdateMemberRequest{Role: "owner"}

	wUpdate := executeRequest("PATCH", updateRolePath, updateRolePayload, adminToken)
	assert.Equal(t, http.StatusOK, wUpdate.Code)

	// 5. Remove Member
	wRemove := executeRequest("DELETE", updateRolePath, nil, adminToken)
	assert.Equal(t, http.StatusNoContent, wRemove.Code)

	// Verify Removal
	wListAgain := executeRequest("GET", addPath, nil, adminToken)
	var membersRespAgain response.PageResponse[orgHttp.MemberResponse]
	json.Unmarshal(wListAgain.Body.Bytes(), &membersRespAgain)
	assert.Equal(t, 0, membersRespAgain.Total)
}
