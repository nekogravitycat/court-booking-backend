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

	locHttp "github.com/nekogravitycat/court-booking-backend/internal/location/http"
	orgHttp "github.com/nekogravitycat/court-booking-backend/internal/organization/http"
	pickupHttp "github.com/nekogravitycat/court-booking-backend/internal/pickup/http"
	resHttp "github.com/nekogravitycat/court-booking-backend/internal/resource/http"
)

// seriesFixture is an organization owner plus a UTC location (06:00-23:00) with two resources.
type seriesFixture struct {
	ownerToken  string
	adminToken  string
	locationID  string
	resourceIDs []string
}

func newSeriesFixture(t *testing.T, suffix string) seriesFixture {
	owner := createTestUser(t, "owner_"+suffix+"@series.com", "pass", false)
	admin := createTestUser(t, "admin_"+suffix+"@series.com", "pass", true)
	ownerToken, adminToken := generateToken(owner.ID), generateToken(admin.ID)

	w := executeRequest("POST", "/v1/organizations", orgHttp.CreateOrganizationRequest{Name: "Series Org " + suffix, OwnerID: owner.ID}, adminToken)
	require.Equal(t, http.StatusCreated, w.Code)
	var org orgHttp.OrganizationResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &org))

	w = executeRequest("POST", "/v1/locations", locHttp.CreateLocationRequest{
		OrganizationID: org.ID, Name: "Series Loc", Capacity: 10,
		OpeningHoursStart: "06:00:00", OpeningHoursEnd: "23:00:00", Opening: true, Timezone: "UTC",
		LocationInfo: "x", Longitude: f64(120), Latitude: f64(23),
	}, ownerToken)
	require.Equal(t, http.StatusCreated, w.Code)
	var loc locHttp.LocationResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &loc))
	assert.Equal(t, 0, loc.MinimumBookingNoticeMinutes)
	assert.Equal(t, 90, loc.MaximumBookingAdvanceDays)

	f := seriesFixture{ownerToken: ownerToken, adminToken: adminToken, locationID: loc.ID}
	for _, name := range []string{"Court A", "Court B"} {
		w = executeRequest("POST", "/v1/resources", resHttp.CreateRequest{Name: name, LocationID: loc.ID, ResourceType: "tennis"}, ownerToken)
		require.Equal(t, http.StatusCreated, w.Code)
		var res resHttp.ResourceResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
		f.resourceIDs = append(f.resourceIDs, res.ID)
	}
	return f
}

func decode(t *testing.T, w interface{ Bytes() []byte }) map[string]any {
	var m map[string]any
	require.NoError(t, json.Unmarshal(w.Bytes(), &m))
	return m
}

func countRows(t *testing.T, query string, args ...any) int {
	var n int
	require.NoError(t, testPool.QueryRow(context.Background(), query, args...).Scan(&n))
	return n
}

func TestBookingSeries(t *testing.T) {
	clearTables()
	f := newSeriesFixture(t, "bs")
	booker := createTestUser(t, "booker@series.com", "pass", false)
	bookerToken := generateToken(booker.ID)
	stranger := createTestUser(t, "stranger@series.com", "pass", false)

	start := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	allDays := []int{0, 1, 2, 3, 4, 5, 6}
	body := func(resourceID string, term int, st, et string) map[string]any {
		return map[string]any{
			"resource_id": resourceID, "term_months": term, "start_date": start,
			"weekdays": allDays, "start_time": st, "end_time": et,
		}
	}

	t.Run("creates all occurrences with one series id", func(t *testing.T) {
		w := executeRequest("POST", "/v1/booking-series", body(f.resourceIDs[0], 3, "08:00", "09:00"), bookerToken)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		resp := decode(t, w.Body)
		bookings := resp["bookings"].([]any)

		first := time.Now().UTC().AddDate(0, 0, 1)
		wantDays := int(first.AddDate(0, 3, 0).Sub(first).Hours() / 24)
		assert.InDelta(t, wantDays, len(bookings), 1) // whole-day span of the 3 month term
		for _, b := range bookings {
			assert.Equal(t, resp["id"], b.(map[string]any)["booking_series_id"])
		}
		assert.Equal(t, len(bookings), countRows(t, "SELECT count(*) FROM public.bookings WHERE booking_series_id = $1", resp["id"]))

		// More than MaxActiveBookingsPerUser (10) bookings were created for one user.
		assert.Greater(t, len(bookings), 10)

		// The owner of the series and a manager can read it; a stranger cannot.
		assert.Equal(t, http.StatusOK, executeRequest("GET", "/v1/booking-series/"+resp["id"].(string), nil, bookerToken).Code)
		assert.Equal(t, http.StatusOK, executeRequest("GET", "/v1/booking-series/"+resp["id"].(string), nil, f.ownerToken).Code)
		assert.Equal(t, http.StatusForbidden, executeRequest("GET", "/v1/booking-series/"+resp["id"].(string), nil, generateToken(stranger.ID)).Code)
	})

	t.Run("a 12 month series exceeds the 90 day advance limit", func(t *testing.T) {
		w := executeRequest("POST", "/v1/booking-series", body(f.resourceIDs[1], 12, "10:00", "11:00"), bookerToken)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		assert.Greater(t, len(decode(t, w.Body)["bookings"].([]any)), 300)
	})

	t.Run("conflict is all-or-nothing and lists the clashing occurrences", func(t *testing.T) {
		before := countRows(t, "SELECT count(*) FROM public.bookings")
		// 08:30-09:30 overlaps every occurrence of the first series on Court A.
		w := executeRequest("POST", "/v1/booking-series", body(f.resourceIDs[0], 3, "08:30", "09:30"), bookerToken)
		require.Equal(t, http.StatusConflict, w.Code)
		resp := decode(t, w.Body)
		assert.Equal(t, "booking series contains conflicting occurrences", resp["error"])
		assert.NotEmpty(t, resp["conflicts"])
		assert.Equal(t, before, countRows(t, "SELECT count(*) FROM public.bookings"))

		// A partial overlap (only the tail of the term) still rejects the whole series.
		partial := body(f.resourceIDs[0], 3, "08:00", "09:00")
		partial["start_date"] = time.Now().UTC().AddDate(0, 3, 0).Add(-48 * time.Hour).Format("2006-01-02")
		w = executeRequest("POST", "/v1/booking-series", partial, bookerToken)
		require.Equal(t, http.StatusConflict, w.Code)
		assert.NotEmpty(t, decode(t, w.Body)["conflicts"])
		assert.Equal(t, before, countRows(t, "SELECT count(*) FROM public.bookings"))
	})

	t.Run("validation failures", func(t *testing.T) {
		assert.Equal(t, http.StatusBadRequest, executeRequest("POST", "/v1/booking-series", body(f.resourceIDs[0], 4, "08:00", "09:00"), bookerToken).Code)
		assert.Equal(t, http.StatusBadRequest, executeRequest("POST", "/v1/booking-series", body(f.resourceIDs[0], 3, "05:00", "06:00"), bookerToken).Code) // outside opening hours
		assert.Equal(t, http.StatusBadRequest, executeRequest("POST", "/v1/booking-series", body(f.resourceIDs[0], 3, "08:10", "09:00"), bookerToken).Code) // not aligned
		past := body(f.resourceIDs[0], 3, "08:00", "09:00")
		past["start_date"] = time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
		assert.Equal(t, http.StatusBadRequest, executeRequest("POST", "/v1/booking-series", past, bookerToken).Code)
		assert.Equal(t, http.StatusNotFound, executeRequest("POST", "/v1/booking-series", body("00000000-0000-0000-0000-000000000000", 3, "08:00", "09:00"), bookerToken).Code)
	})
}

func TestLocationBookingWindowAndAvailability(t *testing.T) {
	clearTables()
	f := newSeriesFixture(t, "bw")
	booker := createTestUser(t, "booker@window.com", "pass", false)
	bookerToken := generateToken(booker.ID)

	// 48 hours notice, 30 days advance.
	notice, advance := 2880, 30
	w := executeRequest("PATCH", "/v1/locations/"+f.locationID, map[string]any{
		"minimum_booking_notice_minutes": notice, "maximum_booking_advance_days": advance,
	}, f.ownerToken)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.EqualValues(t, notice, decode(t, w.Body)["minimum_booking_notice_minutes"])

	t.Run("advance above 90 days or notice above advance is rejected", func(t *testing.T) {
		assert.Equal(t, http.StatusBadRequest, executeRequest("PATCH", "/v1/locations/"+f.locationID, map[string]any{"maximum_booking_advance_days": 91}, f.ownerToken).Code)
		assert.Equal(t, http.StatusBadRequest, executeRequest("PATCH", "/v1/locations/"+f.locationID, map[string]any{"minimum_booking_notice_minutes": 31 * 1440}, f.ownerToken).Code)
	})

	day := func(offset int, hour int) time.Time {
		return time.Now().UTC().AddDate(0, 0, offset).Truncate(24 * time.Hour).Add(time.Duration(hour) * time.Hour)
	}
	book := func(start time.Time) int {
		return executeRequest("POST", "/v1/bookings", map[string]any{
			"resource_id": f.resourceIDs[0], "start_time": start, "end_time": start.Add(time.Hour),
		}, bookerToken).Code
	}

	t.Run("bookings outside the window are rejected", func(t *testing.T) {
		assert.Equal(t, http.StatusBadRequest, book(day(1, 8)))  // within the 48h notice
		assert.Equal(t, http.StatusBadRequest, book(day(31, 8))) // beyond 30 days
		assert.Equal(t, http.StatusCreated, book(day(5, 8)))
	})

	t.Run("availability honours the window", func(t *testing.T) {
		tomorrow := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
		w := executeRequest("GET", "/v1/resources/"+f.resourceIDs[1]+"/availability?date="+tomorrow, nil, bookerToken)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Empty(t, decode(t, w.Body)["slots"])

		farDay := time.Now().UTC().AddDate(0, 0, 32).Format("2006-01-02")
		w = executeRequest("GET", "/v1/resources/"+f.resourceIDs[1]+"/availability?date="+farDay, nil, bookerToken)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Empty(t, decode(t, w.Body)["slots"])

		// The earliest slot is now + notice rounded up to the 30 minute grid.
		earliest := time.Now().UTC().Add(48 * time.Hour).Truncate(30 * time.Minute)
		if earliest.Before(time.Now().UTC().Add(48 * time.Hour)) {
			earliest = earliest.Add(30 * time.Minute)
		}
		date := earliest.Format("2006-01-02")
		w = executeRequest("GET", "/v1/resources/"+f.resourceIDs[1]+"/availability?date="+date, nil, bookerToken)
		require.Equal(t, http.StatusOK, w.Code)
		slots := decode(t, w.Body)["slots"].([]any)
		if h := earliest.Hour(); h >= 6 && h < 23 && len(slots) > 0 {
			got, err := time.Parse(time.RFC3339, slots[0].(map[string]any)["start_time"].(string))
			require.NoError(t, err)
			assert.True(t, got.Equal(earliest), "first slot %v want %v", got, earliest)
		}
	})

	t.Run("location availability matches resource availability", func(t *testing.T) {
		date := day(5, 0).Format("2006-01-02") // day with the booking above on Court A
		w := executeRequest("GET", "/v1/locations/"+f.locationID+"/availability?date="+date, nil, bookerToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		resp := decode(t, w.Body)
		assert.Equal(t, date, resp["date"])
		items := resp["resources"].([]any)
		require.Len(t, items, 2)

		for _, it := range items {
			m := it.(map[string]any)
			id := m["resource"].(map[string]any)["id"].(string)
			single := executeRequest("GET", "/v1/resources/"+id+"/availability?date="+date, nil, bookerToken)
			require.Equal(t, http.StatusOK, single.Code)
			want, _ := json.Marshal(decode(t, single.Body)["slots"])
			got, _ := json.Marshal(m["slots"])
			assert.JSONEq(t, string(want), string(got))
		}
		assert.Equal(t, http.StatusNotFound, executeRequest("GET", "/v1/locations/00000000-0000-0000-0000-000000000000/availability", nil, bookerToken).Code)
	})
}

func pickupGroupPayload(locationID, sportID string, level int, extra map[string]any) map[string]any {
	m := map[string]any{
		"title": "Series Group", "fee": 100, "capacity": 10,
		"location_id": locationID, "sport_id": sportID, "min_skill_level": level,
	}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func TestPickupGroupSeriesAndSocial(t *testing.T) {
	clearTables()
	host := createTestUser(t, "host@pseries.com", "pass", false)
	grantPickupHost(t, host.ID)
	hostToken := generateToken(host.ID)
	locationID := setupTestLocation(t, hostToken, host.ID)
	sportID, level := getSportSkill(t, "BADMINTON", "A")

	occ := func(daysAhead int) map[string]any {
		s := time.Now().UTC().AddDate(0, 0, daysAhead).Truncate(time.Hour)
		return map[string]any{"start_time": s, "end_time": s.Add(2 * time.Hour)}
	}
	series := func(extra map[string]any) map[string]any {
		return pickupGroupPayload(locationID, sportID, level, extra)
	}

	t.Run("creates independent groups sharing a series id", func(t *testing.T) {
		w := executeRequest("POST", "/v1/pickup-group-series", series(map[string]any{
			"social": "  line: @club  ", "registration_deadline_minutes_before_start": 1440,
			"occurrences": []any{occ(3), occ(10), occ(17)},
		}), hostToken)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		resp := decode(t, w.Body)
		groups := resp["groups"].([]any)
		require.Len(t, groups, 3)
		for _, g := range groups {
			gm := g.(map[string]any)
			assert.Equal(t, resp["id"], gm["pickup_group_series_id"])
			assert.Equal(t, "line: @club", gm["social"])
			start, _ := time.Parse(time.RFC3339, gm["start_time"].(string))
			deadline, _ := time.Parse(time.RFC3339, gm["registration_deadline"].(string))
			assert.Equal(t, 24*time.Hour, start.Sub(deadline))
		}
		// Cancelling one group leaves its siblings active.
		first := groups[0].(map[string]any)["id"].(string)
		require.Equal(t, http.StatusOK, executeRequest("PATCH", "/v1/pickup-groups/"+first, map[string]any{"status": "cancelled"}, hostToken).Code)
		assert.Equal(t, 2, countRows(t, "SELECT count(*) FROM public.pickup_groups WHERE pickup_group_series_id = $1 AND status = 'active'", resp["id"]))
	})

	t.Run("all-or-nothing validation", func(t *testing.T) {
		before := countRows(t, "SELECT count(*) FROM public.pickup_groups")
		beforeSeries := countRows(t, "SELECT count(*) FROM public.pickup_group_series")

		tooMany := make([]any, 65)
		for i := range tooMany {
			tooMany[i] = occ(2)
		}
		cases := map[string]map[string]any{
			"more than 64":     {"registration_deadline_minutes_before_start": 60, "occurrences": tooMany},
			"beyond horizon":   {"registration_deadline_minutes_before_start": 60, "occurrences": []any{occ(3), occ(95)}},
			"in the past":      {"registration_deadline_minutes_before_start": 60, "occurrences": []any{occ(3), occ(-1)}},
			"deadline passed":  {"registration_deadline_minutes_before_start": 60 * 24 * 5, "occurrences": []any{occ(3)}},
			"no occurrences":   {"registration_deadline_minutes_before_start": 60, "occurrences": []any{}},
			"social too long":  {"registration_deadline_minutes_before_start": 60, "occurrences": []any{occ(3)}, "social": string(make([]rune, 501))},
			"bad time range":   {"registration_deadline_minutes_before_start": 60, "occurrences": []any{map[string]any{"start_time": occ(3)["end_time"], "end_time": occ(3)["start_time"]}}},
			"missing deadline": {"occurrences": []any{occ(3)}},
		}
		for name, extra := range cases {
			w := executeRequest("POST", "/v1/pickup-group-series", series(extra), hostToken)
			assert.Equal(t, http.StatusBadRequest, w.Code, name+": "+w.Body.String())
		}
		assert.Equal(t, before, countRows(t, "SELECT count(*) FROM public.pickup_groups"))
		assert.Equal(t, beforeSeries, countRows(t, "SELECT count(*) FROM public.pickup_group_series"))

		exactly64 := make([]any, 64)
		for i := range exactly64 {
			exactly64[i] = occ(2)
		}
		w := executeRequest("POST", "/v1/pickup-group-series", series(map[string]any{"registration_deadline_minutes_before_start": 60, "occurrences": exactly64}), hostToken)
		assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	})

	t.Run("social create, patch, clear", func(t *testing.T) {
		w := executeRequest("POST", "/v1/pickup-groups", pickupGroupPayload(locationID, sportID, level, map[string]any{
			"start_time": time.Now().Add(48 * time.Hour), "registration_deadline": time.Now().Add(24 * time.Hour),
			"end_time": time.Now().Add(50 * time.Hour), "social": "   ",
		}), hostToken)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		resp := decode(t, w.Body)
		assert.Nil(t, resp["social"]) // blank is stored as NULL
		assert.Nil(t, resp["pickup_group_series_id"])
		path := "/v1/pickup-groups/" + resp["id"].(string)

		w = executeRequest("PATCH", path, map[string]any{"social": "discord: abc"}, hostToken)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "discord: abc", decode(t, w.Body)["social"])

		w = executeRequest("PATCH", path, map[string]any{"title": "Renamed"}, hostToken) // absent: unchanged
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "discord: abc", decode(t, w.Body)["social"])

		w = executeRequest("PATCH", path, map[string]any{"social": nil}, hostToken) // null: cleared
		require.Equal(t, http.StatusOK, w.Code)
		assert.Nil(t, decode(t, w.Body)["social"])

		executeRequest("PATCH", path, map[string]any{"social": "x"}, hostToken)
		w = executeRequest("PATCH", path, map[string]any{"social": "  "}, hostToken) // blank: cleared
		require.Equal(t, http.StatusOK, w.Code)
		assert.Nil(t, decode(t, w.Body)["social"])

		assert.Equal(t, http.StatusBadRequest, executeRequest("PATCH", path, map[string]any{"social": string(make([]rune, 501))}, hostToken).Code)
		got := executeRequest("GET", path, nil, hostToken)
		assert.Nil(t, decode(t, got.Body)["social"])
	})
}

func TestPickupAttendanceAndStats(t *testing.T) {
	clearTables()
	host := createTestUser(t, "host@attend.com", "pass", false)
	grantPickupHost(t, host.ID)
	admin := createTestUser(t, "admin@attend.com", "pass", true)
	player := createTestUser(t, "player@attend.com", "pass", false)
	other := createTestUser(t, "other@attend.com", "pass", false)
	hostToken, adminToken := generateToken(host.ID), generateToken(admin.ID)
	playerToken, otherToken := generateToken(player.ID), generateToken(other.ID)

	locationID := setupTestLocation(t, hostToken, host.ID)
	sportID, level := getSportSkill(t, "BADMINTON", "A")

	// Groups are spaced apart so one player can enroll in all of them.
	newGroup := func(i int) string {
		start := time.Now().Add(time.Duration(48*(i+1)) * time.Hour)
		w := executeRequest("POST", "/v1/pickup-groups", pickupGroupPayload(locationID, sportID, level, map[string]any{
			"start_time": start, "registration_deadline": time.Now().Add(24 * time.Hour),
			"end_time": start.Add(2 * time.Hour),
		}), hostToken)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		return decode(t, w.Body)["id"].(string)
	}
	// enroll creates an order for the user and optionally confirms it.
	enroll := func(groupID, token string, confirm bool) string {
		w := executeRequest("POST", "/v1/pickup-groups/"+groupID+"/orders", nil, token)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		id := decode(t, w.Body)["id"].(string)
		if confirm {
			require.Equal(t, http.StatusOK, executeRequest("PATCH", "/v1/pickup-orders/"+id, map[string]any{"status": "confirmed"}, hostToken).Code)
		}
		return id
	}
	// finish moves the group into the past (ended an hour ago).
	finish := func(groupID string, offset time.Duration) {
		_, err := testPool.Exec(context.Background(),
			`UPDATE public.pickup_groups SET start_time = now() + $2::interval - interval '3 hours',
			 registration_deadline = now() + $2::interval - interval '4 hours', end_time = now() + $2::interval WHERE id = $1`,
			groupID, fmt.Sprintf("%d seconds", int(offset.Seconds())))
		require.NoError(t, err)
	}
	stats := func(userID string) map[string]any {
		w := executeRequest("GET", "/v1/users/"+userID+"/pickup-stats", nil, otherToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		return decode(t, w.Body)
	}
	absence := func(groupID, orderID, token string) *httpResult {
		w := executeRequest("PUT", fmt.Sprintf("/v1/pickup-groups/%s/orders/%s/absence", groupID, orderID), nil, token)
		return &httpResult{Code: w.Code, Body: w.Body.String()}
	}

	// Stats with no participation: rate is null, not 0.
	s := stats(player.ID)
	assert.EqualValues(t, 0, s["pickup_participation_count"])
	assert.EqualValues(t, 0, s["pickup_absence_count"])
	assert.Nil(t, s["pickup_absence_rate"])
	assert.Equal(t, http.StatusNotFound, executeRequest("GET", "/v1/users/00000000-0000-0000-0000-000000000000/pickup-stats", nil, otherToken).Code)

	g1, g2, g3 := newGroup(0), newGroup(1), newGroup(2)
	o1 := enroll(g1, playerToken, true)
	o2 := enroll(g2, playerToken, true)
	o3 := enroll(g3, playerToken, true)
	pendingOther := enroll(g1, otherToken, false)

	t.Run("cannot mark before the group ends", func(t *testing.T) {
		assert.Equal(t, http.StatusConflict, absence(g1, o1, hostToken).Code)
	})

	finish(g1, -time.Hour)
	finish(g2, -time.Hour)

	t.Run("permissions and preconditions", func(t *testing.T) {
		assert.Equal(t, http.StatusForbidden, absence(g1, o1, playerToken).Code)
		assert.Equal(t, http.StatusForbidden, absence(g1, o1, otherToken).Code)
		assert.Equal(t, http.StatusConflict, absence(g1, pendingOther, hostToken).Code) // not confirmed
		assert.Equal(t, http.StatusNotFound, absence(g2, o1, hostToken).Code)           // order of another group
	})

	t.Run("mark, count and revoke", func(t *testing.T) {
		s := stats(player.ID)
		assert.EqualValues(t, 2, s["pickup_participation_count"]) // g3 has not ended
		assert.EqualValues(t, 0, s["pickup_absence_count"])
		assert.EqualValues(t, 0, s["pickup_absence_rate"])

		w := absence(g1, o1, hostToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body)
		assert.Equal(t, http.StatusOK, absence(g2, o2, adminToken).Code)

		s = stats(player.ID)
		assert.EqualValues(t, 2, s["pickup_participation_count"])
		assert.EqualValues(t, 2, s["pickup_absence_count"])
		assert.EqualValues(t, 1, s["pickup_absence_rate"])

		// Revoking restores the statistics immediately.
		del := executeRequest("DELETE", fmt.Sprintf("/v1/pickup-groups/%s/orders/%s/absence", g2, o2), nil, hostToken)
		assert.Equal(t, http.StatusNoContent, del.Code)
		s = stats(player.ID)
		assert.EqualValues(t, 1, s["pickup_absence_count"])
		assert.EqualValues(t, 0.5, s["pickup_absence_rate"])

		assert.Equal(t, http.StatusForbidden, executeRequest("DELETE", fmt.Sprintf("/v1/pickup-groups/%s/orders/%s/absence", g1, o1), nil, playerToken).Code)

		// The mark is visible to the host on the orders list.
		list := executeRequest("GET", "/v1/pickup-groups/"+g1+"/orders", nil, hostToken)
		require.Equal(t, http.StatusOK, list.Code)
		var orders []pickupHttp.PickupOrderResponse
		require.NoError(t, json.Unmarshal(list.Body.Bytes(), &orders))
		for _, o := range orders {
			if o.ID == o1 {
				require.NotNil(t, o.AttendanceStatus)
				assert.Equal(t, "absent", *o.AttendanceStatus)
			}
		}
	})

	t.Run("cancelled groups and cancelled orders do not count", func(t *testing.T) {
		finish(g3, -time.Hour)
		require.Equal(t, http.StatusOK, executeRequest("PATCH", "/v1/pickup-groups/"+g3, map[string]any{"status": "cancelled"}, hostToken).Code)
		assert.Equal(t, http.StatusConflict, absence(g3, o3, hostToken).Code)
		assert.EqualValues(t, 2, stats(player.ID)["pickup_participation_count"])

		// A cancelled order drops out of the statistics even if it was marked earlier.
		_, err := testPool.Exec(context.Background(), "UPDATE public.pickup_orders SET status = 'cancelled' WHERE id = $1", o1)
		require.NoError(t, err)
		s := stats(player.ID)
		assert.EqualValues(t, 1, s["pickup_participation_count"])
		assert.EqualValues(t, 0, s["pickup_absence_count"])
	})
}

type httpResult struct {
	Code int
	Body string
}
