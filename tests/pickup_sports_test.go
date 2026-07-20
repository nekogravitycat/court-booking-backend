package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pickupHttp "github.com/nekogravitycat/court-booking-backend/internal/pickup/http"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
	skillHttp "github.com/nekogravitycat/court-booking-backend/internal/skilllevel/http"
	sportsHttp "github.com/nekogravitycat/court-booking-backend/internal/sports/http"
	userHttp "github.com/nekogravitycat/court-booking-backend/internal/user/http"
)

func TestSportsAndSkillLevelAdmin(t *testing.T) {
	clearTables()

	admin := createTestUser(t, "sportadmin@example.com", "pass", true)
	regular := createTestUser(t, "sportuser@example.com", "pass", false)
	adminToken := generateToken(admin.ID)
	regularToken := generateToken(regular.ID)

	t.Run("Public list sports returns seeded catalog", func(t *testing.T) {
		w := executeRequest("GET", "/v1/sports", nil, "")
		require.Equal(t, http.StatusOK, w.Code)

		var resp response.PageResponse[sportsHttp.SportResponse]
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.GreaterOrEqual(t, resp.Total, 3) // BADMINTON / BASKETBALL / VOLLEYBALL
	})

	// Unique code so repeated go-test invocations do not collide on the
	// persisted sports catalog.
	code := fmt.Sprintf("test_%d", time.Now().UnixNano())
	var newSportID string

	t.Run("Create sport requires system admin", func(t *testing.T) {
		body := sportsHttp.CreateSportBody{Code: code, Name: "Test Sport"}

		wForbidden := executeRequest("POST", "/v1/sports", body, regularToken)
		assert.Equal(t, http.StatusForbidden, wForbidden.Code)

		w := executeRequest("POST", "/v1/sports", body, adminToken)
		require.Equal(t, http.StatusCreated, w.Code)

		var resp sportsHttp.SportResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "TEST_"+code[5:], resp.Code) // code is uppercased on create
		assert.True(t, resp.IsActive)
		newSportID = resp.ID
	})

	t.Run("Create and list skill levels scoped to sport", func(t *testing.T) {
		body := skillHttp.CreateSkillLevelBody{SportID: newSportID, Name: "Beginner", SortOrder: 1}
		w := executeRequest("POST", "/v1/skill-levels", body, adminToken)
		require.Equal(t, http.StatusCreated, w.Code)

		w = executeRequest("GET", "/v1/skill-levels?sport_id="+newSportID, nil, "")
		require.Equal(t, http.StatusOK, w.Code)

		var resp response.PageResponse[skillHttp.SkillLevelResponse]
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 1, resp.Total)
		assert.Equal(t, "Beginner", resp.Items[0].Name)
		assert.Equal(t, newSportID, resp.Items[0].SportID)
	})
}

func TestPickupEnrolledStatusAndOrderDelete(t *testing.T) {
	clearTables()

	host := createTestUser(t, "ehost@pickup.com", "pass", false)
	grantPickupHost(t, host.ID)
	user1 := createTestUser(t, "euser1@pickup.com", "pass", false)
	admin := createTestUser(t, "eadmin@pickup.com", "pass", true)

	hostToken := generateToken(host.ID)
	user1Token := generateToken(user1.ID)
	adminToken := generateToken(admin.ID)

	locationID := setupTestLocation(t, hostToken, host.ID)
	sportID, skillID := getSportSkill(t, "BADMINTON", "A")

	payload := pickupHttp.CreateGroupBody{
		Title:        "Enroll Group",
		StartTime:    time.Now().Add(24 * time.Hour),
		EndTime:      time.Now().Add(26 * time.Hour),
		Fee:          50,
		Capacity:     4,
		LocationID:   locationID,
		SportID:      sportID,
		SkillLevelID: skillID,
	}
	w := executeRequest("POST", "/v1/pickup-groups", payload, hostToken)
	require.Equal(t, http.StatusCreated, w.Code)
	var groupResp pickupHttp.PickupGroupResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &groupResp))
	groupID := groupResp.ID

	// Host details are resolved via JOIN (no snapshot).
	assert.Equal(t, host.ID, groupResp.Host.ID)
	assert.NotEmpty(t, groupResp.Host.Username)

	briefFor := func(token string) pickupHttp.PickupGroupBrief {
		wl := executeRequest("GET", "/v1/pickup-groups", nil, token)
		require.Equal(t, http.StatusOK, wl.Code)
		var resp response.PageResponse[pickupHttp.PickupGroupBrief]
		require.NoError(t, json.Unmarshal(wl.Body.Bytes(), &resp))
		for _, it := range resp.Items {
			if it.ID == groupID {
				return it
			}
		}
		t.Fatalf("group %s not found in public list", groupID)
		return pickupHttp.PickupGroupBrief{}
	}

	t.Run("enrolled_status is free before enrolling and anonymous", func(t *testing.T) {
		assert.Equal(t, "free", briefFor(user1Token).EnrolledStatus)

		anon := briefFor("")
		assert.Equal(t, "free", anon.EnrolledStatus)
		assert.Equal(t, host.ID, anon.HostID)
		assert.NotEmpty(t, anon.HostUsername)
	})

	var orderID string
	t.Run("enrolled_status reflects the viewer's order", func(t *testing.T) {
		wo := executeRequest("POST", fmt.Sprintf("/v1/pickup-groups/%s/orders", groupID), nil, user1Token)
		require.Equal(t, http.StatusCreated, wo.Code)
		var o pickupHttp.PickupOrderResponse
		require.NoError(t, json.Unmarshal(wo.Body.Bytes(), &o))
		orderID = o.ID

		assert.Equal(t, "pending", briefFor(user1Token).EnrolledStatus)
		// A different (anonymous) viewer still sees free.
		assert.Equal(t, "free", briefFor("").EnrolledStatus)
	})

	strPtr := func(s string) *string { return &s }

	groupEnrolled := func(token string) int {
		wg := executeRequest("GET", "/v1/pickup-groups/"+groupID, nil, token)
		require.Equal(t, http.StatusOK, wg.Code)
		var g pickupHttp.PickupGroupResponse
		require.NoError(t, json.Unmarshal(wg.Body.Bytes(), &g))
		return g.CurrentEnrolled
	}

	t.Run("booker cannot hard-delete an order", func(t *testing.T) {
		w := executeRequest("DELETE", "/v1/pickup-orders/"+orderID, nil, user1Token)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("host cannot hard-delete an order (admin only)", func(t *testing.T) {
		w := executeRequest("DELETE", "/v1/pickup-orders/"+orderID, nil, hostToken)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("cancelled user can re-enroll (reuses the same row)", func(t *testing.T) {
		require.Equal(t, 1, groupEnrolled(hostToken))

		wc := executeRequest("PATCH", "/v1/pickup-orders/"+orderID, pickupHttp.UpdateOrderBody{Status: strPtr("cancelled")}, user1Token)
		require.Equal(t, http.StatusOK, wc.Code)
		assert.Equal(t, "cancelled", briefFor(user1Token).EnrolledStatus)
		assert.Equal(t, 0, groupEnrolled(hostToken))

		wr := executeRequest("POST", fmt.Sprintf("/v1/pickup-groups/%s/orders", groupID), nil, user1Token)
		require.Equal(t, http.StatusCreated, wr.Code)
		var o pickupHttp.PickupOrderResponse
		require.NoError(t, json.Unmarshal(wr.Body.Bytes(), &o))
		// Re-enrollment reuses the existing row (unique group+user), so id is stable.
		assert.Equal(t, orderID, o.ID)
		assert.Equal(t, "pending", briefFor(user1Token).EnrolledStatus)
		assert.Equal(t, 1, groupEnrolled(hostToken))
	})

	t.Run("cancel_request still occupies the seat", func(t *testing.T) {
		wc := executeRequest("PATCH", "/v1/pickup-orders/"+orderID, pickupHttp.UpdateOrderBody{Status: strPtr("cancel_request")}, user1Token)
		require.Equal(t, http.StatusOK, wc.Code)
		assert.Equal(t, "cancel_request", briefFor(user1Token).EnrolledStatus)
		// The seat is held until the order is actually cancelled.
		assert.Equal(t, 1, groupEnrolled(hostToken))

		// Re-enrolling while in cancel_request is a duplicate, not a fresh enroll.
		wr := executeRequest("POST", fmt.Sprintf("/v1/pickup-groups/%s/orders", groupID), nil, user1Token)
		assert.Equal(t, http.StatusConflict, wr.Code)

		// Restore to pending (reviewer-only) for the subsequent reject subtest.
		wp := executeRequest("PATCH", "/v1/pickup-orders/"+orderID, pickupHttp.UpdateOrderBody{Status: strPtr("pending")}, hostToken)
		require.Equal(t, http.StatusOK, wp.Code)
		assert.Equal(t, 1, groupEnrolled(hostToken))
	})

	t.Run("booker cannot self-reject", func(t *testing.T) {
		w := executeRequest("PATCH", "/v1/pickup-orders/"+orderID, pickupHttp.UpdateOrderBody{Status: strPtr("rejected")}, user1Token)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("host rejects an order and current_enrolled decrements", func(t *testing.T) {
		wj := executeRequest("PATCH", "/v1/pickup-orders/"+orderID, pickupHttp.UpdateOrderBody{Status: strPtr("rejected")}, hostToken)
		require.Equal(t, http.StatusOK, wj.Code)
		var o pickupHttp.PickupOrderResponse
		require.NoError(t, json.Unmarshal(wj.Body.Bytes(), &o))
		assert.Equal(t, "rejected", o.Status)

		assert.Equal(t, "rejected", briefFor(user1Token).EnrolledStatus)
		assert.Equal(t, 0, groupEnrolled(hostToken))
	})

	t.Run("rejected user cannot re-enroll", func(t *testing.T) {
		w := executeRequest("POST", fmt.Sprintf("/v1/pickup-groups/%s/orders", groupID), nil, user1Token)
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("system admin can hard-delete an order", func(t *testing.T) {
		wDel := executeRequest("DELETE", "/v1/pickup-orders/"+orderID, nil, adminToken)
		assert.Equal(t, http.StatusNoContent, wDel.Code)
	})

	t.Run("skill level must belong to the selected sport", func(t *testing.T) {
		badmintonSport, _ := getSportSkill(t, "BADMINTON", "A")
		_, volleyballSkill := getSportSkill(t, "VOLLEYBALL", "A")

		mismatch := pickupHttp.CreateGroupBody{
			Title:        "Mismatch Group",
			StartTime:    time.Now().Add(24 * time.Hour),
			EndTime:      time.Now().Add(26 * time.Hour),
			Fee:          0,
			Capacity:     2,
			LocationID:   locationID,
			SportID:      badmintonSport,
			SkillLevelID: volleyballSkill,
		}
		wm := executeRequest("POST", "/v1/pickup-groups", mismatch, hostToken)
		assert.Equal(t, http.StatusBadRequest, wm.Code)
	})
}

func TestUsernameRules(t *testing.T) {
	clearTables()

	t.Run("uppercase username is lowercased", func(t *testing.T) {
		body := userHttp.RegisterRequest{Email: "u1@ex.com", Username: "CoolUser_1", Password: "password123", DisplayName: "U"}
		w := executeRequest("POST", "/v1/auth/register", body, "")
		require.Equal(t, http.StatusCreated, w.Code)

		var resp userHttp.MeResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "cooluser_1", resp.User.Username)
	})

	t.Run("too-short username is rejected", func(t *testing.T) {
		body := userHttp.RegisterRequest{Email: "u2@ex.com", Username: "ab", Password: "password123", DisplayName: "U"}
		w := executeRequest("POST", "/v1/auth/register", body, "")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid characters are rejected", func(t *testing.T) {
		body := userHttp.RegisterRequest{Email: "u3@ex.com", Username: "bad-name!", Password: "password123", DisplayName: "U"}
		w := executeRequest("POST", "/v1/auth/register", body, "")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("duplicate username (case-insensitive) conflicts", func(t *testing.T) {
		first := userHttp.RegisterRequest{Email: "u4@ex.com", Username: "dupuser", Password: "password123", DisplayName: "U"}
		w := executeRequest("POST", "/v1/auth/register", first, "")
		require.Equal(t, http.StatusCreated, w.Code)

		second := userHttp.RegisterRequest{Email: "u5@ex.com", Username: "DupUser", Password: "password123", DisplayName: "U"}
		w2 := executeRequest("POST", "/v1/auth/register", second, "")
		assert.Equal(t, http.StatusConflict, w2.Code)
	})
}
