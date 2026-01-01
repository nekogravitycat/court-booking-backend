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
)

func TestRoleMutualExclusionAndUnifiedList(t *testing.T) {
	clearTables()

	// Setup System Admin
	sysAdmin := createTestUser(t, "sysadmin@exclusion.com", "pass", true)
	sysToken := generateToken(sysAdmin.ID)

	// Create Actors
	owner := createTestUser(t, "owner@exclusion.com", "pass", false)
	orgAdmin := createTestUser(t, "orgadmin@exclusion.com", "pass", false)
	locAdmin := createTestUser(t, "locadmin@exclusion.com", "pass", false)

	// Create Org (Assign Owner directly)
	createPayload := orgHttp.CreateOrganizationRequest{
		Name:    "Exclusion Corp",
		OwnerID: owner.ID,
	}
	w := executeRequest("POST", "/v1/organizations", createPayload, sysToken)
	require.Equal(t, http.StatusCreated, w.Code)
	var orgResp orgHttp.OrganizationResponse
	json.Unmarshal(w.Body.Bytes(), &orgResp)
	orgID := orgResp.ID

	ownerToken := generateToken(owner.ID)

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
		w := executeRequest("POST", fmt.Sprintf("/v1/locations/%s/managers", locID), payload, ownerToken)
		assert.Equal(t, http.StatusCreated, w.Code)
	})

	// -------------------------------------------------------------------------
	// Case 2: Try to Add Same User as Org Manager (Failure - Mutual Exclusion)
	// -------------------------------------------------------------------------
	// -------------------------------------------------------------------------
	// Case 2: Try to Add Same User as Org Manager (Failure - Mutual Exclusion)
	// -------------------------------------------------------------------------
	t.Run("Fail Adding LocManager as OrgManager", func(t *testing.T) {
		payload := orgHttp.AddOrganizationManagerRequest{UserID: locAdmin.ID}
		w := executeRequest("POST", fmt.Sprintf("/v1/organizations/%s/managers", orgID), payload, ownerToken)
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	// -------------------------------------------------------------------------
	// Case 3: Add Org Manager (Success)
	// -------------------------------------------------------------------------
	t.Run("Add Org Manager Success", func(t *testing.T) {
		payload := orgHttp.AddOrganizationManagerRequest{UserID: orgAdmin.ID}
		w := executeRequest("POST", fmt.Sprintf("/v1/organizations/%s/managers", orgID), payload, ownerToken)
		assert.Equal(t, http.StatusCreated, w.Code)
	})

	// -------------------------------------------------------------------------
	// Case 4: Try to Add Org Manager as Location Manager (Failure - Mutual Exclusion)
	// -------------------------------------------------------------------------
	t.Run("Fail Adding OrgManager as LocManager", func(t *testing.T) {
		payload := map[string]string{"user_id": orgAdmin.ID}
		w := executeRequest("POST", fmt.Sprintf("/v1/locations/%s/managers", locID), payload, ownerToken)
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	// -------------------------------------------------------------------------
	// Case 5: Try to Add Owner as Location Manager (Failure - Mutual Exclusion)
	// -------------------------------------------------------------------------
	t.Run("Fail Adding Owner as LocManager", func(t *testing.T) {
		payload := map[string]string{"user_id": owner.ID}
		w := executeRequest("POST", fmt.Sprintf("/v1/locations/%s/managers", locID), payload, ownerToken)
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	// -------------------------------------------------------------------------
	// Case 6: ListManagers (Only Managers)
	// -------------------------------------------------------------------------
	t.Run("ListManagers", func(t *testing.T) {
		w := executeRequest("GET", fmt.Sprintf("/v1/organizations/%s/managers", orgID), nil, ownerToken)
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string][]orgHttp.MemberResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		// We expect 1 manager (orgAdmin). Owner is not in this list. LocAdmin is not in this list.
		assert.Equal(t, 1, len(resp["data"]))
		assert.Equal(t, orgAdmin.ID, resp["data"][0].UserID)
	})
	// -------------------------------------------------------------------------
	// Case 7: Fail Adding Owner as Manager (Self-check or API check)
	// -------------------------------------------------------------------------
	t.Run("Fail Adding Owner as Manager", func(t *testing.T) {
		payload := orgHttp.AddOrganizationManagerRequest{UserID: owner.ID}
		w := executeRequest("POST", fmt.Sprintf("/v1/organizations/%s/managers", orgID), payload, ownerToken)
		assert.Equal(t, http.StatusConflict, w.Code)
	})
}
