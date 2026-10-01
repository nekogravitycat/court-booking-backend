package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/notification"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
	"github.com/stretchr/testify/require"
)

func TestReviewPaginationTotals(t *testing.T) {
	clearTables()
	ctx := context.Background()
	host := createTestUser(t, "page_host@example.com", "password", true)
	player := createTestUser(t, "page_player@example.com", "password", false)
	token := generateToken(host.ID)
	locationID := setupTestLocation(t, token, host.ID)
	sportID, level := getSportSkill(t, "BADMINTON", "A")
	start := time.Now().Add(48 * time.Hour)
	group := createGroup(t, token, locationID, sportID, level, 5, 0, start, start.Add(time.Hour))
	enroll(t, generateToken(player.ID), group.ID, level)
	var orgID, resourceID string
	require.NoError(t, testPool.QueryRow(ctx, "SELECT organization_id FROM public.locations WHERE id=$1", locationID).Scan(&orgID))
	require.NoError(t, testPool.QueryRow(ctx, "INSERT INTO public.resources (location_id, resource_type, name) VALUES ($1,'badminton','Pagination Court') RETURNING id", locationID).Scan(&resourceID))
	_, err := testPool.Exec(ctx, "INSERT INTO public.bookings (user_id, resource_id, start_time, end_time) VALUES ($1,$2,$3,$4)", host.ID, resourceID, start, start.Add(time.Hour))
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, "INSERT INTO public.announcements (title, content) VALUES ('Pagination', 'Content')")
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, "INSERT INTO public.organization_members (organization_id, user_id) VALUES ($1,$2)", orgID, player.ID)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, "INSERT INTO public.organization_managers (organization_id, user_id) VALUES ($1,$2)", orgID, player.ID)
	require.NoError(t, err)
	require.NoError(t, notification.NewPgxRepository(testPool).Create(ctx, &notification.Notification{UserID: host.ID, Type: notification.TypePickupOrderCreated, Title: "Pagination", Content: "Content"}))
	require.NoError(t, user.NewPgxRepository(testPool).AddPickupHost(ctx, host.ID))
	paths := []string{"/v1/users?sort_by=name", "/v1/pickup-hosts", "/v1/organizations", "/v1/locations?organization_id=" + orgID,
		"/v1/resources?location_id=" + locationID, "/v1/bookings", "/v1/pickup-groups?sport_id=" + sportID,
		"/v1/sports", "/v1/skill-levels?sport_id=" + sportID, "/v1/announcements", "/v1/notifications?unread_only=true",
		"/v1/organizations/" + orgID + "/members", "/v1/organizations/" + orgID + "/managers"}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			separator := "?"
			for _, c := range path {
				if c == '?' {
					separator = "&"
					break
				}
			}
			type page struct {
				Items []json.RawMessage `json:"items"`
				Total int               `json:"total"`
			}
			first := executeRequest("GET", path+separator+"page=1&page_size=100", nil, token)
			require.Equal(t, http.StatusOK, first.Code, first.Body.String())
			var a, b page
			require.NoError(t, json.Unmarshal(first.Body.Bytes(), &a))
			require.Greater(t, a.Total, 0)
			last := executeRequest("GET", path+separator+"page=999&page_size=100", nil, token)
			require.Equal(t, http.StatusOK, last.Code, last.Body.String())
			require.NoError(t, json.Unmarshal(last.Body.Bytes(), &b))
			require.Empty(t, b.Items)
			require.Equal(t, a.Total, b.Total, fmt.Sprintf("path=%s", path))
		})
	}
	// A genuinely empty filter must still report zero.
	w := executeRequest("GET", "/v1/users?email=no_such_person&page=999&page_size=100", nil, token)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var empty struct {
		Total int `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &empty))
	require.Zero(t, empty.Total)
}
