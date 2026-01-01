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
	rtHttp "github.com/nekogravitycat/court-booking-backend/internal/resourcetype/http"
)

func TestResourceTypeCRUDAndPermissions(t *testing.T) {
	clearTables()

	// Setup Users with different roles
	sysAdmin := createTestUser(t, "sysadmin@rt.com", "pass", true)

	// Org A Users (The primary organization for testing)
	ownerA := createTestUser(t, "owner.a@rt.com", "pass", false)
	adminA := createTestUser(t, "admin.a@rt.com", "pass", false)
	memberA := createTestUser(t, "member.a@rt.com", "pass", false)

	// Org B User (To test cross-organization boundaries / isolation)
	adminB := createTestUser(t, "admin.b@rt.com", "pass", false)

	// Unaffiliated User (To test public/authenticated access boundaries)
	stranger := createTestUser(t, "stranger@rt.com", "pass", false)

	// Generate Tokens
	sysAdminToken := generateToken(sysAdmin.ID, sysAdmin.Email)
	ownerAToken := generateToken(ownerA.ID, ownerA.Email)
	adminAToken := generateToken(adminA.ID, adminA.Email)
	memberAToken := generateToken(memberA.ID, memberA.Email)
	adminBToken := generateToken(adminB.ID, adminB.Email)
	strangerToken := generateToken(stranger.ID, stranger.Email)

	// Shared variables across sub-tests
	var orgA_ID string
	var orgB_ID string
	var resourceTypeID string

	t.Run("Setup Context: Create Organizations and Assign Roles", func(t *testing.T) {
		// 1. Create Organization A
		createPayload := orgHttp.CreateOrganizationRequest{Name: "Sports Center A", OwnerID: ownerA.ID}
		w := executeRequest("POST", "/v1/organizations", createPayload, sysAdminToken)
		require.Equal(t, http.StatusCreated, w.Code, "Should create organization A successfully")

		var orgResp orgHttp.OrganizationResponse
		json.Unmarshal(w.Body.Bytes(), &orgResp)
		orgA_ID = orgResp.ID

		// Assign Roles for Org A
		// Owner is set. Add Manager:
		executeRequest("POST", fmt.Sprintf("/v1/organizations/%s/managers", orgA_ID),
			orgHttp.AddOrganizationManagerRequest{UserID: adminA.ID}, sysAdminToken)

		// 2. Create Organization B (Target for cross-org attack test)
		// We need an owner for Org B. Let's create one or reuse sysAdmin as owner for simplicity?
		// To match previous test logic where adminB was manager, we can set sysAdmin as owner.
		createPayloadB := orgHttp.CreateOrganizationRequest{Name: "Sports Center B", OwnerID: sysAdmin.ID}
		wB := executeRequest("POST", "/v1/organizations", createPayloadB, sysAdminToken)
		require.Equal(t, http.StatusCreated, wB.Code, "Should create organization B successfully")

		var orgRespB orgHttp.OrganizationResponse
		json.Unmarshal(wB.Body.Bytes(), &orgRespB)
		orgB_ID = orgRespB.ID

		// Assign Admin Role for Org B
		executeRequest("POST", fmt.Sprintf("/v1/organizations/%s/managers", orgB_ID),
			orgHttp.AddOrganizationManagerRequest{UserID: adminB.ID}, sysAdminToken)
	})

	t.Run("Create Resource Type: Input Validation (Bad Request)", func(t *testing.T) {
		// Case 1: Missing Required Fields (Name)
		// The binding:"required" tag should trigger 400
		invalidPayload := rtHttp.CreateRequest{
			OrganizationID: orgA_ID,
			Name:           "", // Empty name
			Description:    "Missing name",
		}
		w := executeRequest("POST", "/v1/resource-types", invalidPayload, adminAToken)
		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 when required name is empty")

		// Case 2: Invalid UUID format for OrganizationID
		// The binding:"uuid" tag should trigger 400
		invalidUUIDPayload := map[string]string{
			"organization_id": "not-a-uuid",
			"name":            "Bad UUID Type",
		}
		wUUID := executeRequest("POST", "/v1/resource-types", invalidUUIDPayload, adminAToken)
		assert.Equal(t, http.StatusBadRequest, wUUID.Code, "Should return 400 when OrganizationID UUID is invalid")

		// Case 3: Invalid JSON Structure (e.g. wrong types)
		invalidTypePayload := map[string]int{
			"name": 12345, // Name expects string, sent int
		}
		wType := executeRequest("POST", "/v1/resource-types", invalidTypePayload, adminAToken)
		assert.Equal(t, http.StatusBadRequest, wType.Code, "Should return 400 for JSON type mismatch")
	})

	t.Run("Create Resource Type: Strict Permission Control", func(t *testing.T) {
		validPayload := rtHttp.CreateRequest{
			OrganizationID: orgA_ID,
			Name:           "Restricted Type",
			Description:    "Should not exist",
		}

		// 1. Regular Member trying to create in their own Org -> Forbidden
		wMember := executeRequest("POST", "/v1/resource-types", validPayload, memberAToken)
		assert.Equal(t, http.StatusForbidden, wMember.Code, "Org members should not be allowed to create resource types")

		// 2. Stranger trying to create -> Forbidden
		wStranger := executeRequest("POST", "/v1/resource-types", validPayload, strangerToken)
		assert.Equal(t, http.StatusForbidden, wStranger.Code, "Strangers should not be allowed to create resource types")

		// 3. Cross-Organization Attack: Admin of Org B trying to create resource for Org A
		// Even though they are an Admin, they are NOT an Admin of Org A.
		wAdminB := executeRequest("POST", "/v1/resource-types", validPayload, adminBToken)
		assert.Equal(t, http.StatusForbidden, wAdminB.Code, "Admin of another org should not be allowed to create resources for this org")

		// 4. Cross-Organization Attack: Admin of Org A trying to create resource for Org B
		payloadForB := rtHttp.CreateRequest{
			OrganizationID: orgB_ID, // Targeting Org B
			Name:           "Attack Type",
		}
		wAdminA := executeRequest("POST", "/v1/resource-types", payloadForB, adminAToken)
		assert.Equal(t, http.StatusForbidden, wAdminA.Code, "Admin A should not be able to create resources in Org B")
	})

	t.Run("Create Resource Type: Success", func(t *testing.T) {
		validPayload := rtHttp.CreateRequest{
			OrganizationID: orgA_ID,
			Name:           "Badminton Court",
			Description:    "Standard BWF approved court",
		}

		// Admin A should succeed
		w := executeRequest("POST", "/v1/resource-types", validPayload, adminAToken)
		require.Equal(t, http.StatusCreated, w.Code, "Admin should be able to create resource type")

		var resp rtHttp.ResourceTypeResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err, "Should unmarshal response successfully")
		assert.Equal(t, "Badminton Court", resp.Name, "Name should match created value")
		assert.Equal(t, "Standard BWF approved court", resp.Description, "Description should match created value")
		assert.NotEmpty(t, resp.ID, "Resource Type ID should be generated")

		resourceTypeID = resp.ID
	})

	t.Run("List Resource Types", func(t *testing.T) {
		path := fmt.Sprintf("/v1/resource-types?organization_id=%s", orgA_ID)

		// Any authenticated user should be able to list
		w := executeRequest("GET", path, nil, strangerToken)
		assert.Equal(t, http.StatusOK, w.Code, "Should allow public/authenticated listing")

		var listResp response.PageResponse[rtHttp.ResourceTypeResponse]
		json.Unmarshal(w.Body.Bytes(), &listResp)

		assert.GreaterOrEqual(t, listResp.Total, 1, "Should return at least one item")
		assert.Equal(t, "Badminton Court", listResp.Items[0].Name, "Should contain the created resource type")
	})

	t.Run("Get Resource Type", func(t *testing.T) {
		// Success case
		path := fmt.Sprintf("/v1/resource-types/%s", resourceTypeID)
		w := executeRequest("GET", path, nil, memberAToken)
		assert.Equal(t, http.StatusOK, w.Code, "Should get existing resource type")

		var resp rtHttp.ResourceTypeResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		assert.Equal(t, resourceTypeID, resp.ID, "ID should match requested ID")

		// Not Found case
		fakeID := "00000000-0000-0000-0000-000000000000"
		wNotFound := executeRequest("GET", fmt.Sprintf("/v1/resource-types/%s", fakeID), nil, memberAToken)
		assert.Equal(t, http.StatusNotFound, wNotFound.Code, "Should return 404 for non-existent ID")
	})

	t.Run("Interact with Invalid UUID Path Parameter", func(t *testing.T) {
		// This test ensures that the server handles malformed UUIDs gracefully (400 Bad Request)
		// instead of crashing or returning 500 Internal Server Error.
		invalidPath := "/v1/resource-types/not-a-uuid"

		// GET
		wGet := executeRequest("GET", invalidPath, nil, memberAToken)
		assert.NotEqual(t, http.StatusInternalServerError, wGet.Code, "Should not return 500 for invalid UUID")
		assert.Equal(t, http.StatusBadRequest, wGet.Code, "Should return 400 for invalid UUID path param in GET")

		// PATCH
		newName := "Dont Update"
		payload := rtHttp.UpdateRequest{Name: &newName}
		wPatch := executeRequest("PATCH", invalidPath, payload, ownerAToken)
		assert.Equal(t, http.StatusBadRequest, wPatch.Code, "Should return 400 for invalid UUID path param in PATCH")

		// DELETE
		wDelete := executeRequest("DELETE", invalidPath, nil, adminAToken)
		assert.Equal(t, http.StatusBadRequest, wDelete.Code, "Should return 400 for invalid UUID path param in DELETE")
	})

	t.Run("Update Resource Type: Input Validation", func(t *testing.T) {
		path := fmt.Sprintf("/v1/resource-types/%s", resourceTypeID)

		// Case: Invalid Type (Binding Error) -> 400 Bad Request
		// This ensures the handler checks JSON format before logic
		invalidTypePayload := map[string]int{
			"name": 12345,
		}
		w := executeRequest("PATCH", path, invalidTypePayload, ownerAToken)
		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 when request body has invalid types")
	})

	t.Run("Update Resource Type: Permission Boundaries", func(t *testing.T) {
		path := fmt.Sprintf("/v1/resource-types/%s", resourceTypeID)
		newName := "Hacked Name"
		payload := rtHttp.UpdateRequest{Name: &newName}

		// 1. Member of same Org -> Forbidden
		wMember := executeRequest("PATCH", path, payload, memberAToken)
		assert.Equal(t, http.StatusForbidden, wMember.Code, "Member should not be allowed to update")

		// 2. Admin of DIFFERENT Org -> Forbidden
		// The handler must check the Organization ID of the *Resource Type being updated*
		wAdminB := executeRequest("PATCH", path, payload, adminBToken)
		assert.Equal(t, http.StatusForbidden, wAdminB.Code, "Admin of different org should not be allowed to update")

		// 3. Stranger -> Forbidden
		wStranger := executeRequest("PATCH", path, payload, strangerToken)
		assert.Equal(t, http.StatusForbidden, wStranger.Code, "Stranger should not be allowed to update")
	})

	t.Run("Update Resource Type: Success", func(t *testing.T) {
		path := fmt.Sprintf("/v1/resource-types/%s", resourceTypeID)
		newName := "Premium Badminton Court"
		newDesc := "Air conditioned"
		payload := rtHttp.UpdateRequest{
			Name:        &newName,
			Description: &newDesc,
		}

		// Owner of Org A should succeed
		w := executeRequest("PATCH", path, payload, ownerAToken)
		require.Equal(t, http.StatusOK, w.Code, "Owner should be able to update")

		var resp rtHttp.ResourceTypeResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		assert.Equal(t, "Premium Badminton Court", resp.Name, "Name should be updated")
		assert.Equal(t, "Air conditioned", resp.Description, "Description should be updated")
	})

	t.Run("Delete Resource Type: Permission Boundaries", func(t *testing.T) {
		path := fmt.Sprintf("/v1/resource-types/%s", resourceTypeID)

		// Member
		wMember := executeRequest("DELETE", path, nil, memberAToken)
		assert.Equal(t, http.StatusForbidden, wMember.Code, "Member should not be allowed to delete")

		// Admin of Different Org
		// Should verify that Admin B cannot delete Org A's resource
		wAdminB := executeRequest("DELETE", path, nil, adminBToken)
		assert.Equal(t, http.StatusForbidden, wAdminB.Code, "Admin of different org should not be allowed to delete")
	})

	t.Run("Delete Resource Type: Success", func(t *testing.T) {
		path := fmt.Sprintf("/v1/resource-types/%s", resourceTypeID)

		// Admin of Org A
		w := executeRequest("DELETE", path, nil, adminAToken)
		assert.Equal(t, http.StatusNoContent, w.Code, "Admin should be able to delete")

		// Verify it is gone
		wGet := executeRequest("GET", path, nil, adminAToken)
		assert.Equal(t, http.StatusNotFound, wGet.Code, "Deleted resource should not be found")
	})

	t.Run("Interact with Non-Existent Resource Type", func(t *testing.T) {
		fakeID := "00000000-0000-0000-0000-000000000000"
		path := fmt.Sprintf("/v1/resource-types/%s", fakeID)
		newName := "Ghost"
		payload := rtHttp.UpdateRequest{Name: &newName}

		// Update -> 404
		wUpdate := executeRequest("PATCH", path, payload, ownerAToken)
		assert.Equal(t, http.StatusNotFound, wUpdate.Code, "Should return 404 when updating non-existent resource")

		// Delete -> 404
		wDelete := executeRequest("DELETE", path, nil, ownerAToken)
		assert.Equal(t, http.StatusNotFound, wDelete.Code, "Should return 404 when deleting non-existent resource")
	})
}
