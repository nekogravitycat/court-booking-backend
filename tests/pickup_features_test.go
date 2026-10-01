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
	notificationHttp "github.com/nekogravitycat/court-booking-backend/internal/notification/http"
	"github.com/nekogravitycat/court-booking-backend/internal/pickup"
	pickupHttp "github.com/nekogravitycat/court-booking-backend/internal/pickup/http"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
	skillRatingHttp "github.com/nekogravitycat/court-booking-backend/internal/skillrating/http"
	userHttp "github.com/nekogravitycat/court-booking-backend/internal/user/http"
)

// createGroup creates a pickup group through the API, bounding its skill range
// to the single given level (min == max), and returns it.
func createGroup(t *testing.T, hostToken, locationID, sportID string, level, capacity, fee int, start, end time.Time) pickupHttp.PickupGroupResponse {
	t.Helper()
	maxLevel := level
	w := executeRequest("POST", "/v1/pickup-groups", pickupHttp.CreateGroupBody{
		Title:         fmt.Sprintf("Group L%d F%d", level, fee),
		StartTime:     start,
		EndTime:       end,
		Fee:           fee,
		Capacity:      capacity,
		LocationID:    locationID,
		SportID:       sportID,
		MinSkillLevel: level,
		MaxSkillLevel: &maxLevel,
	}, hostToken)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp pickupHttp.PickupGroupResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

// enroll configures the caller's account level and enrolls without a body.
func enroll(t *testing.T, token, groupID string, level int) pickupHttp.PickupOrderResponse {
	t.Helper()
	var sportID string
	require.NoError(t, testPool.QueryRow(context.Background(),
		"SELECT sport_id FROM public.pickup_groups WHERE id = $1", groupID).Scan(&sportID))
	wSet := executeRequest("PUT", "/v1/me/skill-levels/"+sportID, userHttp.SetSkillLevelBody{SkillLevel: level}, token)
	require.Equal(t, http.StatusOK, wSet.Code, wSet.Body.String())
	w := executeRequest("POST", fmt.Sprintf("/v1/pickup-groups/%s/orders", groupID),
		nil, token)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp pickupHttp.PickupOrderResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

// setOrderStatus PATCHes an order status as the given user.
func setOrderStatus(t *testing.T, token, orderID, status string) {
	t.Helper()
	w := executeRequest("PATCH", "/v1/pickup-orders/"+orderID, pickupHttp.UpdateOrderBody{Status: &status}, token)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func listNotifications(t *testing.T, token, query string) response.PageResponse[notificationHttp.NotificationResponse] {
	t.Helper()
	w := executeRequest("GET", "/v1/notifications"+query, nil, token)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp response.PageResponse[notificationHttp.NotificationResponse]
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

func TestUserProfileFields(t *testing.T) {
	clearTables()

	t.Run("register with gender and birth date", func(t *testing.T) {
		gender, birth := "female", "1990-05-20"
		body := userHttp.RegisterRequest{
			Email: "prof1@ex.com", Username: "prof_one", Password: "password123", DisplayName: "P",
			Gender: &gender, BirthDate: &birth,
		}
		w := executeRequest("POST", "/v1/auth/register", body, "")
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

		var resp userHttp.MeResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.NotNil(t, resp.User.Gender)
		assert.Equal(t, "female", *resp.User.Gender)
		require.NotNil(t, resp.User.BirthDate)
		assert.Equal(t, "1990-05-20", *resp.User.BirthDate)
		require.NotNil(t, resp.User.Age)
		assert.GreaterOrEqual(t, *resp.User.Age, 35)
	})

	t.Run("register without them leaves both null", func(t *testing.T) {
		body := userHttp.RegisterRequest{Email: "prof2@ex.com", Username: "prof_two", Password: "password123", DisplayName: "P"}
		w := executeRequest("POST", "/v1/auth/register", body, "")
		require.Equal(t, http.StatusCreated, w.Code)

		var resp userHttp.MeResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Nil(t, resp.User.Gender)
		assert.Nil(t, resp.User.BirthDate)
		assert.Nil(t, resp.User.Age)
	})

	t.Run("register rejects invalid gender", func(t *testing.T) {
		gender := "robot"
		body := userHttp.RegisterRequest{Email: "prof3@ex.com", Username: "prof_three", Password: "password123", DisplayName: "P", Gender: &gender}
		w := executeRequest("POST", "/v1/auth/register", body, "")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	u := createTestUser(t, "prof4@ex.com", "pass", false)
	token := generateToken(u.ID)

	t.Run("PATCH /me updates own profile", func(t *testing.T) {
		name, gender, birth := "New Name", "male", "1985-01-31"
		w := executeRequest("PATCH", "/v1/me", userHttp.UpdateMeRequest{DisplayName: &name, Gender: &gender, BirthDate: &birth}, token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp userHttp.MeResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "New Name", *resp.User.DisplayName)
		assert.Equal(t, "male", *resp.User.Gender)
		assert.Equal(t, "1985-01-31", *resp.User.BirthDate)

		// Persisted.
		wGet := executeRequest("GET", "/v1/me", nil, token)
		require.Equal(t, http.StatusOK, wGet.Code)
		var got userHttp.MeResponse
		require.NoError(t, json.Unmarshal(wGet.Body.Bytes(), &got))
		assert.Equal(t, "1985-01-31", *got.User.BirthDate)
	})

	t.Run("PATCH /me cannot escalate privileges", func(t *testing.T) {
		raw := map[string]any{"is_system_admin": true, "display_name": "Sneaky"}
		w := executeRequest("PATCH", "/v1/me", raw, token)
		require.Equal(t, http.StatusOK, w.Code)

		var resp userHttp.MeResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.False(t, resp.User.IsSystemAdmin)
	})

	t.Run("PATCH /me rejects a future birth date", func(t *testing.T) {
		future := time.Now().AddDate(1, 0, 0).Format("2006-01-02")
		w := executeRequest("PATCH", "/v1/me", userHttp.UpdateMeRequest{BirthDate: &future}, token)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("PATCH /me rejects a malformed birth date and invalid gender", func(t *testing.T) {
		bad := "20-01-1990"
		assert.Equal(t, http.StatusBadRequest,
			executeRequest("PATCH", "/v1/me", userHttp.UpdateMeRequest{BirthDate: &bad}, token).Code)

		gender := "x"
		assert.Equal(t, http.StatusBadRequest,
			executeRequest("PATCH", "/v1/me", userHttp.UpdateMeRequest{Gender: &gender}, token).Code)
	})

	t.Run("PATCH /me requires authentication", func(t *testing.T) {
		name := "x"
		w := executeRequest("PATCH", "/v1/me", userHttp.UpdateMeRequest{DisplayName: &name}, "")
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestLocationParking(t *testing.T) {
	clearTables()

	host := createTestUser(t, "parkhost@pickup.com", "pass", false)
	hostToken := generateToken(host.ID)
	locationID := setupTestLocation(t, hostToken, host.ID)

	getLoc := func() locHttp.LocationResponse {
		w := executeRequest("GET", "/v1/locations/"+locationID, nil, hostToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp locHttp.LocationResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		return resp
	}

	t.Run("parking is null by default", func(t *testing.T) {
		loc := getLoc()
		assert.Nil(t, loc.ParkingName)
		assert.Nil(t, loc.ParkingLatitude)
		assert.Nil(t, loc.ParkingLongitude)
	})

	t.Run("partial parking data is rejected", func(t *testing.T) {
		name := "Lot A"
		w := executeRequest("PATCH", "/v1/locations/"+locationID, locHttp.UpdateLocationRequest{ParkingName: &name}, hostToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("set parking", func(t *testing.T) {
		name, lat, lng := "Lot A", 25.03, 121.56
		w := executeRequest("PATCH", "/v1/locations/"+locationID,
			locHttp.UpdateLocationRequest{ParkingName: &name, ParkingLatitude: &lat, ParkingLongitude: &lng}, hostToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		loc := getLoc()
		require.NotNil(t, loc.ParkingName)
		assert.Equal(t, "Lot A", *loc.ParkingName)
		assert.InDelta(t, 25.03, *loc.ParkingLatitude, 1e-9)
		assert.InDelta(t, 121.56, *loc.ParkingLongitude, 1e-9)
	})

	t.Run("unrelated update keeps parking", func(t *testing.T) {
		newName := "Renamed Court"
		w := executeRequest("PATCH", "/v1/locations/"+locationID, locHttp.UpdateLocationRequest{Name: &newName}, hostToken)
		require.Equal(t, http.StatusOK, w.Code)
		assert.NotNil(t, getLoc().ParkingName)
	})

	t.Run("remove_parking clears all three fields", func(t *testing.T) {
		w := executeRequest("PATCH", "/v1/locations/"+locationID, locHttp.UpdateLocationRequest{RemoveParking: true}, hostToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		loc := getLoc()
		assert.Nil(t, loc.ParkingName)
		assert.Nil(t, loc.ParkingLatitude)
		assert.Nil(t, loc.ParkingLongitude)
	})

	t.Run("remove_parking cannot be combined with parking fields", func(t *testing.T) {
		name, lat, lng := "Lot B", 25.0, 121.0
		w := executeRequest("PATCH", "/v1/locations/"+locationID,
			locHttp.UpdateLocationRequest{ParkingName: &name, ParkingLatitude: &lat, ParkingLongitude: &lng, RemoveParking: true}, hostToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestPickupListFilters(t *testing.T) {
	clearTables()

	host := createTestUser(t, "fhost@pickup.com", "pass", false)
	grantPickupHost(t, host.ID)
	otherHost := createTestUser(t, "fhost2@pickup.com", "pass", false)
	grantPickupHost(t, otherHost.ID)
	viewer := createTestUser(t, "fviewer@pickup.com", "pass", false)

	hostToken := generateToken(host.ID)
	otherHostToken := generateToken(otherHost.ID)
	viewerToken := generateToken(viewer.ID)

	nearLoc := setupTestLocation(t, hostToken, host.ID) // 25.0, 121.0
	farLoc := setupTestLocation(t, hostToken, host.ID)
	_, err := testPool.Exec(context.Background(),
		"UPDATE public.locations SET latitude = 22.6, longitude = 120.3 WHERE id = $1", farLoc)
	require.NoError(t, err)
	otherLoc := setupTestLocation(t, otherHostToken, otherHost.ID)

	sportID, _ := getSportSkill(t, "BADMINTON", "A")
	start := time.Now().Add(24 * time.Hour)
	end := start.Add(2 * time.Hour)

	cheap := createGroup(t, hostToken, farLoc, sportID, 1, 8, 0, start, end)
	mid := createGroup(t, hostToken, nearLoc, sportID, 3, 8, 100, start.Add(time.Hour), end.Add(time.Hour))
	pricey := createGroup(t, hostToken, farLoc, sportID, 2, 8, 300, start.Add(2*time.Hour), end.Add(2*time.Hour))
	other := createGroup(t, otherHostToken, otherLoc, sportID, 4, 8, 50, start.Add(3*time.Hour), end.Add(3*time.Hour))

	list := func(t *testing.T, token, query string) (int, response.PageResponse[pickupHttp.PickupGroupBrief]) {
		w := executeRequest("GET", "/v1/pickup-groups"+query, nil, token)
		var resp response.PageResponse[pickupHttp.PickupGroupBrief]
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w.Code, resp
	}
	ids := func(resp response.PageResponse[pickupHttp.PickupGroupBrief]) []string {
		out := make([]string, len(resp.Items))
		for i, it := range resp.Items {
			out[i] = it.ID
		}
		return out
	}

	t.Run("fee range is inclusive", func(t *testing.T) {
		code, resp := list(t, "", "?fee_min=50&fee_max=200")
		require.Equal(t, http.StatusOK, code)
		assert.ElementsMatch(t, []string{mid.ID, other.ID}, ids(resp))

		_, resp = list(t, "", "?fee_min=100")
		assert.ElementsMatch(t, []string{mid.ID, pricey.ID}, ids(resp))

		_, resp = list(t, "", "?fee_max=0")
		assert.Equal(t, []string{cheap.ID}, ids(resp))
	})

	t.Run("inverted fee range is rejected", func(t *testing.T) {
		code, _ := list(t, "", "?fee_min=300&fee_max=10")
		assert.Equal(t, http.StatusBadRequest, code)
	})

	t.Run("sort by skill level", func(t *testing.T) {
		_, asc := list(t, "", "?sort_by=min_skill_level&sort_order=asc")
		assert.Equal(t, []string{cheap.ID, pricey.ID, mid.ID, other.ID}, ids(asc))

		_, desc := list(t, "", "?sort_by=min_skill_level&sort_order=desc")
		assert.Equal(t, []string{other.ID, mid.ID, pricey.ID, cheap.ID}, ids(desc))
	})

	t.Run("filter by exact skill level", func(t *testing.T) {
		_, resp := list(t, "", "?min_skill_level=3&max_skill_level=3")
		assert.Equal(t, []string{mid.ID}, ids(resp))
		assert.Equal(t, 3, resp.Items[0].MinSkillLevel.Level)
		assert.Equal(t, "C", resp.Items[0].MinSkillLevel.Label)
	})

	t.Run("sort by distance nearest first", func(t *testing.T) {
		code, resp := list(t, "", "?sort_by=distance&latitude=25.0&longitude=121.0")
		require.Equal(t, http.StatusOK, code)
		require.Len(t, resp.Items, 4)

		// Both near locations are at the origin; equal distances sort by ID.
		assert.ElementsMatch(t, []string{mid.ID, other.ID}, ids(resp)[:2])
		expectedFirst := mid.ID
		if other.ID < mid.ID {
			expectedFirst = other.ID
		}
		assert.Equal(t, expectedFirst, resp.Items[0].ID)
		require.NotNil(t, resp.Items[0].DistanceKm)
		assert.InDelta(t, 0, *resp.Items[0].DistanceKm, 0.01)
		require.NotNil(t, resp.Items[3].DistanceKm)
		assert.Greater(t, *resp.Items[3].DistanceKm, 200.0)

		for i := 1; i < len(resp.Items); i++ {
			assert.LessOrEqual(t, *resp.Items[i-1].DistanceKm, *resp.Items[i].DistanceKm)
		}

		_, farthest := list(t, "", "?sort_by=distance&sort_order=desc&latitude=25.0&longitude=121.0")
		assert.NotEqual(t, mid.ID, farthest.Items[0].ID)
	})

	t.Run("distance_km is null without an origin", func(t *testing.T) {
		_, resp := list(t, "", "")
		require.NotEmpty(t, resp.Items)
		for _, it := range resp.Items {
			assert.Nil(t, it.DistanceKm)
		}
	})

	t.Run("distance sort needs an origin", func(t *testing.T) {
		code, _ := list(t, "", "?sort_by=distance")
		assert.Equal(t, http.StatusBadRequest, code)
	})

	t.Run("latitude and longitude must come together", func(t *testing.T) {
		code, _ := list(t, "", "?latitude=25.0")
		assert.Equal(t, http.StatusBadRequest, code)
		code, _ = list(t, "", "?latitude=95&longitude=121")
		assert.Equal(t, http.StatusBadRequest, code)
	})

	t.Run("followed_only requires auth", func(t *testing.T) {
		code, _ := list(t, "", "?followed_only=true")
		assert.Equal(t, http.StatusUnauthorized, code)
	})

	t.Run("followed_only shows only followed hosts", func(t *testing.T) {
		// Not following anyone yet.
		code, resp := list(t, viewerToken, "?followed_only=true")
		require.Equal(t, http.StatusOK, code)
		assert.Empty(t, resp.Items)
		assert.Equal(t, 0, resp.Total)

		w := executeRequest("POST", "/v1/favorites/host", map[string]string{"host_id": otherHost.ID}, viewerToken)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

		_, resp = list(t, viewerToken, "?followed_only=true")
		assert.Equal(t, []string{other.ID}, ids(resp))

		// Without the flag every group is still listed.
		_, resp = list(t, viewerToken, "")
		assert.Len(t, resp.Items, 4)
	})

	t.Run("filters combine with pagination totals", func(t *testing.T) {
		_, resp := list(t, "", "?fee_min=0&page_size=2&page=2&sort_by=min_skill_level&sort_order=asc")
		assert.Equal(t, 4, resp.Total)
		assert.Equal(t, []string{mid.ID, other.ID}, ids(resp))
	})
}

func TestPartyEnrollmentAndParticipantStats(t *testing.T) {
	clearTables()

	host := createTestUser(t, "phost@pickup.com", "pass", false)
	grantPickupHost(t, host.ID)
	organizer := createTestUser(t, "porg@pickup.com", "pass", false)
	solo := createTestUser(t, "psolo@pickup.com", "pass", false)
	late := createTestUser(t, "plate@pickup.com", "pass", false)

	hostToken := generateToken(host.ID)
	orgToken := generateToken(organizer.ID)
	soloToken := generateToken(solo.ID)
	lateToken := generateToken(late.ID)

	locationID := setupTestLocation(t, hostToken, host.ID)
	sportID, _ := getSportSkill(t, "BADMINTON", "A")
	start := time.Now().Add(24 * time.Hour)
	group := createGroup(t, hostToken, locationID, sportID, 2, 5, 100, start, start.Add(2*time.Hour))
	partyPath := fmt.Sprintf("/v1/pickup-groups/%s/party-orders", group.ID)

	members := []pickupHttp.OrderMemberBody{
		{Gender: "male", SkillLevel: 1},
		{Gender: "female", SkillLevel: 2},
		{Gender: "other", SkillLevel: 2},
	}

	t.Run("validation", func(t *testing.T) {
		// Too few members for the declared size.
		w := executeRequest("POST", partyPath, pickupHttp.CreatePartyOrderBody{OrganizerName: "Al", PartySize: 4, Members: members}, orgToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)

		// A party needs at least two seats (use the single enrollment otherwise).
		w = executeRequest("POST", partyPath, pickupHttp.CreatePartyOrderBody{OrganizerName: "Al", PartySize: 1, Members: members[:1]}, orgToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)

		// Unknown gender.
		bad := []pickupHttp.OrderMemberBody{{Gender: "male", SkillLevel: 1}, {Gender: "alien", SkillLevel: 1}}
		w = executeRequest("POST", partyPath, pickupHttp.CreatePartyOrderBody{OrganizerName: "Al", PartySize: 2, Members: bad}, orgToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)

		// Level not defined on the sport scale.
		undefined := []pickupHttp.OrderMemberBody{{Gender: "male", SkillLevel: 1}, {Gender: "male", SkillLevel: 99}}
		w = executeRequest("POST", partyPath, pickupHttp.CreatePartyOrderBody{OrganizerName: "Al", PartySize: 2, Members: undefined}, orgToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)

		// Missing organizer name.
		w = executeRequest("POST", partyPath, pickupHttp.CreatePartyOrderBody{PartySize: 3, Members: members}, orgToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)

		// Requires authentication.
		w = executeRequest("POST", partyPath, pickupHttp.CreatePartyOrderBody{OrganizerName: "Al", PartySize: 3, Members: members}, "")
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	// Give the solo enrollee a known gender and age so the stats are checkable.
	gender := "female"
	birth := time.Now().AddDate(-30, -1, 0).Format("2006-01-02")
	wMe := executeRequest("PATCH", "/v1/me", userHttp.UpdateMeRequest{Gender: &gender, BirthDate: &birth}, soloToken)
	require.Equal(t, http.StatusOK, wMe.Code, wMe.Body.String())

	var partyOrder pickupHttp.PickupOrderResponse
	t.Run("party occupies party_size seats", func(t *testing.T) {
		w := executeRequest("POST", partyPath, pickupHttp.CreatePartyOrderBody{OrganizerName: "Alice", PartySize: 3, Members: members}, orgToken)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &partyOrder))

		assert.Equal(t, 3, partyOrder.PartySize)
		assert.Equal(t, "Alice", partyOrder.BookerName)
		assert.Equal(t, 1, partyOrder.SkillLevel) // the organizer is Members[0]
		assert.Len(t, partyOrder.Members, 3)
		assert.Equal(t, "pending", partyOrder.Status)

		wGroup := executeRequest("GET", "/v1/pickup-groups/"+group.ID, nil, hostToken)
		var g pickupHttp.PickupGroupResponse
		require.NoError(t, json.Unmarshal(wGroup.Body.Bytes(), &g))
		assert.Equal(t, 3, g.CurrentEnrolled)
	})

	t.Run("whole party must fit", func(t *testing.T) {
		// 3 of 5 seats taken: another party of 3 does not fit.
		w := executeRequest("POST", partyPath, pickupHttp.CreatePartyOrderBody{OrganizerName: "Bob", PartySize: 3, Members: members}, soloToken)
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("same user cannot enroll twice", func(t *testing.T) {
		w := executeRequest("POST", partyPath, pickupHttp.CreatePartyOrderBody{OrganizerName: "Alice", PartySize: 2, Members: members[:2]}, orgToken)
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	soloOrder := enroll(t, soloToken, group.ID, 1)

	t.Run("single enrollment stores the self-reported level", func(t *testing.T) {
		assert.Equal(t, 1, soloOrder.PartySize)
		assert.Equal(t, 1, soloOrder.SkillLevel)
		assert.Empty(t, soloOrder.Members)
	})

	t.Run("single enrollment requires a defined skill level", func(t *testing.T) {
		wDelete := executeRequest("DELETE", "/v1/me/skill-levels/"+sportID, nil, lateToken)
		require.Equal(t, http.StatusNoContent, wDelete.Code)
		wNone := executeRequest("POST", fmt.Sprintf("/v1/pickup-groups/%s/orders", group.ID), nil, lateToken)
		assert.Equal(t, http.StatusBadRequest, wNone.Code)

		wBad := executeRequest("POST", fmt.Sprintf("/v1/pickup-groups/%s/orders", group.ID),
			map[string]int{"skill_level": 99}, lateToken)
		assert.Equal(t, http.StatusBadRequest, wBad.Code)
	})

	t.Run("group is full at 4 of 5 plus one more single", func(t *testing.T) {
		enroll(t, lateToken, group.ID, 2) // seat 5
		other := createTestUser(t, "pextra@pickup.com", "pass", false)
		w := executeRequest("POST", fmt.Sprintf("/v1/pickup-groups/%s/orders", group.ID), enrollBody(), generateToken(other.ID))
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("participant stats aggregate every seat anonymously", func(t *testing.T) {
		// No auth: the endpoint is public.
		w := executeRequest("GET", fmt.Sprintf("/v1/pickup-groups/%s/participant-stats", group.ID), nil, "")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var stats pickupHttp.ParticipantStatsResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &stats))

		assert.Equal(t, 5, stats.Total)

		genders := map[string]int{}
		for _, g := range stats.Genders {
			genders[g.Gender] = g.Count
		}
		// party: male, female, other; solo: female; late: no gender on profile.
		assert.Equal(t, map[string]int{"male": 1, "female": 2, "other": 1, "unknown": 1}, genders)

		ages := map[string]int{}
		for _, a := range stats.AgeGroups {
			ages[a.AgeGroup] = a.Count
		}
		assert.Equal(t, 1, ages[pickup.AgeGroup25To34])
		assert.Equal(t, 4, ages[pickup.AgeGroupUnknown]) // party members and a user without a birth date
		assert.Len(t, stats.AgeGroups, 7)

		levels := map[int]int{}
		labels := map[int]string{}
		for _, l := range stats.SkillLevels {
			levels[l.Level] = l.Count
			labels[l.Level] = l.Label
		}
		// party: 1,2,2; solo: 1; late: 2
		assert.Equal(t, 2, levels[1])
		assert.Equal(t, 3, levels[2])
		assert.Equal(t, 0, levels[3])
		assert.Equal(t, "A", labels[1])
		assert.Equal(t, "D", labels[4]) // every level of the sport scale is listed
	})

	t.Run("stats exclude cancelled orders", func(t *testing.T) {
		setOrderStatus(t, orgToken, partyOrder.ID, "cancelled")

		w := executeRequest("GET", fmt.Sprintf("/v1/pickup-groups/%s/participant-stats", group.ID), nil, "")
		var stats pickupHttp.ParticipantStatsResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &stats))
		assert.Equal(t, 2, stats.Total)

		// The freed seats are available again.
		wGroup := executeRequest("GET", "/v1/pickup-groups/"+group.ID, nil, hostToken)
		var g pickupHttp.PickupGroupResponse
		require.NoError(t, json.Unmarshal(wGroup.Body.Bytes(), &g))
		assert.Equal(t, 2, g.CurrentEnrolled)
	})

	t.Run("re-enrolling as a smaller party replaces the members", func(t *testing.T) {
		w := executeRequest("POST", partyPath, pickupHttp.CreatePartyOrderBody{OrganizerName: "Alice", PartySize: 2, Members: members[:2]}, orgToken)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

		var again pickupHttp.PickupOrderResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &again))
		assert.Equal(t, partyOrder.ID, again.ID) // the cancelled row is reused
		assert.Equal(t, 2, again.PartySize)
		assert.Len(t, again.Members, 2)

		wOrders := executeRequest("GET", fmt.Sprintf("/v1/pickup-groups/%s/orders", group.ID), nil, hostToken)
		require.Equal(t, http.StatusOK, wOrders.Code)
		var orders []pickupHttp.PickupOrderResponse
		require.NoError(t, json.Unmarshal(wOrders.Body.Bytes(), &orders))
		for _, o := range orders {
			if o.ID == partyOrder.ID {
				assert.Len(t, o.Members, 2, "old members must not linger after re-enrolling")
			}
		}
	})

	t.Run("stats for a missing group are 404", func(t *testing.T) {
		w := executeRequest("GET", "/v1/pickup-groups/00000000-0000-0000-0000-000000000000/participant-stats", nil, "")
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestSkillRatings(t *testing.T) {
	clearTables()

	host := createTestUser(t, "rhost@pickup.com", "pass", false)
	grantPickupHost(t, host.ID)
	player := createTestUser(t, "rplayer@pickup.com", "pass", false)
	pending := createTestUser(t, "rpending@pickup.com", "pass", false)
	viewer := createTestUser(t, "rviewer@pickup.com", "pass", false)

	hostToken := generateToken(host.ID)
	playerToken := generateToken(player.ID)
	pendingToken := generateToken(pending.ID)
	viewerToken := generateToken(viewer.ID)

	locationID := setupTestLocation(t, hostToken, host.ID)
	sportID, _ := getSportSkill(t, "BADMINTON", "A")
	start := time.Now().Add(24 * time.Hour)

	g1 := createGroup(t, hostToken, locationID, sportID, 2, 8, 0, start, start.Add(time.Hour))
	g2 := createGroup(t, hostToken, locationID, sportID, 2, 8, 0, start.Add(24*time.Hour), start.Add(25*time.Hour))

	playerG1 := enroll(t, playerToken, g1.ID, 2)
	playerG2 := enroll(t, playerToken, g2.ID, 2)
	pendingG1 := enroll(t, pendingToken, g1.ID, 2)
	setOrderStatus(t, hostToken, playerG1.ID, "confirmed")
	setOrderStatus(t, hostToken, playerG2.ID, "confirmed")
	_ = pendingG1 // stays pending

	ratePath := func(groupID, userID string) string {
		return fmt.Sprintf("/v1/pickup-groups/%s/ratings/%s", groupID, userID)
	}

	t.Run("cannot rate before the group has ended", func(t *testing.T) {
		w := executeRequest("PUT", ratePath(g1.ID, player.ID), skillRatingHttp.RateBody{SkillLevel: 3}, hostToken)
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	// Move both groups into the past (non-overlapping) and mark them completed.
	_, err := testPool.Exec(context.Background(),
		`UPDATE public.pickup_groups SET start_time = now() - interval '6 hours', end_time = now() - interval '5 hours', status = 'completed' WHERE id = $1`, g1.ID)
	require.NoError(t, err)
	_, err = testPool.Exec(context.Background(),
		`UPDATE public.pickup_groups SET start_time = now() - interval '4 hours', end_time = now() - interval '3 hours', status = 'completed' WHERE id = $1`, g2.ID)
	require.NoError(t, err)

	t.Run("only the host or an admin may rate", func(t *testing.T) {
		w := executeRequest("PUT", ratePath(g1.ID, player.ID), skillRatingHttp.RateBody{SkillLevel: 3}, viewerToken)
		assert.Equal(t, http.StatusForbidden, w.Code)

		w = executeRequest("PUT", ratePath(g1.ID, player.ID), skillRatingHttp.RateBody{SkillLevel: 3}, playerToken)
		assert.Equal(t, http.StatusForbidden, w.Code)

		w = executeRequest("GET", fmt.Sprintf("/v1/pickup-groups/%s/ratings", g1.ID), nil, playerToken)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("only confirmed participants can be rated", func(t *testing.T) {
		w := executeRequest("PUT", ratePath(g1.ID, pending.ID), skillRatingHttp.RateBody{SkillLevel: 3}, hostToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)

		// Someone who never enrolled.
		w = executeRequest("PUT", ratePath(g1.ID, viewer.ID), skillRatingHttp.RateBody{SkillLevel: 3}, hostToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("level must exist on the sport scale", func(t *testing.T) {
		w := executeRequest("PUT", ratePath(g1.ID, player.ID), skillRatingHttp.RateBody{SkillLevel: 99}, hostToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)

		w = executeRequest("PUT", ratePath(g1.ID, player.ID), skillRatingHttp.RateBody{SkillLevel: 0}, hostToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("rating overwrites per group", func(t *testing.T) {
		w := executeRequest("PUT", ratePath(g1.ID, player.ID), skillRatingHttp.RateBody{SkillLevel: 1}, hostToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		w = executeRequest("PUT", ratePath(g1.ID, player.ID), skillRatingHttp.RateBody{SkillLevel: 2}, hostToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		wList := executeRequest("GET", fmt.Sprintf("/v1/pickup-groups/%s/ratings", g1.ID), nil, hostToken)
		require.Equal(t, http.StatusOK, wList.Code)
		var ratings []skillRatingHttp.RatingResponse
		require.NoError(t, json.Unmarshal(wList.Body.Bytes(), &ratings))
		require.Len(t, ratings, 1)
		assert.Equal(t, 2, ratings[0].SkillLevel)
		assert.Equal(t, player.ID, ratings[0].UserID)
	})

	t.Run("composite rating is the average across groups", func(t *testing.T) {
		w := executeRequest("PUT", ratePath(g2.ID, player.ID), skillRatingHttp.RateBody{SkillLevel: 4}, hostToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		// Any authenticated user can read a user's composite rating.
		wSum := executeRequest("GET", fmt.Sprintf("/v1/users/%s/skill-ratings", player.ID), nil, viewerToken)
		require.Equal(t, http.StatusOK, wSum.Code, wSum.Body.String())
		var summary []skillRatingHttp.SkillSummaryResponse
		require.NoError(t, json.Unmarshal(wSum.Body.Bytes(), &summary))
		require.Len(t, summary, 1)
		assert.Equal(t, "BADMINTON", summary[0].Sport.Code)
		assert.InDelta(t, 3.0, summary[0].Average, 1e-9) // (2 + 4) / 2
		assert.Equal(t, 2, summary[0].RatingCount)
		assert.Equal(t, 3, summary[0].Level)
		assert.Equal(t, "C", summary[0].Label)

		// The caller's own view matches.
		wMe := executeRequest("GET", "/v1/me/skill-ratings", nil, playerToken)
		require.Equal(t, http.StatusOK, wMe.Code)
		var mine []skillRatingHttp.SkillSummaryResponse
		require.NoError(t, json.Unmarshal(wMe.Body.Bytes(), &mine))
		assert.Equal(t, summary, mine)
	})

	t.Run("unrated users have an empty composite rating", func(t *testing.T) {
		w := executeRequest("GET", fmt.Sprintf("/v1/users/%s/skill-ratings", viewer.ID), nil, viewerToken)
		require.Equal(t, http.StatusOK, w.Code)
		var summary []skillRatingHttp.SkillSummaryResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &summary))
		assert.Empty(t, summary)

		wMissing := executeRequest("GET", "/v1/users/00000000-0000-0000-0000-000000000000/skill-ratings", nil, viewerToken)
		assert.Equal(t, http.StatusNotFound, wMissing.Code)

		wAnon := executeRequest("GET", fmt.Sprintf("/v1/users/%s/skill-ratings", player.ID), nil, "")
		assert.Equal(t, http.StatusUnauthorized, wAnon.Code)
	})

	t.Run("participant is notified of a rating", func(t *testing.T) {
		resp := listNotifications(t, playerToken, "")
		var rated int
		for _, n := range resp.Items {
			if n.Type == "skill_rated" {
				rated++
			}
		}
		assert.Equal(t, 3, rated) // three PUTs by the host succeeded
	})

	t.Run("removing a rating updates the average", func(t *testing.T) {
		w := executeRequest("DELETE", ratePath(g2.ID, player.ID), nil, hostToken)
		assert.Equal(t, http.StatusNoContent, w.Code)

		wAgain := executeRequest("DELETE", ratePath(g2.ID, player.ID), nil, hostToken)
		assert.Equal(t, http.StatusNotFound, wAgain.Code)

		wSum := executeRequest("GET", "/v1/me/skill-ratings", nil, playerToken)
		var summary []skillRatingHttp.SkillSummaryResponse
		require.NoError(t, json.Unmarshal(wSum.Body.Bytes(), &summary))
		require.Len(t, summary, 1)
		assert.Equal(t, 1, summary[0].RatingCount)
		assert.InDelta(t, 2.0, summary[0].Average, 1e-9)
	})
}

func TestNotifications(t *testing.T) {
	clearTables()

	host := createTestUser(t, "nhost@pickup.com", "pass", false)
	grantPickupHost(t, host.ID)
	user1 := createTestUser(t, "nuser1@pickup.com", "pass", false)
	user2 := createTestUser(t, "nuser2@pickup.com", "pass", false)

	hostToken := generateToken(host.ID)
	user1Token := generateToken(user1.ID)
	user2Token := generateToken(user2.ID)

	locationID := setupTestLocation(t, hostToken, host.ID)
	sportID, _ := getSportSkill(t, "BADMINTON", "A")
	start := time.Now().Add(24 * time.Hour)
	group := createGroup(t, hostToken, locationID, sportID, 1, 8, 0, start, start.Add(2*time.Hour))

	t.Run("requires authentication", func(t *testing.T) {
		assert.Equal(t, http.StatusUnauthorized, executeRequest("GET", "/v1/notifications", nil, "").Code)
		assert.Equal(t, http.StatusUnauthorized, executeRequest("GET", "/v1/notifications/unread-count", nil, "").Code)
	})

	t.Run("empty inbox", func(t *testing.T) {
		resp := listNotifications(t, hostToken, "")
		assert.Equal(t, 0, resp.Total)
		assert.NotNil(t, resp.Items)
	})

	order1 := enroll(t, user1Token, group.ID, 1)

	t.Run("host is notified of a new enrollment", func(t *testing.T) {
		resp := listNotifications(t, hostToken, "")
		require.Equal(t, 1, resp.Total)
		n := resp.Items[0]
		assert.Equal(t, "pickup_order_created", n.Type)
		assert.False(t, n.IsRead)
		require.NotNil(t, n.PickupGroupID)
		assert.Equal(t, group.ID, *n.PickupGroupID)
		require.NotNil(t, n.PickupOrderID)
		assert.Equal(t, order1.ID, *n.PickupOrderID)
		assert.NotEmpty(t, n.Title)
		assert.NotEmpty(t, n.Content)

		// The enrollee is not notified of their own action.
		assert.Equal(t, 0, listNotifications(t, user1Token, "").Total)
	})

	t.Run("booker is notified when the host confirms", func(t *testing.T) {
		setOrderStatus(t, hostToken, order1.ID, "confirmed")

		resp := listNotifications(t, user1Token, "")
		require.Equal(t, 1, resp.Total)
		assert.Equal(t, "pickup_order_confirmed", resp.Items[0].Type)
	})

	t.Run("host is notified when the booker cancels", func(t *testing.T) {
		order2 := enroll(t, user2Token, group.ID, 1)
		setOrderStatus(t, user2Token, order2.ID, "cancelled")

		resp := listNotifications(t, hostToken, "")
		types := map[string]int{}
		for _, n := range resp.Items {
			types[n.Type]++
		}
		assert.Equal(t, 2, types["pickup_order_created"])
		assert.Equal(t, 1, types["pickup_order_cancelled"])
	})

	t.Run("payment status change notifies the booker", func(t *testing.T) {
		done := "done"
		w := executeRequest("PATCH", "/v1/pickup-orders/"+order1.ID, pickupHttp.UpdateOrderBody{PaymentStatus: &done}, hostToken)
		require.Equal(t, http.StatusOK, w.Code)

		resp := listNotifications(t, user1Token, "")
		var found bool
		for _, n := range resp.Items {
			if n.Type == "pickup_payment_updated" {
				found = true
			}
		}
		assert.True(t, found)
	})

	t.Run("unread count, mark read, and unread filter", func(t *testing.T) {
		count := func(token string) int {
			w := executeRequest("GET", "/v1/notifications/unread-count", nil, token)
			require.Equal(t, http.StatusOK, w.Code)
			var resp notificationHttp.UnreadCountResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			return resp.Unread
		}

		before := count(user1Token)
		require.Equal(t, 2, before) // confirmed + payment

		list := listNotifications(t, user1Token, "")
		first := list.Items[0].ID

		w := executeRequest("PATCH", "/v1/notifications/"+first+"/read", nil, user1Token)
		assert.Equal(t, http.StatusNoContent, w.Code)
		assert.Equal(t, before-1, count(user1Token))

		// Marking again is idempotent.
		w = executeRequest("PATCH", "/v1/notifications/"+first+"/read", nil, user1Token)
		assert.Equal(t, http.StatusNoContent, w.Code)

		unread := listNotifications(t, user1Token, "?unread_only=true")
		assert.Equal(t, before-1, unread.Total)
		for _, n := range unread.Items {
			assert.False(t, n.IsRead)
		}

		// Another user's notification looks like it does not exist.
		w = executeRequest("PATCH", "/v1/notifications/"+first+"/read", nil, user2Token)
		assert.Equal(t, http.StatusNotFound, w.Code)
		w = executeRequest("PATCH", "/v1/notifications/not-a-uuid/read", nil, user1Token)
		assert.Equal(t, http.StatusBadRequest, w.Code)

		wAll := executeRequest("POST", "/v1/notifications/read-all", nil, user1Token)
		require.Equal(t, http.StatusOK, wAll.Code)
		var readAll notificationHttp.MarkAllReadResponse
		require.NoError(t, json.Unmarshal(wAll.Body.Bytes(), &readAll))
		assert.EqualValues(t, before-1, readAll.Updated)
		assert.Equal(t, 0, count(user1Token))
	})

	t.Run("enrolled users are notified when the group is cancelled", func(t *testing.T) {
		status := "cancelled"
		w := executeRequest("PATCH", "/v1/pickup-groups/"+group.ID, pickupHttp.UpdateGroupBody{Status: &status}, hostToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		resp := listNotifications(t, user1Token, "?unread_only=true")
		require.Equal(t, 1, resp.Total)
		assert.Equal(t, "pickup_group_cancelled", resp.Items[0].Type)

		// user2 cancelled their own order earlier, so they hold no seat.
		for _, n := range listNotifications(t, user2Token, "").Items {
			assert.NotEqual(t, "pickup_group_cancelled", n.Type)
		}
	})

	t.Run("rescheduling notifies enrolled users", func(t *testing.T) {
		g := createGroup(t, hostToken, locationID, sportID, 1, 8, 0, start.Add(48*time.Hour), start.Add(50*time.Hour))
		enroll(t, user2Token, g.ID, 1)

		newStart := start.Add(52 * time.Hour)
		newEnd := start.Add(54 * time.Hour)
		w := executeRequest("PATCH", "/v1/pickup-groups/"+g.ID, pickupHttp.UpdateGroupBody{StartTime: &newStart, EndTime: &newEnd}, hostToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var found bool
		for _, n := range listNotifications(t, user2Token, "").Items {
			if n.Type == "pickup_group_updated" {
				found = true
			}
		}
		assert.True(t, found)
	})
}

func TestSkillLevelMapping(t *testing.T) {
	clearTables()

	admin := createTestUser(t, "sladmin@pickup.com", "pass", true)
	adminToken := generateToken(admin.ID)

	wSport := executeRequest("POST", "/v1/sports", map[string]string{
		"code": fmt.Sprintf("MAP_%d", time.Now().UnixNano()%1000000), "name": "Mapping Sport"}, adminToken)
	require.Equal(t, http.StatusCreated, wSport.Code, wSport.Body.String())
	var sport struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(wSport.Body.Bytes(), &sport))

	create := func(level int, label string) *httpRecorder {
		w := executeRequest("POST", "/v1/skill-levels", map[string]any{"sport_id": sport.ID, "level": level, "label": label}, adminToken)
		return &httpRecorder{code: w.Code, body: w.Body.String()}
	}

	t.Run("each sport defines its own scale", func(t *testing.T) {
		assert.Equal(t, http.StatusCreated, create(1, "Newbie").code)
		assert.Equal(t, http.StatusCreated, create(2, "Beginner").code)
		assert.Equal(t, http.StatusCreated, create(5, "Master").code) // gaps are allowed
	})

	t.Run("duplicate level or label is a conflict", func(t *testing.T) {
		assert.Equal(t, http.StatusConflict, create(2, "Another").code)
		assert.Equal(t, http.StatusConflict, create(3, "Beginner").code)
	})

	t.Run("level must be positive", func(t *testing.T) {
		assert.Equal(t, http.StatusBadRequest, create(0, "Zero").code)
		assert.Equal(t, http.StatusBadRequest, create(-1, "Negative").code)
	})

	t.Run("listing is ordered by level", func(t *testing.T) {
		w := executeRequest("GET", "/v1/skill-levels?sport_id="+sport.ID, nil, "")
		require.Equal(t, http.StatusOK, w.Code)
		var resp struct {
			Items []struct {
				ID    string `json:"id"`
				Level int    `json:"level"`
				Label string `json:"label"`
			} `json:"items"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Len(t, resp.Items, 3)
		assert.Equal(t, []int{1, 2, 5}, []int{resp.Items[0].Level, resp.Items[1].Level, resp.Items[2].Level})

		// The label can be renamed, but the level itself is fixed.
		wPatch := executeRequest("PATCH", "/v1/skill-levels/"+resp.Items[0].ID, map[string]any{"label": "Rookie", "level": 9}, adminToken)
		require.Equal(t, http.StatusOK, wPatch.Code)
		var patched struct {
			Level int    `json:"level"`
			Label string `json:"label"`
		}
		require.NoError(t, json.Unmarshal(wPatch.Body.Bytes(), &patched))
		assert.Equal(t, 1, patched.Level)
		assert.Equal(t, "Rookie", patched.Label)
	})
}

// httpRecorder is a tiny snapshot of a response used by table-style helpers.
type httpRecorder struct {
	code int
	body string
}
