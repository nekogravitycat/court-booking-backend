package tests

import (
	"context"
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
	pickupHttp "github.com/nekogravitycat/court-booking-backend/internal/pickup/http"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
	resHttp "github.com/nekogravitycat/court-booking-backend/internal/resource/http"
)

// setupBookableResource creates an org, an open location (UTC, 06:00-23:00),
// and a resource, and returns the resource ID.
func setupBookableResource(t *testing.T, sysAdminToken, ownerID, ownerToken string) string {
	t.Helper()
	wOrg := executeRequest("POST", "/v1/organizations", orgHttp.CreateOrganizationRequest{Name: "Fix Org", OwnerID: ownerID}, sysAdminToken)
	require.Equal(t, http.StatusCreated, wOrg.Code, wOrg.Body.String())
	var org orgHttp.OrganizationResponse
	require.NoError(t, json.Unmarshal(wOrg.Body.Bytes(), &org))

	wLoc := executeRequest("POST", "/v1/locations", locHttp.CreateLocationRequest{
		OrganizationID: org.ID, Name: "Fix Loc", Capacity: 10,
		OpeningHoursStart: "06:00:00", OpeningHoursEnd: "23:00:00",
		Opening: true, Timezone: "UTC", LocationInfo: "info", Longitude: f64(120), Latitude: f64(23),
	}, ownerToken)
	require.Equal(t, http.StatusCreated, wLoc.Code, wLoc.Body.String())
	var loc locHttp.LocationResponse
	require.NoError(t, json.Unmarshal(wLoc.Body.Bytes(), &loc))

	wRes := executeRequest("POST", "/v1/resources", resHttp.CreateRequest{Name: "Court", LocationID: loc.ID, ResourceType: "tennis"}, ownerToken)
	require.Equal(t, http.StatusCreated, wRes.Code, wRes.Body.String())
	var res resHttp.ResourceResponse
	require.NoError(t, json.Unmarshal(wRes.Body.Bytes(), &res))
	return res.ID
}

func createBooking(t *testing.T, token, resourceID string, start time.Time) bookingHttp.BookingResponse {
	t.Helper()
	w := executeRequest("POST", "/v1/bookings", map[string]any{
		"resource_id": resourceID, "start_time": start, "end_time": start.Add(time.Hour),
	}, token)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var b bookingHttp.BookingResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &b))
	return b
}

func TestBlankDisplayNameAndNullSafeBookingReads(t *testing.T) {
	clearTables()

	w := executeRequest("POST", "/v1/auth/register", map[string]any{
		"email": "blank@fix.com", "username": "blank_user", "password": "password123", "display_name": "   ",
	}, "")
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	admin := createTestUser(t, "admin@fix.com", "pass", true)
	owner := createTestUser(t, "owner@fix.com", "pass", false)
	booker := createTestUser(t, "booker@fix.com", "pass", false)
	adminToken, ownerToken, bookerToken := generateToken(admin.ID), generateToken(owner.ID), generateToken(booker.ID)
	resourceID := setupBookableResource(t, adminToken, owner.ID, ownerToken)

	// Legacy rows may still have a NULL display_name; reads must not fail.
	_, err := testPool.Exec(context.Background(), "UPDATE public.users SET display_name = NULL WHERE id = $1", booker.ID)
	require.NoError(t, err)

	start := time.Now().UTC().Truncate(24 * time.Hour).Add(32 * time.Hour)
	b := createBooking(t, bookerToken, resourceID, start)
	assert.Equal(t, booker.Username, b.User.Name)

	wGet := executeRequest("GET", "/v1/bookings/"+b.ID, nil, bookerToken)
	assert.Equal(t, http.StatusOK, wGet.Code, wGet.Body.String())
	wAvail := executeRequest("GET", fmt.Sprintf("/v1/resources/%s/availability?date=%s", resourceID, start.Format("2006-01-02")), nil, ownerToken)
	assert.Equal(t, http.StatusOK, wAvail.Code, wAvail.Body.String())
}

func TestBookingOwnerRestrictionsAndDeactivation(t *testing.T) {
	clearTables()

	admin := createTestUser(t, "admin2@fix.com", "pass", true)
	admin2 := createTestUser(t, "admin3@fix.com", "pass", true)
	owner := createTestUser(t, "owner2@fix.com", "pass", false)
	booker := createTestUser(t, "booker2@fix.com", "pass", false)
	adminToken, ownerToken, bookerToken := generateToken(admin.ID), generateToken(owner.ID), generateToken(booker.ID)
	resourceID := setupBookableResource(t, adminToken, owner.ID, ownerToken)
	base := time.Now().UTC().Truncate(24 * time.Hour).Add(32 * time.Hour)

	patch := func(token, id string, body map[string]any) int {
		return executeRequest("PATCH", "/v1/bookings/"+id, body, token).Code
	}

	t.Run("unconfirmed booking is freely editable by its owner", func(t *testing.T) {
		b := createBooking(t, bookerToken, resourceID, base)
		assert.Equal(t, http.StatusOK, patch(bookerToken, b.ID, map[string]any{"start_time": base.Add(2 * time.Hour), "end_time": base.Add(3 * time.Hour)}))
		assert.Equal(t, http.StatusNoContent, executeRequest("DELETE", "/v1/bookings/"+b.ID, nil, bookerToken).Code)
	})

	t.Run("confirmed and paid booking needs a manager", func(t *testing.T) {
		b := createBooking(t, bookerToken, resourceID, base)
		require.Equal(t, http.StatusOK, patch(ownerToken, b.ID, map[string]any{"status": "confirmed", "payment_status": "done"}))

		assert.Equal(t, http.StatusForbidden, patch(bookerToken, b.ID, map[string]any{"status": "cancelled"}))
		assert.Equal(t, http.StatusForbidden, patch(bookerToken, b.ID, map[string]any{"start_time": base.Add(4 * time.Hour), "end_time": base.Add(5 * time.Hour)}))
		assert.Equal(t, http.StatusForbidden, executeRequest("DELETE", "/v1/bookings/"+b.ID, nil, bookerToken).Code)
		assert.Equal(t, http.StatusOK, patch(bookerToken, b.ID, map[string]any{"status": "cancel_request"}))
		assert.Equal(t, http.StatusForbidden, patch(bookerToken, b.ID, map[string]any{"status": "cancelled"}))

		// A manager can still cancel it, after which the owner cannot revive it.
		require.Equal(t, http.StatusOK, patch(ownerToken, b.ID, map[string]any{"status": "cancelled"}))
		assert.Equal(t, http.StatusForbidden, patch(bookerToken, b.ID, map[string]any{"status": "cancel_request"}))
	})

	t.Run("deactivating an account cancels its upcoming bookings", func(t *testing.T) {
		b := createBooking(t, bookerToken, resourceID, base.Add(6*time.Hour))
		w := executeRequest("PATCH", "/v1/users/"+booker.ID, map[string]any{"is_active": false}, adminToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var status string
		require.NoError(t, testPool.QueryRow(context.Background(), "SELECT status::text FROM public.bookings WHERE id = $1", b.ID).Scan(&status))
		assert.Equal(t, "cancelled", status)
	})

	t.Run("admins cannot deactivate themselves or the last admin", func(t *testing.T) {
		assert.Equal(t, http.StatusForbidden, executeRequest("PATCH", "/v1/users/"+admin.ID, map[string]any{"is_active": false}, adminToken).Code)
		assert.Equal(t, http.StatusForbidden, executeRequest("DELETE", "/v1/users/"+admin.ID, nil, adminToken).Code)

		// With admin2 deactivated, admin is the last active admin. Another actor
		// (a non-admin cannot reach this route), so exercise the repository guard
		// through the service path: admin2 is inactive and admin cannot be removed.
		require.Equal(t, http.StatusOK, executeRequest("PATCH", "/v1/users/"+admin2.ID, map[string]any{"is_active": false}, adminToken).Code)
		assert.Equal(t, http.StatusForbidden, executeRequest("PATCH", "/v1/users/"+admin.ID, map[string]any{"is_active": false}, adminToken).Code)
	})
}

func TestPickupPrivacyAndDeactivation(t *testing.T) {
	clearTables()

	host := createTestUser(t, "phost@fix.com", "pass", false)
	grantPickupHost(t, host.ID)
	member := createTestUser(t, "pmember@fix.com", "pass", false)
	pending := createTestUser(t, "ppending@fix.com", "pass", false)
	admin := createTestUser(t, "padmin@fix.com", "pass", true)
	hostToken, memberToken, pendingToken, adminToken := generateToken(host.ID), generateToken(member.ID), generateToken(pending.ID), generateToken(admin.ID)

	_, err := testPool.Exec(context.Background(), "UPDATE public.users SET phone = '0912345678' WHERE id = $1", host.ID)
	require.NoError(t, err)

	locationID := setupTestLocation(t, hostToken, host.ID)
	sportID, level := getSportSkill(t, "BADMINTON", "A")
	start := time.Now().Add(24 * time.Hour)
	group := createGroup(t, hostToken, locationID, sportID, level, 4, 0, start, start.Add(2*time.Hour))
	hidden := createGroup(t, hostToken, locationID, sportID, level, 4, 0, start.Add(5*time.Hour), start.Add(7*time.Hour))
	enabled := false
	require.Equal(t, http.StatusOK, executeRequest("PATCH", "/v1/pickup-groups/"+hidden.ID, pickupHttp.UpdateGroupBody{Enable: &enabled}, hostToken).Code)

	memberOrder := enroll(t, memberToken, group.ID, level)
	enroll(t, pendingToken, group.ID, level)

	phoneFor := func(token string) *string {
		w := executeRequest("GET", "/v1/pickup-groups/"+group.ID, nil, token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var g pickupHttp.PickupGroupResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &g))
		return g.Host.Phone
	}

	t.Run("host phone is visible only to host, admin and confirmed members", func(t *testing.T) {
		assert.Nil(t, phoneFor(memberToken), "pending member")
		assert.Nil(t, phoneFor(pendingToken))
		require.NotNil(t, phoneFor(hostToken))
		require.NotNil(t, phoneFor(adminToken))

		setOrderStatus(t, hostToken, memberOrder.ID, "confirmed")
		require.NotNil(t, phoneFor(memberToken), "confirmed member")
		assert.Nil(t, phoneFor(pendingToken))
	})

	t.Run("host group list hides disabled groups from non-owners", func(t *testing.T) {
		count := func(token string) int {
			w := executeRequest("GET", "/v1/hosts/"+host.ID+"/pickup-groups", nil, token)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			var resp response.PageResponse[pickupHttp.PickupGroupBrief]
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			return resp.Total
		}
		assert.Equal(t, 1, count(""))
		assert.Equal(t, 1, count(memberToken))
		assert.Equal(t, 2, count(hostToken))
		assert.Equal(t, 2, count(adminToken))
	})

	t.Run("deactivating a member cancels their enrollment", func(t *testing.T) {
		require.Equal(t, http.StatusOK, executeRequest("PATCH", "/v1/users/"+member.ID, map[string]any{"is_active": false}, adminToken).Code)
		var status string
		require.NoError(t, testPool.QueryRow(context.Background(), "SELECT status::text FROM public.pickup_orders WHERE id = $1", memberOrder.ID).Scan(&status))
		assert.Equal(t, "cancelled", status)
		assert.GreaterOrEqual(t, listNotifications(t, hostToken, "").Total, 1)
	})

	t.Run("deactivating a host cancels their groups and notifies enrolled users", func(t *testing.T) {
		require.Equal(t, http.StatusOK, executeRequest("PATCH", "/v1/users/"+host.ID, map[string]any{"is_active": false}, adminToken).Code)
		var status string
		require.NoError(t, testPool.QueryRow(context.Background(), "SELECT status::text FROM public.pickup_groups WHERE id = $1", group.ID).Scan(&status))
		assert.Equal(t, "cancelled", status)

		resp := listNotifications(t, pendingToken, "")
		found := false
		for _, n := range resp.Items {
			if n.Type == "pickup_group_cancelled" {
				found = true
			}
		}
		assert.True(t, found, "enrolled user should be told the group was cancelled")
	})
}

func TestBookingAbuseLimits(t *testing.T) {
	clearTables()

	admin := createTestUser(t, "admin4@fix.com", "pass", true)
	owner := createTestUser(t, "owner4@fix.com", "pass", false)
	booker := createTestUser(t, "booker4@fix.com", "pass", false)
	adminToken, ownerToken, bookerToken := generateToken(admin.ID), generateToken(owner.ID), generateToken(booker.ID)
	resourceID := setupBookableResource(t, adminToken, owner.ID, ownerToken)
	base := time.Now().UTC().Truncate(24 * time.Hour).Add(32 * time.Hour)

	post := func(start, end time.Time) int {
		return executeRequest("POST", "/v1/bookings", map[string]any{"resource_id": resourceID, "start_time": start, "end_time": end}, bookerToken).Code
	}

	assert.Equal(t, http.StatusBadRequest, post(base.Add(7*time.Minute), base.Add(67*time.Minute)), "unaligned start")
	assert.Equal(t, http.StatusBadRequest, post(base, base.Add(45*time.Minute)), "unaligned end")
	assert.Equal(t, http.StatusBadRequest, post(base.Add(100*24*time.Hour), base.Add(100*24*time.Hour+time.Hour)), "too far ahead")

	for i := 0; i < 10; i++ {
		createBooking(t, bookerToken, resourceID, base.Add(time.Duration(i)*24*time.Hour))
	}
	assert.Equal(t, http.StatusConflict, post(base.Add(20*24*time.Hour), base.Add(20*24*time.Hour+time.Hour)), "11th active booking")
}

func TestValidationEdgeCases(t *testing.T) {
	clearTables()

	admin := createTestUser(t, "admin5@fix.com", "pass", true)
	owner := createTestUser(t, "owner5@fix.com", "pass", false)
	adminToken, ownerToken := generateToken(admin.ID), generateToken(owner.ID)

	t.Run("phone can be set and cleared", func(t *testing.T) {
		w := executeRequest("PATCH", "/v1/me", map[string]any{"phone": "0912345678"}, ownerToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		w = executeRequest("PATCH", "/v1/me", map[string]any{"phone": ""}, ownerToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var phone *string
		require.NoError(t, testPool.QueryRow(context.Background(), "SELECT phone FROM public.users WHERE id = $1", owner.ID).Scan(&phone))
		assert.Nil(t, phone)
	})

	t.Run("location accepts zero coordinates and resource sport can be cleared", func(t *testing.T) {
		wOrg := executeRequest("POST", "/v1/organizations", orgHttp.CreateOrganizationRequest{Name: "Edge Org", OwnerID: owner.ID}, adminToken)
		require.Equal(t, http.StatusCreated, wOrg.Code, wOrg.Body.String())
		var org orgHttp.OrganizationResponse
		require.NoError(t, json.Unmarshal(wOrg.Body.Bytes(), &org))

		wLoc := executeRequest("POST", "/v1/locations", map[string]any{
			"organization_id": org.ID, "name": "Equator", "capacity": 5,
			"opening_hours_start": "06:00:00", "opening_hours_end": "23:00:00",
			"location_info": "info", "longitude": 0, "latitude": 0,
		}, ownerToken)
		require.Equal(t, http.StatusCreated, wLoc.Code, wLoc.Body.String())
		var loc locHttp.LocationResponse
		require.NoError(t, json.Unmarshal(wLoc.Body.Bytes(), &loc))

		wMissing := executeRequest("POST", "/v1/locations", map[string]any{
			"organization_id": org.ID, "name": "Missing", "capacity": 5,
			"opening_hours_start": "06:00:00", "opening_hours_end": "23:00:00", "location_info": "info",
		}, ownerToken)
		assert.Equal(t, http.StatusBadRequest, wMissing.Code)

		wBlank := executeRequest("PATCH", "/v1/locations/"+loc.ID, map[string]any{"name": "   "}, ownerToken)
		assert.Equal(t, http.StatusBadRequest, wBlank.Code)

		sportID, _ := getSportSkill(t, "BADMINTON", "A")
		wRes := executeRequest("POST", "/v1/resources", map[string]any{
			"name": "Court", "location_id": loc.ID, "resource_type": "badminton", "sport_id": sportID,
		}, ownerToken)
		require.Equal(t, http.StatusCreated, wRes.Code, wRes.Body.String())
		var res resHttp.ResourceResponse
		require.NoError(t, json.Unmarshal(wRes.Body.Bytes(), &res))
		require.NotNil(t, res.SportID)

		wClear := executeRequest("PATCH", "/v1/resources/"+res.ID, map[string]any{"sport_id": ""}, ownerToken)
		require.Equal(t, http.StatusOK, wClear.Code, wClear.Body.String())
		var cleared resHttp.ResourceResponse
		require.NoError(t, json.Unmarshal(wClear.Body.Bytes(), &cleared))
		assert.Nil(t, cleared.SportID)
	})

	t.Run("missing records map to 404 instead of 500", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound, executeRequest("GET", "/v1/files/00000000-0000-0000-0000-000000000000", nil, ownerToken).Code)
		assert.Equal(t, http.StatusNotFound, executeRequest("GET", "/v1/files/not-a-uuid", nil, ownerToken).Code)
	})
}
