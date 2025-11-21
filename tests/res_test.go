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
	resHttp "github.com/nekogravitycat/court-booking-backend/internal/resource/http"
	rtHttp "github.com/nekogravitycat/court-booking-backend/internal/resourcetype/http"
)

func TestResourceCRUDAndPermissions(t *testing.T) {
	clearTables()

	// ==== Setup Users & Tokens ====
	sysAdmin := createTestUser(t, "sysadmin@res.com", "pass", true)

	// Org A Users (Primary context)
	ownerA := createTestUser(t, "owner.a@res.com", "pass", false)
	adminA := createTestUser(t, "admin.a@res.com", "pass", false)
	memberA := createTestUser(t, "member.a@res.com", "pass", false)

	// Org B User (For cross-organization isolation tests)
	adminB := createTestUser(t, "admin.b@res.com", "pass", false)

	// Unaffiliated User
	stranger := createTestUser(t, "stranger@res.com", "pass", false)

	// Generate Tokens
	sysAdminToken := generateToken(sysAdmin.ID, sysAdmin.Email)
	ownerAToken := generateToken(ownerA.ID, ownerA.Email)
	adminAToken := generateToken(adminA.ID, adminA.Email)
	memberAToken := generateToken(memberA.ID, memberA.Email)
	adminBToken := generateToken(adminB.ID, adminB.Email)
	strangerToken := generateToken(stranger.ID, stranger.Email)

	// Shared ID variables
	var orgA_ID, orgB_ID string
	var locA_ID string
	var rtA_ID, rtB_ID string
	var resourceID string

	// ==== Setup Prerequisites (Orgs, Locs, RTs) ====
	t.Run("Setup Prerequisites", func(t *testing.T) {
		// Create Organization A & B
		wOrgA := executeRequest("POST", "/v1/organizations", orgHttp.CreateOrganizationRequest{Name: "Org A"}, sysAdminToken)
		var orgRespA orgHttp.OrganizationResponse
		json.Unmarshal(wOrgA.Body.Bytes(), &orgRespA)
		orgA_ID = orgRespA.ID

		wOrgB := executeRequest("POST", "/v1/organizations", orgHttp.CreateOrganizationRequest{Name: "Org B"}, sysAdminToken)
		var orgRespB orgHttp.OrganizationResponse
		json.Unmarshal(wOrgB.Body.Bytes(), &orgRespB)
		orgB_ID = orgRespB.ID

		// Assign Roles
		executeRequest("POST", fmt.Sprintf("/v1/organizations/%s/members", orgA_ID),
			orgHttp.AddMemberRequest{UserID: ownerA.ID, Role: "owner"}, sysAdminToken)
		executeRequest("POST", fmt.Sprintf("/v1/organizations/%s/members", orgA_ID),
			orgHttp.AddMemberRequest{UserID: adminA.ID, Role: "admin"}, sysAdminToken)
		executeRequest("POST", fmt.Sprintf("/v1/organizations/%s/members", orgA_ID),
			orgHttp.AddMemberRequest{UserID: memberA.ID, Role: "member"}, sysAdminToken)
		executeRequest("POST", fmt.Sprintf("/v1/organizations/%s/members", orgB_ID),
			orgHttp.AddMemberRequest{UserID: adminB.ID, Role: "admin"}, sysAdminToken)

		// Create Locations (Loc A in Org A, Loc B in Org B)
		locPayloadA := locHttp.CreateLocationRequest{
			OrganizationID: orgA_ID, Name: "Loc A", Capacity: 10,
			OpeningHoursStart: "09:00:00", OpeningHoursEnd: "22:00:00",
			LocationInfo: "Info A", Longitude: 121.0, Latitude: 25.0,
		}
		wLocA := executeRequest("POST", "/v1/locations", locPayloadA, adminAToken)
		var locRespA locHttp.LocationResponse
		json.Unmarshal(wLocA.Body.Bytes(), &locRespA)
		locA_ID = locRespA.ID

		locPayloadB := locHttp.CreateLocationRequest{
			OrganizationID: orgB_ID, Name: "Loc B", Capacity: 10,
			OpeningHoursStart: "09:00:00", OpeningHoursEnd: "22:00:00",
			LocationInfo: "Info B", Longitude: 121.0, Latitude: 25.0,
		}
		wLocB := executeRequest("POST", "/v1/locations", locPayloadB, adminBToken)
		var locRespB locHttp.LocationResponse
		json.Unmarshal(wLocB.Body.Bytes(), &locRespB)

		// Create Resource Types (RT A in Org A, RT B in Org B)
		rtPayloadA := rtHttp.CreateRequest{OrganizationID: orgA_ID, Name: "Type A"}
		wRTA := executeRequest("POST", "/v1/resource-types", rtPayloadA, adminAToken)
		var rtRespA rtHttp.ResourceTypeResponse
		json.Unmarshal(wRTA.Body.Bytes(), &rtRespA)
		rtA_ID = rtRespA.ID

		rtPayloadB := rtHttp.CreateRequest{OrganizationID: orgB_ID, Name: "Type B"}
		wRTB := executeRequest("POST", "/v1/resource-types", rtPayloadB, adminBToken)
		var rtRespB rtHttp.ResourceTypeResponse
		json.Unmarshal(wRTB.Body.Bytes(), &rtRespB)
		rtB_ID = rtRespB.ID
	})

	// ==== Input Validation Tests (Bad Request) ====
	t.Run("Create Resource: Input Validation", func(t *testing.T) {
		// Case: Missing Name (Binding validation)
		invalidPayload := resHttp.CreateRequest{
			Name:           "",
			LocationID:     locA_ID,
			ResourceTypeID: rtA_ID,
		}
		w := executeRequest("POST", "/v1/resources", invalidPayload, adminAToken)
		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 when required name is empty")

		// Case: Invalid UUIDs (Binding validation)
		invalidIDsPayload := map[string]interface{}{
			"name":             "Test Court",
			"location_id":      "not-a-uuid",
			"resource_type_id": rtA_ID,
		}
		wIDs := executeRequest("POST", "/v1/resources", invalidIDsPayload, adminAToken)
		assert.Equal(t, http.StatusBadRequest, wIDs.Code, "Should return 400 for invalid UUID format")

		// Case: Invalid JSON Types
		invalidTypePayload := map[string]int{
			"name": 12345,
		}
		wType := executeRequest("POST", "/v1/resources", invalidTypePayload, adminAToken)
		assert.Equal(t, http.StatusBadRequest, wType.Code, "Should return 400 for JSON type mismatch")
	})

	t.Run("Create Resource: Business Logic Consistency", func(t *testing.T) {
		// Attempt to create a resource linking Location A (Org A) and Resource Type B (Org B).
		// This is a logical error, not just a formatting error.
		// The server should catch this inconsistency and return 400 Bad Request, not 500.
		mismatchPayload := resHttp.CreateRequest{
			Name:           "Inconsistent Resource",
			LocationID:     locA_ID, // Org A
			ResourceTypeID: rtB_ID,  // Org B
		}

		w := executeRequest("POST", "/v1/resources", mismatchPayload, adminAToken)

		// Verify we get a 400 Bad Request
		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 for organization mismatch between Location and ResourceType")

		var errResp map[string]string
		json.Unmarshal(w.Body.Bytes(), &errResp)
		assert.Contains(t, errResp["error"], "must belong to the same organization", "Error message should explain the mismatch")
	})

	t.Run("Interact with Invalid UUID Path Parameter", func(t *testing.T) {
		invalidPath := "/v1/resources/not-a-uuid"

		// GET
		wGet := executeRequest("GET", invalidPath, nil, memberAToken)
		assert.Equal(t, http.StatusBadRequest, wGet.Code, "Should return 400 for invalid UUID in GET")

		// PATCH
		newName := "Ignored"
		payload := resHttp.UpdateRequest{Name: &newName}
		wPatch := executeRequest("PATCH", invalidPath, payload, adminAToken)
		assert.Equal(t, http.StatusBadRequest, wPatch.Code, "Should return 400 for invalid UUID in PATCH")

		// DELETE
		wDelete := executeRequest("DELETE", invalidPath, nil, adminAToken)
		assert.Equal(t, http.StatusBadRequest, wDelete.Code, "Should return 400 for invalid UUID in DELETE")
	})

	// ==== Permission Control Tests ====
	t.Run("Create Resource: Permission Denied", func(t *testing.T) {
		validPayload := resHttp.CreateRequest{
			Name:           "Court 1",
			LocationID:     locA_ID,
			ResourceTypeID: rtA_ID,
		}

		// 1. Regular Member of Org A -> Forbidden
		wMember := executeRequest("POST", "/v1/resources", validPayload, memberAToken)
		assert.Equal(t, http.StatusForbidden, wMember.Code, "Member should not be allowed to create resources")

		// 2. Stranger -> Forbidden
		wStranger := executeRequest("POST", "/v1/resources", validPayload, strangerToken)
		assert.Equal(t, http.StatusForbidden, wStranger.Code, "Stranger should not be allowed to create resources")

		// 3. Admin of Org B (Cross-Organization Attack) -> Forbidden
		// Admin B tries to create a resource attached to Location A (Org A).
		wAdminB := executeRequest("POST", "/v1/resources", validPayload, adminBToken)
		assert.Equal(t, http.StatusForbidden, wAdminB.Code, "Admin of another org should not be allowed to create resources in this org")
	})

	// ==== Happy Path (Success Cases) ====
	t.Run("Create Resource: Success", func(t *testing.T) {
		validPayload := resHttp.CreateRequest{
			Name:           "Badminton Court 1",
			LocationID:     locA_ID,
			ResourceTypeID: rtA_ID,
		}

		// Owner of Org A should succeed
		w := executeRequest("POST", "/v1/resources", validPayload, ownerAToken)
		require.Equal(t, http.StatusCreated, w.Code)

		var resp resHttp.ResourceResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "Badminton Court 1", resp.Name)
		assert.Equal(t, locA_ID, resp.LocationID)
		assert.NotEmpty(t, resp.ID)

		resourceID = resp.ID
	})

	t.Run("List Resources", func(t *testing.T) {
		// Filter by Location
		path := fmt.Sprintf("/v1/resources?location_id=%s", locA_ID)
		w := executeRequest("GET", path, nil, strangerToken) // Public/Authenticated read access
		assert.Equal(t, http.StatusOK, w.Code)

		var listResp response.PageResponse[resHttp.ResourceResponse]
		json.Unmarshal(w.Body.Bytes(), &listResp)
		assert.GreaterOrEqual(t, listResp.Total, 1)
		assert.Equal(t, "Badminton Court 1", listResp.Items[0].Name)

		// Filter by Resource Type
		pathRT := fmt.Sprintf("/v1/resources?resource_type_id=%s", rtA_ID)
		wRT := executeRequest("GET", pathRT, nil, memberAToken)
		assert.Equal(t, http.StatusOK, wRT.Code)
	})

	t.Run("Get Resource", func(t *testing.T) {
		path := fmt.Sprintf("/v1/resources/%s", resourceID)
		w := executeRequest("GET", path, nil, memberAToken)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp resHttp.ResourceResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		assert.Equal(t, resourceID, resp.ID)
	})

	// ==== Update & Delete Tests ====
	t.Run("Update Resource: Permission Boundaries", func(t *testing.T) {
		path := fmt.Sprintf("/v1/resources/%s", resourceID)
		newName := "Hacked Name"
		payload := resHttp.UpdateRequest{Name: &newName}

		// 1. Member of Org A -> Forbidden
		wMember := executeRequest("PATCH", path, payload, memberAToken)
		assert.Equal(t, http.StatusForbidden, wMember.Code, "Member cannot update resource")

		// 2. Admin of Org B -> Forbidden
		// Handlers must check the Org ID of the resource being updated
		wAdminB := executeRequest("PATCH", path, payload, adminBToken)
		assert.Equal(t, http.StatusForbidden, wAdminB.Code, "Admin of different org cannot update resource")
	})

	t.Run("Update Resource: Business Logic Validation", func(t *testing.T) {
		path := fmt.Sprintf("/v1/resources/%s", resourceID)

		// Case: Empty Name (Business logic violation)
		// Even if the binding permits it (via pointer), the Service layer might forbid empty strings.
		// This should return 400, not 500.
		emptyName := ""
		invalidPayload := resHttp.UpdateRequest{Name: &emptyName}
		wInvalid := executeRequest("PATCH", path, invalidPayload, adminAToken)
		assert.Equal(t, http.StatusBadRequest, wInvalid.Code, "Should return 400 when updating with empty name")
	})

	t.Run("Update Resource: Success", func(t *testing.T) {
		path := fmt.Sprintf("/v1/resources/%s", resourceID)
		newName := "Renamed Court 1"
		validPayload := resHttp.UpdateRequest{Name: &newName}

		wSuccess := executeRequest("PATCH", path, validPayload, adminAToken)
		assert.Equal(t, http.StatusOK, wSuccess.Code)

		var resp resHttp.ResourceResponse
		json.Unmarshal(wSuccess.Body.Bytes(), &resp)
		assert.Equal(t, "Renamed Court 1", resp.Name)
	})

	t.Run("Delete Resource: Permission Boundaries", func(t *testing.T) {
		path := fmt.Sprintf("/v1/resources/%s", resourceID)

		// 1. Member -> Forbidden
		wMember := executeRequest("DELETE", path, nil, memberAToken)
		assert.Equal(t, http.StatusForbidden, wMember.Code)

		// 2. Admin of Different Org -> Forbidden
		wAdminB := executeRequest("DELETE", path, nil, adminBToken)
		assert.Equal(t, http.StatusForbidden, wAdminB.Code)
	})

	t.Run("Delete Resource: Success", func(t *testing.T) {
		path := fmt.Sprintf("/v1/resources/%s", resourceID)

		// Admin of Org A
		w := executeRequest("DELETE", path, nil, adminAToken)
		assert.Equal(t, http.StatusNoContent, w.Code)

		// Verify Deletion (Get should return 404)
		wGet := executeRequest("GET", path, nil, adminAToken)
		assert.Equal(t, http.StatusNotFound, wGet.Code)
	})

	// ==== Not Found Edge Cases ====
	t.Run("Interact with Non-Existent Resource", func(t *testing.T) {
		fakeID := "00000000-0000-0000-0000-000000000000"
		path := fmt.Sprintf("/v1/resources/%s", fakeID)

		// Get
		wGet := executeRequest("GET", path, nil, adminAToken)
		assert.Equal(t, http.StatusNotFound, wGet.Code)

		// Update
		newName := "Ghost"
		payload := resHttp.UpdateRequest{Name: &newName}
		wUpdate := executeRequest("PATCH", path, payload, adminAToken)
		assert.Equal(t, http.StatusNotFound, wUpdate.Code)

		// Delete
		wDelete := executeRequest("DELETE", path, nil, adminAToken)
		assert.Equal(t, http.StatusNotFound, wDelete.Code)
	})
}
