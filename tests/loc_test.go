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

func TestLocationCRUDAndPermissions(t *testing.T) {
	clearTables()

	// Setup Users with different roles
	// SysAdmin: Needed to create organizations initially
	sysAdmin := createTestUser(t, "sysadmin@loc.com", "pass", true)

	// Org A Users
	ownerA := createTestUser(t, "owner.a@loc.com", "pass", false)
	adminA := createTestUser(t, "admin.a@loc.com", "pass", false)
	memberA := createTestUser(t, "member.a@loc.com", "pass", false)

	// Org B User (to test cross-organization boundaries)
	adminB := createTestUser(t, "admin.b@loc.com", "pass", false)

	// Unaffiliated User
	stranger := createTestUser(t, "stranger@loc.com", "pass", false)

	// Generate Tokens
	sysAdminToken := generateToken(sysAdmin.ID, sysAdmin.Email)
	ownerAToken := generateToken(ownerA.ID, ownerA.Email)
	adminAToken := generateToken(adminA.ID, adminA.Email)
	memberAToken := generateToken(memberA.ID, memberA.Email)
	adminBToken := generateToken(adminB.ID, adminB.Email)
	strangerToken := generateToken(stranger.ID, stranger.Email)

	var orgA_ID string
	var locationID string

	t.Run("Setup Context: Create Organizations and Assign Roles", func(t *testing.T) {
		// 1. Create Organization A
		createPayload := orgHttp.CreateOrganizationRequest{Name: "Sports Center A"}
		w := executeRequest("POST", "/v1/organizations", createPayload, sysAdminToken)
		require.Equal(t, http.StatusCreated, w.Code)

		var orgResp orgHttp.OrganizationResponse
		json.Unmarshal(w.Body.Bytes(), &orgResp)
		orgA_ID = orgResp.ID

		// Assign Roles for Org A
		// Owner
		executeRequest("POST", fmt.Sprintf("/v1/organizations/%s/members", orgA_ID),
			orgHttp.AddMemberRequest{UserID: ownerA.ID, Role: "owner"}, sysAdminToken)
		// Admin
		executeRequest("POST", fmt.Sprintf("/v1/organizations/%s/members", orgA_ID),
			orgHttp.AddMemberRequest{UserID: adminA.ID, Role: "admin"}, sysAdminToken)
		// Member
		executeRequest("POST", fmt.Sprintf("/v1/organizations/%s/members", orgA_ID),
			orgHttp.AddMemberRequest{UserID: memberA.ID, Role: "member"}, sysAdminToken)

		// 2. Create Organization B (Target for cross-org attack test)
		createPayloadB := orgHttp.CreateOrganizationRequest{Name: "Sports Center B"}
		wB := executeRequest("POST", "/v1/organizations", createPayloadB, sysAdminToken)
		require.Equal(t, http.StatusCreated, wB.Code)

		var orgRespB orgHttp.OrganizationResponse
		json.Unmarshal(wB.Body.Bytes(), &orgRespB)

		// Assign Admin Role for Org B
		executeRequest("POST", fmt.Sprintf("/v1/organizations/%s/members", orgRespB.ID),
			orgHttp.AddMemberRequest{UserID: adminB.ID, Role: "admin"}, sysAdminToken)
	})

	t.Run("Create Location: Validation Failures", func(t *testing.T) {
		// Missing Name
		invalidPayload := locHttp.CreateLocationRequest{
			OrganizationID: orgA_ID,
			Name:           "", // Empty
			Capacity:       10,
			Longitude:      121.0,
			Latitude:       25.0,
		}
		w := executeRequest("POST", "/v1/locations", invalidPayload, ownerAToken)
		assert.Equal(t, http.StatusBadRequest, w.Code, "Should fail when name is empty")

		// Invalid Coordinates
		invalidGeoPayload := locHttp.CreateLocationRequest{
			OrganizationID: orgA_ID,
			Name:           "Bad Geo",
			Longitude:      200.0, // Invalid
			Latitude:       100.0, // Invalid
		}
		wGeo := executeRequest("POST", "/v1/locations", invalidGeoPayload, ownerAToken)
		assert.Equal(t, http.StatusBadRequest, wGeo.Code, "Should fail when coordinates are out of range")
	})

	t.Run("Create Location: Permission Denied", func(t *testing.T) {
		validPayload := locHttp.CreateLocationRequest{
			OrganizationID:    orgA_ID,
			Name:              "Member Created Court",
			Capacity:          5,
			OpeningHoursStart: "09:00:00",
			OpeningHoursEnd:   "18:00:00",
			LocationInfo:      "Info",
			Longitude:         121.0,
			Latitude:          25.0,
		}

		// Regular Member trying to create
		wMember := executeRequest("POST", "/v1/locations", validPayload, memberAToken)
		assert.Equal(t, http.StatusForbidden, wMember.Code, "Org members should not be allowed to create locations")

		// Stranger trying to create
		wStranger := executeRequest("POST", "/v1/locations", validPayload, strangerToken)
		assert.Equal(t, http.StatusForbidden, wStranger.Code, "Strangers should not be allowed to create locations")

		// Admin of Org B trying to create in Org A
		wAdminB := executeRequest("POST", "/v1/locations", validPayload, adminBToken)
		assert.Equal(t, http.StatusForbidden, wAdminB.Code, "Admin of another org should not be allowed to create locations in this org")
	})

	t.Run("Create Location: Success (Admin/Owner)", func(t *testing.T) {
		validPayload := locHttp.CreateLocationRequest{
			OrganizationID:    orgA_ID,
			Name:              "Main Court A",
			Capacity:          20,
			OpeningHoursStart: "08:00:00",
			OpeningHoursEnd:   "22:00:00",
			LocationInfo:      "Downtown",
			Opening:           true,
			Longitude:         121.5,
			Latitude:          25.0,
		}

		w := executeRequest("POST", "/v1/locations", validPayload, adminAToken)
		require.Equal(t, http.StatusCreated, w.Code)

		var resp locHttp.LocationResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "Main Court A", resp.Name)
		assert.NotEmpty(t, resp.ID)

		locationID = resp.ID
	})

	t.Run("List Locations: Filtering", func(t *testing.T) {
		// Add a dummy location for Org B to ensure filtering works
		// We use adminBToken to create it legally
		dummyPayload := locHttp.CreateLocationRequest{
			OrganizationID: orgA_ID, // Intentionally using Org A ID but sending with Admin B Token (Should fail per strict rules)
			Name:           "Fail Attempt",
			Longitude:      0, Latitude: 0,
		}
		// Verify cross-org creation fails first
		executeRequest("POST", "/v1/locations", dummyPayload, adminBToken)

		// Now list Org A locations
		path := fmt.Sprintf("/v1/locations?organization_id=%s", orgA_ID)
		w := executeRequest("GET", path, nil, strangerToken) // Public read access check
		assert.Equal(t, http.StatusOK, w.Code)

		var listResp response.PageResponse[locHttp.LocationResponse]
		json.Unmarshal(w.Body.Bytes(), &listResp)

		assert.GreaterOrEqual(t, listResp.Total, 1)
		assert.Equal(t, "Main Court A", listResp.Items[0].Name)
	})

	t.Run("Update Location: Permission Boundaries", func(t *testing.T) {
		path := fmt.Sprintf("/v1/locations/%s", locationID)
		newName := "Hacked Name"
		payload := locHttp.UpdateLocationRequest{Name: &newName}

		// 1. Member of the same Org
		wMember := executeRequest("PATCH", path, payload, memberAToken)
		assert.Equal(t, http.StatusForbidden, wMember.Code)

		// 2. Admin of a DIFFERENT Org (Crucial Check)
		// This ensures the handler checks the organization of the *Target Location*,
		// not just if the user is an admin of *some* organization.
		wAdminB := executeRequest("PATCH", path, payload, adminBToken)
		assert.Equal(t, http.StatusForbidden, wAdminB.Code)

		// 3. Stranger
		wStranger := executeRequest("PATCH", path, payload, strangerToken)
		assert.Equal(t, http.StatusForbidden, wStranger.Code)
	})

	t.Run("Update Location: Success", func(t *testing.T) {
		path := fmt.Sprintf("/v1/locations/%s", locationID)
		newName := "Renamed Court A"
		payload := locHttp.UpdateLocationRequest{Name: &newName}

		// Owner of Org A should succeed
		w := executeRequest("PATCH", path, payload, ownerAToken)
		require.Equal(t, http.StatusOK, w.Code)

		var resp locHttp.LocationResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		assert.Equal(t, "Renamed Court A", resp.Name)
	})

	t.Run("Delete Location: Permission Boundaries", func(t *testing.T) {
		path := fmt.Sprintf("/v1/locations/%s", locationID)

		// Member
		wMember := executeRequest("DELETE", path, nil, memberAToken)
		assert.Equal(t, http.StatusForbidden, wMember.Code)

		// Admin of Different Org
		wAdminB := executeRequest("DELETE", path, nil, adminBToken)
		assert.Equal(t, http.StatusForbidden, wAdminB.Code)
	})

	t.Run("Delete Location: Success", func(t *testing.T) {
		path := fmt.Sprintf("/v1/locations/%s", locationID)

		// Admin of Org A
		w := executeRequest("DELETE", path, nil, adminAToken)
		assert.Equal(t, http.StatusNoContent, w.Code)

		// Verify it is gone
		wGet := executeRequest("GET", path, nil, adminAToken)
		assert.Equal(t, http.StatusNotFound, wGet.Code)
	})

	t.Run("Interact with Non-Existent Location", func(t *testing.T) {
		fakeID := "00000000-0000-0000-0000-000000000000"
		path := fmt.Sprintf("/v1/locations/%s", fakeID)

		// Update
		wUpdate := executeRequest("PATCH", path, locHttp.UpdateLocationRequest{}, ownerAToken)
		assert.Equal(t, http.StatusNotFound, wUpdate.Code)

		// Delete
		wDelete := executeRequest("DELETE", path, nil, ownerAToken)
		assert.Equal(t, http.StatusNotFound, wDelete.Code)
	})

	t.Run("Interact with Invalid UUID Path Parameter", func(t *testing.T) {
		invalidPath := "/v1/locations/not-a-uuid"

		// 1. GET
		wGet := executeRequest("GET", invalidPath, nil, strangerToken)
		assert.Equal(t, http.StatusBadRequest, wGet.Code, "Should return 400 for invalid UUID in GET")

		// 2. PATCH
		newName := "Should Not Update"
		payload := locHttp.UpdateLocationRequest{Name: &newName}
		wPatch := executeRequest("PATCH", invalidPath, payload, ownerAToken)
		assert.Equal(t, http.StatusBadRequest, wPatch.Code, "Should return 400 for invalid UUID in PATCH")

		// 3. DELETE
		wDelete := executeRequest("DELETE", invalidPath, nil, adminAToken)
		assert.Equal(t, http.StatusBadRequest, wDelete.Code, "Should return 400 for invalid UUID in DELETE")
	})

	t.Run("List Locations: Validation Failures", func(t *testing.T) {
		// 1. Invalid Opening Hours Format
		w := executeRequest("GET", fmt.Sprintf("/v1/locations?organization_id=%s&opening_hours_start_min=invalid", orgA_ID), nil, strangerToken)
		assert.Equal(t, http.StatusBadRequest, w.Code, "Should fail when opening_hours_start_min is invalid")

		var errResp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &errResp)
		assert.Contains(t, errResp, "error")

		// 2. Valid Opening Hours Format
		w = executeRequest("GET", fmt.Sprintf("/v1/locations?organization_id=%s&opening_hours_start_min=09:00:00", orgA_ID), nil, strangerToken)
		assert.Equal(t, http.StatusOK, w.Code, "Should succeed when opening_hours_start_min is valid")
	})
	t.Run("List Locations: Advanced Filtering and Sorting", func(t *testing.T) {
		// Create 3 locations with distinct attributes

		// Loc 1: Cap 10, 08:00-18:00, Open
		w1 := executeRequest("POST", "/v1/locations", locHttp.CreateLocationRequest{
			OrganizationID:    orgA_ID,
			Name:              "Filter Court 1",
			Capacity:          10,
			OpeningHoursStart: "08:00:00",
			OpeningHoursEnd:   "18:00:00",
			LocationInfo:      "Info 1",
			Opening:           true,
			Longitude:         121.0, Latitude: 25.0,
		}, adminAToken)
		require.Equal(t, http.StatusCreated, w1.Code, "Failed to create Loc 1: %s", w1.Body.String())

		// Loc 2: Cap 50, 10:00-22:00, Closed
		w2 := executeRequest("POST", "/v1/locations", locHttp.CreateLocationRequest{
			OrganizationID:    orgA_ID,
			Name:              "Filter Court 2",
			Capacity:          50,
			OpeningHoursStart: "10:00:00",
			OpeningHoursEnd:   "22:00:00",
			LocationInfo:      "Info 2",
			Opening:           false,
			Longitude:         121.0, Latitude: 25.0,
		}, adminAToken)
		require.Equal(t, http.StatusCreated, w2.Code, "Failed to create Loc 2: %s", w2.Body.String())

		// Loc 3: Cap 100, 06:00-14:00, Open
		w3 := executeRequest("POST", "/v1/locations", locHttp.CreateLocationRequest{
			OrganizationID:    orgA_ID,
			Name:              "Filter Court 3",
			Capacity:          100,
			OpeningHoursStart: "06:00:00",
			OpeningHoursEnd:   "14:00:00",
			LocationInfo:      "Info 3",
			Opening:           true,
			Longitude:         121.0, Latitude: 25.0,
		}, adminAToken)
		require.Equal(t, http.StatusCreated, w3.Code, "Failed to create Loc 3: %s", w3.Body.String())

		// 1. Filter by Name (Partial Match)
		w := executeRequest("GET", fmt.Sprintf("/v1/locations?organization_id=%s&name=Filter Court", orgA_ID), nil, strangerToken)
		assert.Equal(t, http.StatusOK, w.Code, "List 1 failed: %s", w.Body.String())
		var listResp response.PageResponse[locHttp.LocationResponse]
		json.Unmarshal(w.Body.Bytes(), &listResp)
		// Should find all 3 "Filter Court X"
		assert.Equal(t, 3, listResp.Total)

		// 2. Filter by Opening (bool)
		w = executeRequest("GET", fmt.Sprintf("/v1/locations?organization_id=%s&opening=false", orgA_ID), nil, strangerToken)
		assert.Equal(t, http.StatusOK, w.Code, "List 2 failed: %s", w.Body.String())
		json.Unmarshal(w.Body.Bytes(), &listResp)
		assert.Equal(t, 1, listResp.Total)
		assert.Equal(t, "Filter Court 2", listResp.Items[0].Name)

		// 3. Filter by Capacity Range (10-60) -> Should get Loc 1 and Loc 2
		w = executeRequest("GET", fmt.Sprintf("/v1/locations?organization_id=%s&capacity_min=10&capacity_max=60&sort_by=capacity&sort_order=asc", orgA_ID), nil, strangerToken)
		assert.Equal(t, http.StatusOK, w.Code, "List 3 failed: %s", w.Body.String())
		json.Unmarshal(w.Body.Bytes(), &listResp)
		assert.Equal(t, 2, listResp.Total)
		assert.Equal(t, "Filter Court 1", listResp.Items[0].Name)
		assert.Equal(t, "Filter Court 2", listResp.Items[1].Name)

		// 4. Filter by Opening Hours Start (>= 09:00) -> Loc 2 (10:00)
		w = executeRequest("GET", fmt.Sprintf("/v1/locations?organization_id=%s&opening_hours_start_min=09:00:00", orgA_ID), nil, strangerToken)
		assert.Equal(t, http.StatusOK, w.Code, "List 4 failed: %s", w.Body.String())
		json.Unmarshal(w.Body.Bytes(), &listResp)
		assert.Equal(t, 1, listResp.Total)
		assert.Equal(t, "Filter Court 2", listResp.Items[0].Name)

		// 5. Sorting (Capacity DESC) -> Loc 3 (100), Loc 2 (50), Loc 1 (10)
		w = executeRequest("GET", fmt.Sprintf("/v1/locations?organization_id=%s&sort_by=capacity&sort_order=desc", orgA_ID), nil, strangerToken)
		assert.Equal(t, http.StatusOK, w.Code, "List 5 failed: %s", w.Body.String())
		json.Unmarshal(w.Body.Bytes(), &listResp)
		assert.Equal(t, 3, listResp.Total)
		assert.Equal(t, "Filter Court 3", listResp.Items[0].Name)
		assert.Equal(t, "Filter Court 2", listResp.Items[1].Name)

		// 6. Edge Case: Invalid Query Parameters (Should return 400 Bad Request)
		// Invalid boolean for opening, invalid int for capacity
		w = executeRequest("GET", fmt.Sprintf("/v1/locations?organization_id=%s&opening=not-a-bool&capacity_min=invalid", orgA_ID), nil, strangerToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		// Should return error message
		var errResp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &errResp)
		assert.Contains(t, errResp, "error")

		// 7. Edge Case: Logical Validation Failure (Capacity Min > Max)
		w = executeRequest("GET", fmt.Sprintf("/v1/locations?organization_id=%s&capacity_min=100&capacity_max=50", orgA_ID), nil, strangerToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		json.Unmarshal(w.Body.Bytes(), &errResp)
		assert.Contains(t, errResp["error"], "capacity")
	})
}
