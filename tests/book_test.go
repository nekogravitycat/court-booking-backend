package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	bookingHttp "github.com/nekogravitycat/court-booking-backend/internal/booking/http"
	locHttp "github.com/nekogravitycat/court-booking-backend/internal/location/http"
	orgHttp "github.com/nekogravitycat/court-booking-backend/internal/organization/http"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
	resHttp "github.com/nekogravitycat/court-booking-backend/internal/resource/http"
	rtHttp "github.com/nekogravitycat/court-booking-backend/internal/resourcetype/http"
)

func TestBookingCRUDAndPermissions(t *testing.T) {
	clearTables()

	// ==== Setup Users & Tokens ====
	sysAdmin := createTestUser(t, "sysadmin@book.com", "pass", true)

	// Org A Users (The Organization owning the resource)
	orgOwnerA := createTestUser(t, "owner.a@book.com", "pass", false)
	orgAdminA := createTestUser(t, "admin.a@book.com", "pass", false)
	orgMemberA := createTestUser(t, "member.a@book.com", "pass", false)

	// Org B User (Admin of a different organization - Cross-Org Attack Vector)
	orgAdminB := createTestUser(t, "admin.b@book.com", "pass", false)

	// Regular User (The Booker - Unaffiliated with Org)
	booker := createTestUser(t, "booker@book.com", "pass", false)

	// Stranger (Another user unrelated to the booking)
	stranger := createTestUser(t, "stranger@book.com", "pass", false)

	// Generate Tokens
	sysAdminToken := generateToken(sysAdmin.ID, sysAdmin.Email)
	orgOwnerAToken := generateToken(orgOwnerA.ID, orgOwnerA.Email)
	orgAdminAToken := generateToken(orgAdminA.ID, orgAdminA.Email)
	orgMemberAToken := generateToken(orgMemberA.ID, orgMemberA.Email)
	orgAdminBToken := generateToken(orgAdminB.ID, orgAdminB.Email)
	bookerToken := generateToken(booker.ID, booker.Email)
	strangerToken := generateToken(stranger.ID, stranger.Email)

	// Shared Variables
	var resourceID string
	var bookingID string

	// ==== Setup Infrastructure (Org -> Loc -> RT -> Resource) ====
	t.Run("Setup Infrastructure", func(t *testing.T) {
		// 1. Create Org A
		wOrg := executeRequest("POST", "/v1/organizations", orgHttp.CreateOrganizationRequest{Name: "Booking Center A"}, sysAdminToken)
		var orgA orgHttp.OrganizationResponse
		json.Unmarshal(wOrg.Body.Bytes(), &orgA)

		// 2. Assign Roles for Org A
		executeRequest("POST", fmt.Sprintf("/v1/organizations/%s/members", orgA.ID),
			orgHttp.AddMemberRequest{UserID: orgOwnerA.ID, Role: "owner"}, sysAdminToken)
		executeRequest("POST", fmt.Sprintf("/v1/organizations/%s/members", orgA.ID),
			orgHttp.AddMemberRequest{UserID: orgAdminA.ID, Role: "admin"}, sysAdminToken)
		executeRequest("POST", fmt.Sprintf("/v1/organizations/%s/members", orgA.ID),
			orgHttp.AddMemberRequest{UserID: orgMemberA.ID, Role: "member"}, sysAdminToken)

		// 3. Create Location in Org A
		locPayload := locHttp.CreateLocationBody{
			OrganizationID:    orgA.ID,
			Name:              "Court Loc A",
			Capacity:          10,
			OpeningHoursStart: "06:00:00", OpeningHoursEnd: "23:00:00",
			LocationInfo: "Test Info", Longitude: 120.0, Latitude: 23.0,
		}
		wLoc := executeRequest("POST", "/v1/locations", locPayload, orgAdminAToken)
		var locA locHttp.LocationResponse
		json.Unmarshal(wLoc.Body.Bytes(), &locA)

		// 4. Create Resource Type in Org A
		rtPayload := rtHttp.CreateBody{OrganizationID: orgA.ID, Name: "Tennis"}
		wRT := executeRequest("POST", "/v1/resource-types", rtPayload, orgAdminAToken)
		var rtA rtHttp.Response
		json.Unmarshal(wRT.Body.Bytes(), &rtA)

		// 5. Create Resource (The Asset to be booked)
		resPayload := resHttp.CreateBody{
			Name:           "Tennis Court 1",
			LocationID:     locA.ID,
			ResourceTypeID: rtA.ID,
		}
		wRes := executeRequest("POST", "/v1/resources", resPayload, orgAdminAToken)
		var resA resHttp.Response
		json.Unmarshal(wRes.Body.Bytes(), &resA)
		resourceID = resA.ID

		// 6. Create Org B (For isolation tests)
		wOrgB := executeRequest("POST", "/v1/organizations", orgHttp.CreateOrganizationRequest{Name: "Center B"}, sysAdminToken)
		var orgB orgHttp.OrganizationResponse
		json.Unmarshal(wOrgB.Body.Bytes(), &orgB)
		executeRequest("POST", fmt.Sprintf("/v1/organizations/%s/members", orgB.ID),
			orgHttp.AddMemberRequest{UserID: orgAdminB.ID, Role: "admin"}, sysAdminToken)
	})

	// ==== Create Booking Tests (Input Validation & Business Logic) ====

	t.Run("Create Booking: Bad Request (Invalid Input Format)", func(t *testing.T) {
		// Case: Missing Resource ID
		invalidPayload := map[string]any{
			"start_time": time.Now().Add(time.Hour),
			"end_time":   time.Now().Add(2 * time.Hour),
		}
		w := executeRequest("POST", "/v1/bookings", invalidPayload, bookerToken)
		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 for missing required fields")

		// Case: Invalid UUID format for Resource ID
		badUUIDPayload := map[string]any{
			"resource_id": "not-a-uuid",
			"start_time":  time.Now().Add(time.Hour),
			"end_time":    time.Now().Add(2 * time.Hour),
		}
		wUUID := executeRequest("POST", "/v1/bookings", badUUIDPayload, bookerToken)
		assert.Equal(t, http.StatusBadRequest, wUUID.Code, "Should return 400 for invalid UUID")

		// Case: Invalid Time Format (JSON Unmarshal Error)
		badTimeFormatPayload := map[string]any{
			"resource_id": resourceID,
			"start_time":  "invalid-time",
			"end_time":    "invalid-time",
		}
		wTime := executeRequest("POST", "/v1/bookings", badTimeFormatPayload, bookerToken)
		assert.Equal(t, http.StatusBadRequest, wTime.Code, "Should return 400 for invalid time format")
	})

	t.Run("Create Booking: Bad Request (Business Logic)", func(t *testing.T) {
		// Case: End Time Before Start Time
		badRangePayload := bookingHttp.CreateBookingBody{
			ResourceID: resourceID,
			StartTime:  time.Now().Add(2 * time.Hour),
			EndTime:    time.Now().Add(1 * time.Hour),
		}
		wRange := executeRequest("POST", "/v1/bookings", badRangePayload, bookerToken)
		assert.Equal(t, http.StatusBadRequest, wRange.Code, "Should return 400 for invalid time range")

		// Case: Start Time in the Past
		pastPayload := bookingHttp.CreateBookingBody{
			ResourceID: resourceID,
			StartTime:  time.Now().Add(-2 * time.Hour),
			EndTime:    time.Now().Add(-1 * time.Hour),
		}
		wPast := executeRequest("POST", "/v1/bookings", pastPayload, bookerToken)
		assert.Equal(t, http.StatusBadRequest, wPast.Code, "Should return 400 when booking in the past")

		// Verify detailed error message is present
		var errResp map[string]string
		json.Unmarshal(wPast.Body.Bytes(), &errResp)
		assert.Contains(t, errResp["error"], "past", "Error message should explain the past time restriction")
	})

	t.Run("Create Booking: Success", func(t *testing.T) {
		// Book for Tomorrow
		startTime := time.Now().UTC().Add(24 * time.Hour)
		endTime := startTime.Add(1 * time.Hour)

		payload := bookingHttp.CreateBookingBody{
			ResourceID: resourceID,
			StartTime:  startTime,
			EndTime:    endTime,
		}

		w := executeRequest("POST", "/v1/bookings", payload, bookerToken)
		require.Equal(t, http.StatusCreated, w.Code)

		var resp bookingHttp.BookingResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.NotEmpty(t, resp.ID)
		assert.Equal(t, resourceID, resp.ResourceID)
		assert.Equal(t, booker.ID, resp.UserID)
		assert.Equal(t, "pending", resp.Status)

		bookingID = resp.ID
	})

	t.Run("Create Booking: Conflict (Overlap)", func(t *testing.T) {
		// Attempt to book overlapping slot (Start same time)
		startTime := time.Now().UTC().Add(24 * time.Hour)
		endTime := startTime.Add(1 * time.Hour)

		payload := bookingHttp.CreateBookingBody{
			ResourceID: resourceID,
			StartTime:  startTime,
			EndTime:    endTime,
		}

		// Different user tries to book same slot
		w := executeRequest("POST", "/v1/bookings", payload, strangerToken)
		assert.Equal(t, http.StatusConflict, w.Code, "Should return 409 Conflict for overlapping booking")

		// Attempt Partial Overlap (Starts inside existing booking)
		partialPayload := bookingHttp.CreateBookingBody{
			ResourceID: resourceID,
			StartTime:  startTime.Add(30 * time.Minute),
			EndTime:    endTime.Add(30 * time.Minute),
		}
		wPartial := executeRequest("POST", "/v1/bookings", partialPayload, strangerToken)
		assert.Equal(t, http.StatusConflict, wPartial.Code, "Should return 409 for partial overlap")
	})

	// ==== List Bookings Tests ====

	t.Run("List Bookings: Visibility Control", func(t *testing.T) {
		// 1. Booker sees their own booking
		wOwner := executeRequest("GET", "/v1/bookings", nil, bookerToken)
		assert.Equal(t, http.StatusOK, wOwner.Code)
		var respOwner response.PageResponse[bookingHttp.BookingResponse]
		json.Unmarshal(wOwner.Body.Bytes(), &respOwner)
		assert.GreaterOrEqual(t, respOwner.Total, 1)
		assert.Equal(t, bookingID, respOwner.Items[0].ID)

		// 2. Stranger sees NOTHING (Filtered by User ID automatically for non-admins)
		wStranger := executeRequest("GET", "/v1/bookings", nil, strangerToken)
		assert.Equal(t, http.StatusOK, wStranger.Code)
		var respStranger response.PageResponse[bookingHttp.BookingResponse]
		json.Unmarshal(wStranger.Body.Bytes(), &respStranger)
		assert.Equal(t, 0, respStranger.Total, "Stranger should see 0 bookings as they have none")

		// 3. SysAdmin sees Everything (and can filter by user)
		path := fmt.Sprintf("/v1/bookings?user_id=%s", booker.ID)
		wAdmin := executeRequest("GET", path, nil, sysAdminToken)
		assert.Equal(t, http.StatusOK, wAdmin.Code)
		var respAdmin response.PageResponse[bookingHttp.BookingResponse]
		json.Unmarshal(wAdmin.Body.Bytes(), &respAdmin)
		assert.Equal(t, 1, respAdmin.Total)
	})

	// ==== Get Single Booking Tests (Permission Matrix) ====

	t.Run("Get Booking: Permission Matrix", func(t *testing.T) {
		path := fmt.Sprintf("/v1/bookings/%s", bookingID)

		// 1. Owner -> OK
		wOwner := executeRequest("GET", path, nil, bookerToken)
		assert.Equal(t, http.StatusOK, wOwner.Code)

		// 2. Org Owner (Resource Owner) -> OK
		wOrgOwner := executeRequest("GET", path, nil, orgOwnerAToken)
		assert.Equal(t, http.StatusOK, wOrgOwner.Code, "Org Owner should view bookings for their resources")

		// 3. Org Admin (Resource Manager) -> OK
		wOrgAdmin := executeRequest("GET", path, nil, orgAdminAToken)
		assert.Equal(t, http.StatusOK, wOrgAdmin.Code, "Org Admin should view bookings for their resources")

		// 4. Regular Member of Org (NOT Admin/Owner) -> Forbidden
		// Members do not have management rights over resources
		wOrgMember := executeRequest("GET", path, nil, orgMemberAToken)
		assert.Equal(t, http.StatusForbidden, wOrgMember.Code, "Regular Org Member should not view others' bookings")

		// 5. Sys Admin -> OK
		wSys := executeRequest("GET", path, nil, sysAdminToken)
		assert.Equal(t, http.StatusOK, wSys.Code)

		// 6. Stranger -> Forbidden
		wStranger := executeRequest("GET", path, nil, strangerToken)
		assert.Equal(t, http.StatusForbidden, wStranger.Code, "Stranger should not view others' bookings")

		// 7. Admin of OTHER Org (Cross-Org Isolation) -> Forbidden
		wOrgB := executeRequest("GET", path, nil, orgAdminBToken)
		assert.Equal(t, http.StatusForbidden, wOrgB.Code, "Admin of Org B cannot view bookings of Org A")
	})

	// ==== Update Booking Tests (Logic & Permissions) ====

	t.Run("Update Booking: Validation & Bad Requests", func(t *testing.T) {
		path := fmt.Sprintf("/v1/bookings/%s", bookingID)

		// Invalid Start > End
		newStart := time.Now().UTC().Add(48 * time.Hour)
		newEnd := newStart.Add(-1 * time.Hour) // Invalid
		payload := bookingHttp.UpdateBookingBody{StartTime: &newStart, EndTime: &newEnd}

		w := executeRequest("PATCH", path, payload, bookerToken)
		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 for invalid time range update")

		// Invalid Status string
		badStatus := "archived"
		payloadStatus := bookingHttp.UpdateBookingBody{Status: &badStatus}
		wStatus := executeRequest("PATCH", path, payloadStatus, sysAdminToken)
		assert.Equal(t, http.StatusBadRequest, wStatus.Code, "Should return 400 for invalid status enum")
	})

	t.Run("Update Booking: Owner Restrictions", func(t *testing.T) {
		path := fmt.Sprintf("/v1/bookings/%s", bookingID)

		// Owner tries to CONFIRM their own booking -> Forbidden
		// Only Managers/Admins can confirm
		statusConfirmed := "confirmed"
		payload := bookingHttp.UpdateBookingBody{Status: &statusConfirmed}
		w := executeRequest("PATCH", path, payload, bookerToken)
		assert.Equal(t, http.StatusForbidden, w.Code, "User cannot confirm their own booking")

		// Owner tries to CANCEL -> OK
		statusCancelled := "cancelled"
		payloadCancel := bookingHttp.UpdateBookingBody{Status: &statusCancelled}
		wCancel := executeRequest("PATCH", path, payloadCancel, bookerToken)
		assert.Equal(t, http.StatusOK, wCancel.Code, "User should be able to cancel their own booking")

		// Reset to pending for next tests (using SysAdmin)
		statusPending := "pending"
		executeRequest("PATCH", path, bookingHttp.UpdateBookingBody{Status: &statusPending}, sysAdminToken)
	})

	t.Run("Update Booking: Reschedule (Success & Conflict)", func(t *testing.T) {
		path := fmt.Sprintf("/v1/bookings/%s", bookingID)

		// 1. Success: Reschedule to empty slot
		newStart := time.Now().UTC().Add(50 * time.Hour)
		newEnd := newStart.Add(1 * time.Hour)
		payload := bookingHttp.UpdateBookingBody{StartTime: &newStart, EndTime: &newEnd}

		w := executeRequest("PATCH", path, payload, bookerToken)
		assert.Equal(t, http.StatusOK, w.Code, "Owner should be able to reschedule")

		// 2. Conflict: Reschedule to overlapped slot
		// Create another booking first
		conflictStart := time.Now().UTC().Add(60 * time.Hour)
		conflictEnd := conflictStart.Add(1 * time.Hour)
		otherPayload := bookingHttp.CreateBookingBody{
			ResourceID: resourceID,
			StartTime:  conflictStart,
			EndTime:    conflictEnd,
		}
		executeRequest("POST", "/v1/bookings", otherPayload, strangerToken)

		// Try to move original booking to this time
		conflictUpdate := bookingHttp.UpdateBookingBody{StartTime: &conflictStart, EndTime: &conflictEnd}
		wConflict := executeRequest("PATCH", path, conflictUpdate, bookerToken)
		assert.Equal(t, http.StatusConflict, wConflict.Code, "Should return 409 when rescheduling to occupied slot")
	})

	t.Run("Update Booking: Cross-Org Protection", func(t *testing.T) {
		path := fmt.Sprintf("/v1/bookings/%s", bookingID)
		statusConfirmed := "confirmed"
		payload := bookingHttp.UpdateBookingBody{Status: &statusConfirmed}

		// Admin of Org B tries to update booking in Org A
		w := executeRequest("PATCH", path, payload, orgAdminBToken)
		assert.Equal(t, http.StatusForbidden, w.Code, "Admin of another org cannot update booking")
	})

	// ==== Delete Booking Tests ====

	t.Run("Delete Booking: Permission Denied", func(t *testing.T) {
		path := fmt.Sprintf("/v1/bookings/%s", bookingID)

		// Stranger
		wStranger := executeRequest("DELETE", path, nil, strangerToken)
		assert.Equal(t, http.StatusForbidden, wStranger.Code)

		// Cross-Org Admin
		wOrgB := executeRequest("DELETE", path, nil, orgAdminBToken)
		assert.Equal(t, http.StatusForbidden, wOrgB.Code)

		// Regular Org Member (Non-Admin)
		wMember := executeRequest("DELETE", path, nil, orgMemberAToken)
		assert.Equal(t, http.StatusForbidden, wMember.Code)
	})

	t.Run("Delete Booking: Success", func(t *testing.T) {
		// Create a disposable booking
		startTime := time.Now().UTC().Add(100 * time.Hour)
		createPayload := bookingHttp.CreateBookingBody{
			ResourceID: resourceID,
			StartTime:  startTime,
			EndTime:    startTime.Add(1 * time.Hour),
		}
		wCreate := executeRequest("POST", "/v1/bookings", createPayload, bookerToken)
		var tempBooking bookingHttp.BookingResponse
		json.Unmarshal(wCreate.Body.Bytes(), &tempBooking)

		// Org Admin deletes it
		path := fmt.Sprintf("/v1/bookings/%s", tempBooking.ID)
		wDelete := executeRequest("DELETE", path, nil, orgAdminAToken)
		assert.Equal(t, http.StatusNoContent, wDelete.Code, "Org Admin should be able to delete booking")

		// Verify it is gone
		wGet := executeRequest("GET", path, nil, sysAdminToken)
		assert.Equal(t, http.StatusNotFound, wGet.Code)
	})

	// ==== Not Found & Invalid ID Edge Cases ====

	t.Run("Interact with Non-Existent Booking", func(t *testing.T) {
		fakeID := "00000000-0000-0000-0000-000000000000"
		path := fmt.Sprintf("/v1/bookings/%s", fakeID)

		// GET -> 404
		wGet := executeRequest("GET", path, nil, sysAdminToken)
		assert.Equal(t, http.StatusNotFound, wGet.Code, "Should return 404 for non-existent ID")

		// PATCH -> 404
		status := "cancelled"
		payload := bookingHttp.UpdateBookingBody{Status: &status}
		wUpdate := executeRequest("PATCH", path, payload, sysAdminToken)
		assert.Equal(t, http.StatusNotFound, wUpdate.Code, "Should return 404 for updating non-existent ID")

		// DELETE -> 404
		wDelete := executeRequest("DELETE", path, nil, sysAdminToken)
		assert.Equal(t, http.StatusNotFound, wDelete.Code, "Should return 404 for deleting non-existent ID")
	})

	t.Run("Interact with Invalid UUID Path Parameter", func(t *testing.T) {
		// This ensures the router/handler catches malformed UUIDs before hitting DB
		invalidPath := "/v1/bookings/not-a-uuid"

		// GET -> 400
		wGet := executeRequest("GET", invalidPath, nil, sysAdminToken)
		assert.Equal(t, http.StatusBadRequest, wGet.Code, "Should return 400 for invalid UUID in GET")

		// PATCH -> 400
		status := "cancelled"
		payload := bookingHttp.UpdateBookingBody{Status: &status}
		wPatch := executeRequest("PATCH", invalidPath, payload, sysAdminToken)
		assert.Equal(t, http.StatusBadRequest, wPatch.Code, "Should return 400 for invalid UUID in PATCH")

		// DELETE -> 400
		wDelete := executeRequest("DELETE", invalidPath, nil, sysAdminToken)
		assert.Equal(t, http.StatusBadRequest, wDelete.Code, "Should return 400 for invalid UUID in DELETE")
	})
}
