package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	locHttp "github.com/nekogravitycat/court-booking-backend/internal/location/http"
	orgHttp "github.com/nekogravitycat/court-booking-backend/internal/organization/http"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
)

func TestRoleMutualExclusionAndUnifiedList(t *testing.T) {
	clearTables()

	// Setup System Admin
	sysAdmin := createTestUser(t, "sysadmin@exclusion.com", "pass", true)
	sysToken := generateToken(sysAdmin.ID, sysAdmin.Email)

	// Create Org
	createPayload := orgHttp.CreateOrganizationRequest{Name: "Exclusion Corp"}
	w := executeRequest("POST", "/v1/organizations", createPayload, sysToken)
	require.Equal(t, http.StatusCreated, w.Code)
	var orgResp orgHttp.OrganizationResponse
	json.Unmarshal(w.Body.Bytes(), &orgResp)
	orgID := orgResp.ID

	// Create Actors
	owner := createTestUser(t, "owner@exclusion.com", "pass", false)
	orgAdmin := createTestUser(t, "orgadmin@exclusion.com", "pass", false)
	locAdmin := createTestUser(t, "locadmin@exclusion.com", "pass", false)

	// Assign Owner directly to DB (Bypassing API restriction if any)
	addMemberToOrg(t, orgID, owner.ID, "owner")
	ownerToken := generateToken(owner.ID, owner.Email)

	// Create Location
	locPayload := locHttp.CreateLocationRequest{
		OrganizationID:    orgID,
		Name:              "Restricted Area",
		Capacity:          5,
		OpeningHoursStart: "09:00",
		OpeningHoursEnd:   "18:00",
		LocationInfo:      "Restricted",
		Opening:           true,
		Longitude:         10,
		Latitude:          10,
	}
	wLoc := executeRequest("POST", "/v1/locations", locPayload, ownerToken)
	require.Equal(t, http.StatusCreated, wLoc.Code, "Owner should be able to create location")
	var locResp locHttp.LocationResponse
	json.Unmarshal(wLoc.Body.Bytes(), &locResp)
	locID := locResp.ID

	// -------------------------------------------------------------------------
	// Case 1: Add Location Manager (Success)
	// -------------------------------------------------------------------------
	t.Run("Add Location Manager Success", func(t *testing.T) {
		payload := map[string]string{"user_id": locAdmin.ID}
		w := executeRequest("POST", fmt.Sprintf("/v1/locations/%s/admins", locID), payload, ownerToken)
		assert.Equal(t, http.StatusCreated, w.Code)
	})

	// -------------------------------------------------------------------------
	// Case 2: Try to Add Same User as Org Manager (Failure - Mutual Exclusion)
	// -------------------------------------------------------------------------
	t.Run("Fail Adding LocManager as OrgManager", func(t *testing.T) {
		payload := orgHttp.AddMemberRequest{UserID: locAdmin.ID, Role: "manager"}
		w := executeRequest("POST", fmt.Sprintf("/v1/organizations/%s/members", orgID), payload, ownerToken)
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	// -------------------------------------------------------------------------
	// Case 3: Add Org Manager (Success)
	// -------------------------------------------------------------------------
	t.Run("Add Org Manager Success", func(t *testing.T) {
		payload := orgHttp.AddMemberRequest{UserID: orgAdmin.ID, Role: "manager"}
		w := executeRequest("POST", fmt.Sprintf("/v1/organizations/%s/members", orgID), payload, ownerToken)
		assert.Equal(t, http.StatusCreated, w.Code)
	})

	// -------------------------------------------------------------------------
	// Case 4: Try to Add Org Manager as Location Manager (Failure - Mutual Exclusion)
	// -------------------------------------------------------------------------
	t.Run("Fail Adding OrgManager as LocManager", func(t *testing.T) {
		payload := map[string]string{"user_id": orgAdmin.ID}
		w := executeRequest("POST", fmt.Sprintf("/v1/locations/%s/admins", locID), payload, ownerToken)
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	// -------------------------------------------------------------------------
	// Case 5: Try to Add Owner as Location Manager (Failure - Mutual Exclusion)
	// -------------------------------------------------------------------------
	t.Run("Fail Adding Owner as LocManager", func(t *testing.T) {
		payload := map[string]string{"user_id": owner.ID}
		w := executeRequest("POST", fmt.Sprintf("/v1/locations/%s/admins", locID), payload, ownerToken)
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	// -------------------------------------------------------------------------
	// Case 6: Unified ListMembers
	// -------------------------------------------------------------------------
	t.Run("Unified ListMembers", func(t *testing.T) {
		w := executeRequest("GET", fmt.Sprintf("/v1/organizations/%s/members", orgID), nil, ownerToken)
		require.Equal(t, http.StatusOK, w.Code)

		var resp response.PageResponse[orgHttp.MemberResponse]
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		// We expect 3 members: Owner, OrgManager, LocManager
		assert.Equal(t, 3, resp.Total)

		roleMap := make(map[string]int)
		for _, m := range resp.Items {
			roleMap[m.Role]++
		}

		assert.Equal(t, 1, roleMap["owner"], "Should have 1 owner")
		assert.Equal(t, 1, roleMap["manager"], "Should have 1 manager")
		// The role returned for location manager should be 'location_manager' as per repository query
		assert.Equal(t, 1, roleMap["location_manager"], "Should have 1 location_manager")
	})
	// -------------------------------------------------------------------------
	// Case 7: Fail Adding User as Owner (API Restriction)
	// -------------------------------------------------------------------------
	t.Run("Fail Adding Owner Role via API", func(t *testing.T) {
		newUser := createTestUser(t, "new_owner@exclusion.com", "pass", false)
		payload := orgHttp.AddMemberRequest{UserID: newUser.ID, Role: "owner"}
		w := executeRequest("POST", fmt.Sprintf("/v1/organizations/%s/members", orgID), payload, ownerToken)
		assert.Equal(t, http.StatusBadRequest, w.Code, "Should strictly disallow adding 'owner' via API")
	})
}
