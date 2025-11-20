package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	annHttp "github.com/nekogravitycat/court-booking-backend/internal/announcement/http"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
)

func TestAnnouncementCRUDAndPermissions(t *testing.T) {
	clearTables()

	// ==== Setup Users & Tokens ====

	sysAdmin := createTestUser(t, "admin@news.com", "pass", true)
	regularUser := createTestUser(t, "user@news.com", "pass", false)

	sysAdminToken := generateToken(sysAdmin.ID, sysAdmin.Email)
	regularUserToken := generateToken(regularUser.ID, regularUser.Email)
	// Empty token for testing 401
	noToken := ""

	var announcementID string

	// ==== Create Tests (Validation & Permissions) ====

	t.Run("Create Announcement: Success (System Admin)", func(t *testing.T) {
		payload := annHttp.CreateBody{
			Title:   "System Maintenance",
			Content: "The system will be down at midnight.",
		}

		w := executeRequest("POST", "/v1/announcements", payload, sysAdminToken)
		require.Equal(t, http.StatusCreated, w.Code)

		var resp annHttp.Response
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.NotEmpty(t, resp.ID)
		assert.Equal(t, payload.Title, resp.Title)
		assert.Equal(t, payload.Content, resp.Content)
		assert.NotEmpty(t, resp.CreatedAt)
		assert.NotEmpty(t, resp.UpdatedAt)

		announcementID = resp.ID
	})

	t.Run("Create Announcement: Unauthorized (No Token)", func(t *testing.T) {
		// Ensure middleware catches missing auth header
		payload := annHttp.CreateBody{
			Title:   "Secret News",
			Content: "Content",
		}
		w := executeRequest("POST", "/v1/announcements", payload, noToken)
		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 when no token is provided")
	})

	t.Run("Create Announcement: Permission Denied (Regular User)", func(t *testing.T) {
		payload := annHttp.CreateBody{
			Title:   "Hacked Announcement",
			Content: "I shouldn't be able to post this.",
		}

		w := executeRequest("POST", "/v1/announcements", payload, regularUserToken)
		assert.Equal(t, http.StatusForbidden, w.Code, "Regular users must not create announcements")
	})

	t.Run("Create Announcement: Validation Failure (Empty Fields)", func(t *testing.T) {
		// Case 1: Empty Title (Logic Check)
		payloadNoTitle := annHttp.CreateBody{
			Title:   "   ", // Only spaces should also be rejected
			Content: "Content without title",
		}
		wTitle := executeRequest("POST", "/v1/announcements", payloadNoTitle, sysAdminToken)
		assert.Equal(t, http.StatusBadRequest, wTitle.Code, "Should return 400 for whitespace-only title")

		// Case 2: Empty Content (Logic Check)
		payloadNoContent := annHttp.CreateBody{
			Title:   "Title without content",
			Content: "",
		}
		wContent := executeRequest("POST", "/v1/announcements", payloadNoContent, sysAdminToken)
		assert.Equal(t, http.StatusBadRequest, wContent.Code, "Should return 400 for empty content")
	})

	t.Run("Create Announcement: Malformed Input (Bad Request)", func(t *testing.T) {
		// Case 1: Invalid JSON Types (e.g. Title is an integer)
		// This ensures the ShouldBindJSON does not panic and returns 400
		invalidTypePayload := map[string]interface{}{
			"title":   12345, // Should be string
			"content": "Valid content",
		}
		w := executeRequest("POST", "/v1/announcements", invalidTypePayload, sysAdminToken)
		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 for JSON type mismatch")
	})

	// ==== Read & List Tests ====

	t.Run("List Announcements: Success & Filtering", func(t *testing.T) {
		// Create a second announcement to test filtering
		secondPayload := annHttp.CreateBody{
			Title:   "Badminton Tournament",
			Content: "Join us next week!",
		}
		executeRequest("POST", "/v1/announcements", secondPayload, sysAdminToken)

		// 1. List All (Authenticated User)
		wAll := executeRequest("GET", "/v1/announcements", nil, regularUserToken)
		assert.Equal(t, http.StatusOK, wAll.Code)

		var listResp response.PageResponse[annHttp.Response]
		json.Unmarshal(wAll.Body.Bytes(), &listResp)
		assert.GreaterOrEqual(t, listResp.Total, 2)

		// 2. Filter by Keyword (Matches "Maintenance")
		wFilter := executeRequest("GET", "/v1/announcements?q=Maintenance", nil, regularUserToken)
		assert.Equal(t, http.StatusOK, wFilter.Code)

		var filterResp response.PageResponse[annHttp.Response]
		json.Unmarshal(wFilter.Body.Bytes(), &filterResp)
		assert.Equal(t, 1, filterResp.Total)
		assert.Equal(t, "System Maintenance", filterResp.Items[0].Title)
	})

	t.Run("Get Announcement: Success", func(t *testing.T) {
		path := fmt.Sprintf("/v1/announcements/%s", announcementID)
		w := executeRequest("GET", path, nil, regularUserToken)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp annHttp.Response
		json.Unmarshal(w.Body.Bytes(), &resp)
		assert.Equal(t, announcementID, resp.ID)
	})

	// ==== Update Tests ====

	t.Run("Update Announcement: Success (System Admin)", func(t *testing.T) {
		path := fmt.Sprintf("/v1/announcements/%s", announcementID)
		newTitle := "Updated Maintenance Schedule"
		payload := annHttp.UpdateBody{
			Title: &newTitle,
		}

		w := executeRequest("PATCH", path, payload, sysAdminToken)
		require.Equal(t, http.StatusOK, w.Code)

		var resp annHttp.Response
		json.Unmarshal(w.Body.Bytes(), &resp)
		assert.Equal(t, newTitle, resp.Title)
		// Content should remain unchanged
		assert.Equal(t, "The system will be down at midnight.", resp.Content)
	})

	t.Run("Update Announcement: Permission Denied (Regular User)", func(t *testing.T) {
		path := fmt.Sprintf("/v1/announcements/%s", announcementID)
		newTitle := "Hacked Title"
		payload := annHttp.UpdateBody{
			Title: &newTitle,
		}

		w := executeRequest("PATCH", path, payload, regularUserToken)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("Update Announcement: Validation Failure (Empty Strings)", func(t *testing.T) {
		path := fmt.Sprintf("/v1/announcements/%s", announcementID)
		emptyStr := "   " // Whitespace should be trimmed and fail
		payload := annHttp.UpdateBody{
			Title: &emptyStr,
		}

		w := executeRequest("PATCH", path, payload, sysAdminToken)
		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 when updating title to empty/whitespace")
	})

	t.Run("Update Announcement: Malformed Input", func(t *testing.T) {
		path := fmt.Sprintf("/v1/announcements/%s", announcementID)
		// sending integer instead of string
		invalidPayload := map[string]int{"title": 123}

		w := executeRequest("PATCH", path, invalidPayload, sysAdminToken)
		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 for invalid JSON types in update")
	})

	// ==== Delete Tests ====

	t.Run("Delete Announcement: Permission Denied (Regular User)", func(t *testing.T) {
		path := fmt.Sprintf("/v1/announcements/%s", announcementID)
		w := executeRequest("DELETE", path, nil, regularUserToken)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("Delete Announcement: Success (System Admin)", func(t *testing.T) {
		path := fmt.Sprintf("/v1/announcements/%s", announcementID)
		w := executeRequest("DELETE", path, nil, sysAdminToken)
		assert.Equal(t, http.StatusNoContent, w.Code)

		// Verify it's gone
		wGet := executeRequest("GET", path, nil, sysAdminToken)
		assert.Equal(t, http.StatusNotFound, wGet.Code)
	})

	// ==== Not Found Edge Cases ====

	t.Run("Interact with Non-Existent Announcement", func(t *testing.T) {
		fakeID := "00000000-0000-0000-0000-000000000000"
		path := fmt.Sprintf("/v1/announcements/%s", fakeID)

		// GET
		wGet := executeRequest("GET", path, nil, regularUserToken)
		assert.Equal(t, http.StatusNotFound, wGet.Code)

		// PATCH
		newTitle := "Ghost"
		payload := annHttp.UpdateBody{Title: &newTitle}
		wPatch := executeRequest("PATCH", path, payload, sysAdminToken)
		assert.Equal(t, http.StatusNotFound, wPatch.Code)

		// DELETE
		wDelete := executeRequest("DELETE", path, nil, sysAdminToken)
		assert.Equal(t, http.StatusNotFound, wDelete.Code)
	})

	// ==== Invalid UUID Edge Cases ====

	t.Run("Interact with Invalid UUID Path Parameter", func(t *testing.T) {
		// This tests specifically that the handler checks UUID format
		// BEFORE calling the service/DB, avoiding DB driver errors.
		invalidPath := "/v1/announcements/not-a-uuid"

		// GET
		wGet := executeRequest("GET", invalidPath, nil, regularUserToken)
		assert.Equal(t, http.StatusBadRequest, wGet.Code, "Should return 400 for invalid UUID in GET")

		// PATCH
		newTitle := "Should fail"
		payload := annHttp.UpdateBody{Title: &newTitle}
		wPatch := executeRequest("PATCH", invalidPath, payload, sysAdminToken)
		assert.Equal(t, http.StatusBadRequest, wPatch.Code, "Should return 400 for invalid UUID in PATCH")

		// DELETE
		wDelete := executeRequest("DELETE", invalidPath, nil, sysAdminToken)
		assert.Equal(t, http.StatusBadRequest, wDelete.Code, "Should return 400 for invalid UUID in DELETE")
	})
}
