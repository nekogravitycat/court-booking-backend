package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	pickupHttp "github.com/nekogravitycat/court-booking-backend/internal/pickup/http"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
	userHttp "github.com/nekogravitycat/court-booking-backend/internal/user/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccountSkillLevelsAndUserHostedGroups(t *testing.T) {
	clearTables()
	host := createTestUser(t, "ordinary_host@skill.com", "pass", false)
	player := createTestUser(t, "player@skill.com", "pass", false)
	other := createTestUser(t, "other@skill.com", "pass", false)
	hostToken, playerToken, otherToken := generateToken(host.ID), generateToken(player.ID), generateToken(other.ID)
	sportID, _ := getSportSkill(t, "BADMINTON", "A")
	secondSport, _ := getSportSkill(t, "BASKETBALL", "A")
	path := "/v1/me/skill-levels/" + sportID
	_, err := testPool.Exec(context.Background(), "DELETE FROM public.user_skill_levels WHERE user_id = $1", player.ID)
	require.NoError(t, err)

	t.Run("authentication and validation", func(t *testing.T) {
		for _, method := range []string{"GET", "PUT", "DELETE"} {
			assert.Equal(t, http.StatusUnauthorized, executeRequest(method, path, map[string]int{"skill_level": 2}, "").Code)
		}
		assert.Equal(t, http.StatusUnauthorized, executeRequest("GET", "/v1/me/skill-levels", nil, "").Code)
		for _, value := range []any{nil, map[string]int{}, map[string]int{"skill_level": 0}, map[string]int{"skill_level": -1}, map[string]int{"skill_level": 101}, map[string]int{"skill_level": 99}, map[string]string{"skill_level": "2"}} {
			assert.Equal(t, http.StatusBadRequest, executeRequest("PUT", path, value, playerToken).Code)
		}
		assert.Equal(t, http.StatusBadRequest, executeRequest("PUT", "/v1/me/skill-levels/not-uuid", map[string]int{"skill_level": 2}, playerToken).Code)
		assert.Equal(t, http.StatusNotFound, executeRequest("PUT", "/v1/me/skill-levels/"+uuid.NewString(), map[string]int{"skill_level": 2}, playerToken).Code)
		w := executeRequest("GET", "/v1/me/skill-levels", nil, playerToken)
		var page response.PageResponse[userHttp.SportSkillLevelResponse]
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
		assert.Empty(t, page.Items)
	})

	// This account has no pickup host role, yet can create a group.
	locationID := setupTestLocation(t, hostToken, host.ID)
	start := time.Now().Add(24 * time.Hour)
	group := createGroup(t, hostToken, locationID, sportID, 2, 8, 100, start, start.Add(time.Hour))
	orderPath := "/v1/pickup-groups/" + group.ID + "/orders"
	assert.False(t, host.IsPickupHost)
	assert.Equal(t, host.ID, group.Host.ID)

	t.Run("missing account declaration cannot be bypassed", func(t *testing.T) {
		for _, body := range []any{nil, map[string]int{"skill_level": 2}} {
			w := executeRequest("POST", orderPath, body, playerToken)
			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, w.Body.String(), "skill level not set")
		}
		assert.Equal(t, http.StatusBadRequest, executeRequest("GET", path, nil, playerToken).Code)
	})

	set := func(sport string, level int) {
		t.Helper()
		w := executeRequest("PUT", "/v1/me/skill-levels/"+sport, userHttp.SetSkillLevelBody{SkillLevel: level}, playerToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}
	set(sportID, 2)
	set(secondSport, 3)
	t.Run("per sport settings are isolated by account", func(t *testing.T) {
		w := executeRequest("GET", "/v1/me/skill-levels", nil, playerToken)
		var page response.PageResponse[userHttp.SportSkillLevelResponse]
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
		require.Len(t, page.Items, 2)
		assert.Equal(t, 2, page.Total)
		wOther := executeRequest("GET", path, nil, otherToken)
		var level userHttp.SportSkillLevelResponse
		require.NoError(t, json.Unmarshal(wOther.Body.Bytes(), &level))
		assert.Equal(t, 1, level.SkillLevel)
	})

	w := executeRequest("POST", orderPath, map[string]int{"skill_level": 99}, playerToken)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var order pickupHttp.PickupOrderResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &order))
	assert.Equal(t, 2, order.SkillLevel)
	set(sportID, 3)

	t.Run("snapshots and creator permissions", func(t *testing.T) {
		title := "Updated by ordinary creator"
		groupPath := "/v1/pickup-groups/" + group.ID
		assert.Equal(t, http.StatusOK, executeRequest("PATCH", groupPath, pickupHttp.UpdateGroupBody{Title: &title}, hostToken).Code)
		assert.Equal(t, http.StatusForbidden, executeRequest("PATCH", groupPath, pickupHttp.UpdateGroupBody{Title: &title}, otherToken).Code)
		assert.Equal(t, http.StatusForbidden, executeRequest("DELETE", groupPath, nil, hostToken).Code)
		assert.Equal(t, http.StatusForbidden, executeRequest("GET", orderPath, nil, otherToken).Code)
		w := executeRequest("GET", orderPath, nil, hostToken)
		require.Equal(t, http.StatusOK, w.Code)
		var orders []pickupHttp.PickupOrderResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &orders))
		require.Len(t, orders, 1)
		assert.Equal(t, 2, orders[0].SkillLevel)
		setOrderStatus(t, hostToken, order.ID, "confirmed")
		setOrderStatus(t, playerToken, order.ID, "cancelled")
		w = executeRequest("POST", orderPath, nil, playerToken)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		var reenrolled pickupHttp.PickupOrderResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &reenrolled))
		assert.Equal(t, 3, reenrolled.SkillLevel)
		assert.Equal(t, order.ID, reenrolled.ID)
	})

	t.Run("party organizer uses account level; companions still require levels", func(t *testing.T) {
		group2 := createGroup(t, hostToken, locationID, sportID, 2, 8, 100, start.Add(3*time.Hour), start.Add(4*time.Hour))
		partyPath := "/v1/pickup-groups/" + group2.ID + "/party-orders"
		body := pickupHttp.CreatePartyOrderBody{OrganizerName: "Player", PartySize: 2,
			Members: []pickupHttp.OrderMemberBody{{Gender: "male"}, {Gender: "female"}}}
		assert.Equal(t, http.StatusBadRequest, executeRequest("POST", partyPath, body, playerToken).Code)
		body.Members[1].SkillLevel = 1
		w := executeRequest("POST", partyPath, body, playerToken)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		var party pickupHttp.PickupOrderResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &party))
		assert.Equal(t, 3, party.SkillLevel)
		require.Len(t, party.Members, 2)
		assert.Equal(t, 3, party.Members[0].SkillLevel)
		assert.Equal(t, 1, party.Members[1].SkillLevel)
	})

	t.Run("inactive levels and sports cannot be used", func(t *testing.T) {
		_, err := testPool.Exec(context.Background(), "UPDATE public.skill_levels SET is_active = false WHERE sport_id = $1 AND level = 3", sportID)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = testPool.Exec(context.Background(), "UPDATE public.skill_levels SET is_active = true WHERE sport_id = $1 AND level = 3", sportID)
		})
		assert.Equal(t, http.StatusBadRequest, executeRequest("PUT", path, userHttp.SetSkillLevelBody{SkillLevel: 3}, playerToken).Code)
		assert.Equal(t, http.StatusBadRequest, executeRequest("POST", orderPath, nil, playerToken).Code)
		_, err = testPool.Exec(context.Background(), "UPDATE public.sports SET is_active = false WHERE id = $1", secondSport)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = testPool.Exec(context.Background(), "UPDATE public.sports SET is_active = true WHERE id = $1", secondSport)
		})
		assert.Equal(t, http.StatusBadRequest, executeRequest("PUT", "/v1/me/skill-levels/"+secondSport, userHttp.SetSkillLevelBody{SkillLevel: 2}, playerToken).Code)
	})

	t.Run("clear is idempotent and preserves orders", func(t *testing.T) {
		for range 2 {
			assert.Equal(t, http.StatusNoContent, executeRequest("DELETE", path, nil, playerToken).Code)
		}
		assert.Equal(t, http.StatusBadRequest, executeRequest("GET", path, nil, playerToken).Code)
		var saved int
		require.NoError(t, testPool.QueryRow(context.Background(), "SELECT skill_level FROM public.pickup_orders WHERE id = $1", order.ID).Scan(&saved))
		assert.Equal(t, 3, saved)
	})
}
