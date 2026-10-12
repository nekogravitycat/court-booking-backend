package tests

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func isPickupHost(t *testing.T, userID string) bool {
	t.Helper()
	var exists bool
	require.NoError(t, testPool.QueryRow(context.Background(),
		"SELECT EXISTS(SELECT 1 FROM public.pickup_hosts WHERE user_id = $1)", userID).Scan(&exists))
	return exists
}

func TestCreatingGroupGrantsPickupHost(t *testing.T) {
	clearTables()
	host := createTestUser(t, "autohost@pickup.com", "pass", false)
	hostToken := generateToken(host.ID)
	require.False(t, isPickupHost(t, host.ID))

	locationID := setupTestLocation(t, hostToken, host.ID)
	sportID, _ := getSportSkill(t, "BADMINTON", "A")
	start := time.Now().Add(24 * time.Hour)
	group := createGroup(t, hostToken, locationID, sportID, 1, 8, 0, start, start.Add(2*time.Hour))
	require.True(t, isPickupHost(t, host.ID))

	// The role survives the group ending / being cancelled.
	_, err := testPool.Exec(context.Background(),
		"UPDATE public.pickup_groups SET status = 'completed' WHERE id = $1", group.ID)
	require.NoError(t, err)
	assert.True(t, isPickupHost(t, host.ID))

	// Creating a second group is idempotent.
	createGroup(t, hostToken, locationID, sportID, 1, 8, 0, start.Add(48*time.Hour), start.Add(50*time.Hour))
	assert.True(t, isPickupHost(t, host.ID))
}

func TestHostMessageToParticipants(t *testing.T) {
	clearTables()
	host := createTestUser(t, "mhost@pickup.com", "pass", false)
	other := createTestUser(t, "mother@pickup.com", "pass", false)
	user1 := createTestUser(t, "muser1@pickup.com", "pass", false)
	user2 := createTestUser(t, "muser2@pickup.com", "pass", false)
	outsider := createTestUser(t, "moutsider@pickup.com", "pass", false)

	hostToken := generateToken(host.ID)
	locationID := setupTestLocation(t, hostToken, host.ID)
	sportID, _ := getSportSkill(t, "BADMINTON", "A")
	start := time.Now().Add(24 * time.Hour)
	group := createGroup(t, hostToken, locationID, sportID, 1, 8, 0, start, start.Add(2*time.Hour))

	order1 := enroll(t, generateToken(user1.ID), group.ID, 1)
	enroll(t, generateToken(user2.ID), group.ID, 1)
	// A cancelled enrollment still counts as a participant.
	setOrderStatus(t, generateToken(user1.ID), order1.ID, "cancelled")

	path := "/v1/pickup-groups/" + group.ID + "/notifications"
	body := map[string]any{"title": "Notice", "content": "Bring your own racket"}

	t.Run("only the host or an admin may send", func(t *testing.T) {
		assert.Equal(t, http.StatusUnauthorized, executeRequest("POST", path, body, "").Code)
		assert.Equal(t, http.StatusForbidden, executeRequest("POST", path, body, generateToken(other.ID)).Code)
		assert.Equal(t, http.StatusForbidden, executeRequest("POST", path, body, generateToken(user1.ID)).Code)
	})

	t.Run("sends to every participant by default", func(t *testing.T) {
		w := executeRequest("POST", path, body, hostToken)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		for _, u := range []string{user1.ID, user2.ID} {
			inbox := listNotifications(t, generateToken(u), "")
			var found bool
			for _, n := range inbox.Items {
				if n.Type == "host_message" {
					found = true
					assert.Equal(t, "Bring your own racket", n.Content)
					require.NotNil(t, n.PickupGroupID)
					assert.Equal(t, group.ID, *n.PickupGroupID)
				}
			}
			assert.True(t, found, "participant %s should receive the message", u)
		}
		for _, u := range []string{outsider.ID, host.ID} {
			for _, n := range listNotifications(t, generateToken(u), "").Items {
				assert.NotEqual(t, "host_message", n.Type)
			}
		}
	})

	t.Run("explicit recipients must be participants", func(t *testing.T) {
		b := map[string]any{"title": "Hi", "content": "Hello", "user_ids": []string{outsider.ID}}
		assert.Equal(t, http.StatusBadRequest, executeRequest("POST", path, b, hostToken).Code)
		b["user_ids"] = []string{uuid.NewString()}
		assert.Equal(t, http.StatusBadRequest, executeRequest("POST", path, b, hostToken).Code)
		b["user_ids"] = []string{user2.ID}
		assert.Equal(t, http.StatusCreated, executeRequest("POST", path, b, hostToken).Code)
	})

	t.Run("validates content", func(t *testing.T) {
		assert.Equal(t, http.StatusBadRequest, executeRequest("POST", path, map[string]any{"title": "  ", "content": "x"}, hostToken).Code)
		assert.Equal(t, http.StatusBadRequest, executeRequest("POST", path, map[string]any{"title": "x"}, hostToken).Code)
	})

	t.Run("unknown group", func(t *testing.T) {
		w := executeRequest("POST", "/v1/pickup-groups/"+uuid.NewString()+"/notifications", body, hostToken)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
