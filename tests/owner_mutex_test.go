package tests

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	orgHttp "github.com/nekogravitycat/court-booking-backend/internal/organization/http"
)

func TestOwnerMutex(t *testing.T) {
	clearTables()

	// Setup users
	admin := createTestUser(t, "admin@mutex.com", "pass", true)
	owner := createTestUser(t, "owner@mutex.com", "pass", false)

	adminToken := generateToken(admin.ID)
	ownerToken := generateToken(owner.ID)

	var orgID string

	// 1. Create Organization with Owner
	t.Run("Setup Organization", func(t *testing.T) {
		createPayload := orgHttp.CreateOrganizationRequest{
			Name:    "Mutex Org",
			OwnerID: owner.ID,
		}
		w := executeRequest("POST", "/v1/organizations", createPayload, adminToken)
		require.Equal(t, http.StatusCreated, w.Code)

		var resp orgHttp.OrganizationResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		orgID = resp.ID
	})

	// 2. Try to add Owner as Member -> Should Fail (Mutex)
	t.Run("Owner Cannot Be Member", func(t *testing.T) {
		payload := orgHttp.AddOrganizationMemberRequest{
			UserID: owner.ID,
		}
		w := executeRequest("POST", "/v1/organizations/"+orgID+"/members", payload, ownerToken)

		// Expecting Conflict (409) as implemented
		assert.Equal(t, http.StatusConflict, w.Code, "Owner should not be able to be added as a member")
	})
}
