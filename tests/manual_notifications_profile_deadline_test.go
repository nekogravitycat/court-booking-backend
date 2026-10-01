package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	pickupHttp "github.com/nekogravitycat/court-booking-backend/internal/pickup/http"
	userHttp "github.com/nekogravitycat/court-booking-backend/internal/user/http"
	"github.com/stretchr/testify/require"
)

func TestManualNotifications(t *testing.T) {
	clearTables()
	admin := createTestUser(t, "sendadmin@test.com", "pass", true)
	a := createTestUser(t, "senda@test.com", "pass", false)
	b := createTestUser(t, "sendb@test.com", "pass", false)
	token := generateToken(admin.ID)
	payload := map[string]any{"user_ids": []string{a.ID, b.ID}, "title": "Hello", "content": "Test message"}
	require.Equal(t, http.StatusUnauthorized, executeRequest("POST", "/v1/notifications", payload, "").Code)
	require.Equal(t, http.StatusForbidden, executeRequest("POST", "/v1/notifications", payload, generateToken(a.ID)).Code)
	require.Equal(t, http.StatusCreated, executeRequest("POST", "/v1/notifications", payload, token).Code)
	inbox := listNotifications(t, generateToken(a.ID), "")
	require.Len(t, inbox.Items, 1)
	require.Equal(t, "admin_message", inbox.Items[0].Type)
	require.Equal(t, "Test message", inbox.Items[0].Content)
	require.Nil(t, inbox.Items[0].PickupGroupID)
	require.Len(t, listNotifications(t, token, "").Items, 0)
	for _, ids := range [][]string{{a.ID, a.ID}, {a.ID, uuid.NewString()}, {"invalid"}, {}} {
		payload["user_ids"] = ids
		require.Equal(t, http.StatusBadRequest, executeRequest("POST", "/v1/notifications", payload, token).Code)
	}
	_, err := testPool.Exec(context.Background(), "UPDATE public.users SET is_active = false WHERE id = $1", b.ID)
	require.NoError(t, err)
	payload["user_ids"] = []string{a.ID, b.ID}
	require.Equal(t, http.StatusBadRequest, executeRequest("POST", "/v1/notifications", payload, token).Code)
	require.Len(t, listNotifications(t, generateToken(a.ID), "").Items, 1)
	payload["user_ids"] = []string{a.ID}
	payload["title"] = "   "
	require.Equal(t, http.StatusBadRequest, executeRequest("POST", "/v1/notifications", payload, token).Code)
	payload["title"] = "Hello"
	payload["content"] = strings.Repeat("a", 2001)
	require.Equal(t, http.StatusBadRequest, executeRequest("POST", "/v1/notifications", payload, token).Code)
	payload["content"] = "Test message"
	// Concurrent sends share the same database limit: one prior success leaves nine slots.
	codes := make(chan int, 12)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); codes <- executeRequest("POST", "/v1/notifications", payload, token).Code }()
	}
	wg.Wait()
	close(codes)
	successes, limited := 0, 0
	for code := range codes {
		if code == http.StatusCreated {
			successes++
		} else {
			require.Equal(t, http.StatusTooManyRequests, code)
			limited++
		}
	}
	require.Equal(t, 9, successes)
	require.Equal(t, 3, limited)
}

func TestLineIDProfile(t *testing.T) {
	clearTables()
	a := createTestUser(t, "linea@test.com", "pass", false)
	b := createTestUser(t, "lineb@test.com", "pass", false)
	admin := createTestUser(t, "lineadmin@test.com", "pass", true)
	token := generateToken(a.ID)
	for _, id := range []string{"abc", strings.Repeat("a", 21), "Abcd", "abcd ef", "中文測試", "abcd@"} {
		require.Equal(t, http.StatusBadRequest, executeRequest("PATCH", "/v1/me", map[string]any{"line_id": id}, token).Code)
	}
	id := "test.line-id_123"
	w := executeRequest("PATCH", "/v1/me", map[string]any{"line_id": id}, token)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var profile userHttp.MeResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &profile))
	require.NotNil(t, profile.User.LineID)
	require.Equal(t, id, *profile.User.LineID)
	require.Equal(t, http.StatusForbidden, executeRequest("PATCH", "/v1/users/"+b.ID, map[string]any{"line_id": id}, token).Code)
	require.Equal(t, http.StatusOK, executeRequest("PATCH", "/v1/users/"+b.ID, map[string]any{"line_id": id}, generateToken(admin.ID)).Code)
	w = executeRequest("PATCH", "/v1/me", map[string]any{"display_name": "Changed"}, token)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &profile))
	require.NotNil(t, profile.User.LineID)
	require.Equal(t, id, *profile.User.LineID)
	w = executeRequest("PATCH", "/v1/me", map[string]any{"line_id": ""}, token)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &profile))
	require.Nil(t, profile.User.LineID)
	registration := map[string]any{"email": "registerline@test.com", "username": "line_test", "password": "password123", "display_name": "LINE", "line_id": id}
	w = executeRequest("POST", "/v1/auth/register", registration, "")
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &profile))
	require.NotNil(t, profile.User.LineID)
	require.Equal(t, id, *profile.User.LineID)
}

func TestRegistrationDeadline(t *testing.T) {
	clearTables()
	host := createTestUser(t, "deadlinehost@test.com", "pass", false)
	grantPickupHost(t, host.ID)
	member := createTestUser(t, "deadlinemember@test.com", "pass", false)
	token := generateToken(host.ID)
	locationID := setupTestLocation(t, token, host.ID)
	sportID, level := getSportSkill(t, "BADMINTON", "A")
	start := time.Now().Add(24 * time.Hour)
	body := pickupHttp.CreateGroupBody{Title: "Deadline", StartTime: start, EndTime: start.Add(time.Hour), RegistrationDeadline: start, Capacity: 8, LocationID: locationID, SportID: sportID, MinSkillLevel: level}
	for _, deadline := range []time.Time{{}, time.Now().Add(-time.Hour), start.Add(time.Second)} {
		body.RegistrationDeadline = deadline
		require.Equal(t, http.StatusBadRequest, executeRequest("POST", "/v1/pickup-groups", body, token).Code)
	}
	body.RegistrationDeadline = start
	w := executeRequest("POST", "/v1/pickup-groups", body, token)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var group pickupHttp.PickupGroupResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &group))
	require.WithinDuration(t, start, group.RegistrationDeadline, time.Microsecond)
	path := "/v1/pickup-groups/" + group.ID
	for _, deadline := range []time.Time{time.Now().Add(-time.Second), start.Add(time.Second)} {
		require.Equal(t, http.StatusBadRequest, executeRequest("PATCH", path, map[string]any{"registration_deadline": deadline}, token).Code)
	}
	require.Equal(t, http.StatusForbidden, executeRequest("PATCH", path, map[string]any{"registration_deadline": start.Add(-time.Hour)}, generateToken(member.ID)).Code)
	require.Equal(t, http.StatusBadRequest, executeRequest("PATCH", path, map[string]any{"start_time": start.Add(-time.Hour)}, token).Code)
	deadline := start.Add(-2 * time.Hour)
	require.Equal(t, http.StatusOK, executeRequest("PATCH", path, map[string]any{"registration_deadline": deadline, "start_time": start.Add(-time.Hour)}, token).Code)
	order := enroll(t, generateToken(member.ID), group.ID, level)
	_, err := testPool.Exec(context.Background(), "UPDATE public.pickup_groups SET registration_deadline = now() - interval '1 second' WHERE id = $1", group.ID)
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, executeRequest("POST", path+"/orders", nil, generateToken(member.ID)).Code)
	require.Equal(t, http.StatusConflict, executeRequest("POST", path+"/party-orders", map[string]any{"organizer_name": "Party", "party_size": 2, "members": []map[string]any{{"gender": "male", "skill_level": level}, {"gender": "female", "skill_level": level}}}, generateToken(member.ID)).Code)
	// Existing participants can still be confirmed after the deadline.
	require.Equal(t, http.StatusOK, executeRequest("PATCH", "/v1/pickup-orders/"+order.ID, map[string]any{"status": "confirmed"}, token).Code)
	require.Equal(t, http.StatusOK, executeRequest("PATCH", "/v1/pickup-orders/"+order.ID, map[string]any{"status": "cancelled"}, token).Code)
	require.Equal(t, http.StatusConflict, executeRequest("PATCH", "/v1/pickup-orders/"+order.ID, map[string]any{"status": "confirmed"}, token).Code)
	// An expired existing deadline does not prevent unrelated edits or cancellation.
	require.Equal(t, http.StatusOK, executeRequest("PATCH", path, map[string]any{"title": "Updated"}, token).Code)
	require.Equal(t, http.StatusOK, executeRequest("PATCH", path, map[string]any{"status": "cancelled"}, token).Code)
}
