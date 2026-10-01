package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	bookinghttp "github.com/nekogravitycat/court-booking-backend/internal/booking/http"
	"github.com/nekogravitycat/court-booking-backend/internal/pickup"
	pickuphttp "github.com/nekogravitycat/court-booking-backend/internal/pickup/http"
	"github.com/nekogravitycat/court-booking-backend/internal/resource"
	resourcehttp "github.com/nekogravitycat/court-booking-backend/internal/resource/http"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
	"github.com/stretchr/testify/require"
)

// Regression coverage for the first review round.
func TestReviewRegressions(t *testing.T) {
	clearTables()
	host := createTestUser(t, "review_host@example.com", "password", false)
	booker := createTestUser(t, "review_booker@example.com", "password", false)
	other := createTestUser(t, "review_other@example.com", "password", false)
	hostToken, bookerToken, otherToken := generateToken(host.ID), generateToken(booker.ID), generateToken(other.ID)
	locationID := setupTestLocation(t, hostToken, host.ID)
	sportID, level := getSportSkill(t, "BADMINTON", "A")
	base := time.Now().Add(48 * time.Hour)
	repo := pickup.NewPgxRepository(testPool)
	ctx := context.Background()

	t.Run("only host and admin read order details", func(t *testing.T) {
		group := createGroup(t, hostToken, locationID, sportID, level, 10, 0, base, base.Add(time.Hour))
		enroll(t, bookerToken, group.ID, level)
		w := executeRequest("GET", "/v1/pickup-groups/"+group.ID+"?include_orders=true", nil, otherToken)
		require.Equal(t, http.StatusForbidden, w.Code)
		for _, token := range []string{hostToken, generateToken(createTestUser(t, "review_order_admin@example.com", "password", true).ID)} {
			w = executeRequest("GET", "/v1/pickup-groups/"+group.ID+"?include_orders=true", nil, token)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), booker.ID)
		}
		wList := executeRequest("GET", "/v1/pickup-groups/"+group.ID+"/orders", nil, otherToken)
		require.Equal(t, http.StatusForbidden, wList.Code)
	})

	t.Run("disabled group rejects enrollment", func(t *testing.T) {
		start := base.Add(3 * time.Hour)
		group := createGroup(t, hostToken, locationID, sportID, level, 10, 0, start, start.Add(time.Hour))
		disabled := false
		w := executeRequest("PATCH", "/v1/pickup-groups/"+group.ID, pickuphttp.UpdateGroupBody{Enable: &disabled}, hostToken)
		require.Equal(t, http.StatusOK, w.Code)
		w = executeRequest("POST", "/v1/pickup-groups/"+group.ID+"/orders", nil, bookerToken)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	})

	t.Run("ended group rejects enrollment", func(t *testing.T) {
		start := time.Now().Add(-48 * time.Hour)
		group := createGroup(t, hostToken, locationID, sportID, level, 10, 0, start, start.Add(time.Hour))
		w := executeRequest("POST", "/v1/pickup-groups/"+group.ID+"/orders", nil, bookerToken)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	})

	t.Run("cancelled group releases time but reactivation revalidates it", func(t *testing.T) {
		start := base.Add(6 * time.Hour)
		groupA := createGroup(t, hostToken, locationID, sportID, level, 10, 0, start, start.Add(time.Hour))
		groupB := createGroup(t, hostToken, locationID, sportID, level, 10, 0, start, start.Add(time.Hour))
		enroll(t, bookerToken, groupA.ID, level)
		cancelled := "cancelled"
		w := executeRequest("PATCH", "/v1/pickup-groups/"+groupA.ID, pickuphttp.UpdateGroupBody{Status: &cancelled}, hostToken)
		require.Equal(t, http.StatusOK, w.Code)
		w = executeRequest("POST", "/v1/pickup-groups/"+groupB.ID+"/orders", nil, bookerToken)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		active := "active"
		w = executeRequest("PATCH", "/v1/pickup-groups/"+groupA.ID, pickuphttp.UpdateGroupBody{Status: &active}, hostToken)
		require.Equal(t, http.StatusConflict, w.Code)
		current, err := repo.GetGroupByID(ctx, groupA.ID)
		require.NoError(t, err)
		require.Equal(t, "cancelled", string(current.Status))
	})

	t.Run("reactivation rejects cross-group time conflict", func(t *testing.T) {
		start := base.Add(9 * time.Hour)
		groupA := createGroup(t, hostToken, locationID, sportID, level, 10, 0, start, start.Add(time.Hour))
		groupB := createGroup(t, hostToken, locationID, sportID, level, 10, 0, start, start.Add(time.Hour))
		orderA := enroll(t, bookerToken, groupA.ID, level)
		setOrderStatus(t, bookerToken, orderA.ID, "cancelled")
		enroll(t, bookerToken, groupB.ID, level)
		confirmed := "confirmed"
		w := executeRequest("PATCH", "/v1/pickup-orders/"+orderA.ID, pickuphttp.UpdateOrderBody{Status: &confirmed}, hostToken)
		require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	})

	t.Run("capacity edit rejects occupied seats", func(t *testing.T) {
		start := base.Add(12 * time.Hour)
		group := createGroup(t, hostToken, locationID, sportID, level, 2, 0, start, start.Add(time.Hour))
		enroll(t, bookerToken, group.ID, level)
		stale, err := repo.GetGroupByID(ctx, group.ID)
		require.NoError(t, err)
		require.Equal(t, 1, stale.CurrentEnrolled)
		stale.Capacity = 1
		enroll(t, otherToken, group.ID, level)
		w := executeRequest("PATCH", "/v1/pickup-groups/"+group.ID, pickuphttp.UpdateGroupBody{Capacity: &stale.Capacity}, hostToken)
		require.Equal(t, http.StatusConflict, w.Code)
		current, err := repo.GetGroupByID(ctx, group.ID)
		require.NoError(t, err)
		require.LessOrEqual(t, current.CurrentEnrolled, current.Capacity)
		t.Logf("capacity=%d enrolled=%d", current.Capacity, current.CurrentEnrolled)
	})

	t.Run("payment patch never restores a rejected order", func(t *testing.T) {
		start := base.Add(15 * time.Hour)
		group := createGroup(t, hostToken, locationID, sportID, level, 1, 0, start, start.Add(time.Hour))
		order := enroll(t, bookerToken, group.ID, level)
		setOrderStatus(t, hostToken, order.ID, "rejected")
		enroll(t, otherToken, group.ID, level)
		done := "done"
		w := executeRequest("PATCH", "/v1/pickup-orders/"+order.ID, pickuphttp.UpdateOrderBody{PaymentStatus: &done}, hostToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		current, err := repo.GetOrderByID(ctx, order.ID)
		require.NoError(t, err)
		require.Equal(t, "rejected", string(current.Status))
		groupNow, err := repo.GetGroupByID(ctx, group.ID)
		require.NoError(t, err)
		require.Equal(t, 1, groupNow.CurrentEnrolled)
	})

	t.Run("sort by name maps to display name", func(t *testing.T) {
		admin := createTestUser(t, "review_admin@example.com", "password", true)
		w := executeRequest("GET", "/v1/users?sort_by=name", nil, generateToken(admin.ID))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	})

	t.Run("profile update preserves account flags", func(t *testing.T) {
		userRepo := user.NewPgxRepository(testPool)
		stale, err := userRepo.GetByID(ctx, other.ID)
		require.NoError(t, err)
		require.NoError(t, userRepo.Delete(ctx, other.ID))
		name := "Changed profile"
		stale.DisplayName = &name
		require.NoError(t, userRepo.Update(ctx, stale.ID, user.UpdateUserRequest{DisplayName: &name}))
		current, err := userRepo.GetByID(ctx, other.ID)
		require.NoError(t, err)
		require.False(t, current.IsActive)
		_, err = testPool.Exec(ctx, "UPDATE public.users SET is_active=true WHERE id=$1", other.ID)
		require.NoError(t, err)
	})

	t.Run("failed resource deletion preserves cover and returns conflict", func(t *testing.T) {
		w := executeRequest("POST", "/v1/resources", resourcehttp.CreateRequest{Name: "Review Court", LocationID: locationID, ResourceType: "badminton"}, hostToken)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		var resource resourcehttp.ResourceResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resource))
		var fileID string
		require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO public.files (user_id, filename, storage_path, content_type, size) VALUES ($1, 'review.jpg', 'review-nonexistent.jpg', 'image/jpeg', 1) RETURNING id`, host.ID).Scan(&fileID))
		_, err := testPool.Exec(ctx, "UPDATE public.resources SET cover=$1 WHERE id=$2", fileID, resource.ID)
		require.NoError(t, err)
		start := time.Now().UTC().Truncate(24 * time.Hour).Add(32 * time.Hour)
		_, err = testPool.Exec(ctx, `INSERT INTO public.bookings (user_id, resource_id, start_time, end_time) VALUES ($1,$2,$3,$4)`, booker.ID, resource.ID, start, start.Add(time.Hour))
		require.NoError(t, err)
		w = executeRequest("DELETE", "/v1/resources/"+resource.ID, nil, hostToken)
		t.Logf("delete HTTP status: %d", w.Code)
		_, debugErr := resourceRepoGet(ctx, resource.ID)
		t.Logf("resource read: %v", debugErr)
		t.Logf("resource delete error: %v", resourceRepoDelete(ctx, resource.ID))
		var cover *string
		require.NoError(t, testPool.QueryRow(ctx, "SELECT cover FROM public.resources WHERE id=$1", resource.ID).Scan(&cover))
		require.NotNil(t, cover)
		require.Equal(t, fileID, *cover)
		require.Equal(t, http.StatusConflict, w.Code)
		var exists bool
		require.NoError(t, testPool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM public.files WHERE id=$1)", fileID).Scan(&exists))
		require.True(t, exists)

		opening := true
		w = executeRequest("PATCH", "/v1/locations/"+locationID, map[string]any{"opening": opening}, hostToken)
		require.Equal(t, http.StatusOK, w.Code)
		w = executeRequest("POST", "/v1/bookings", bookinghttp.CreateBookingRequest{ResourceID: resource.ID, StartTime: start.Add(2 * time.Hour), EndTime: start.Add(3 * time.Hour)}, hostToken)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		var booking bookinghttp.BookingResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &booking))
		done := "done"
		w = executeRequest("PATCH", "/v1/bookings/"+booking.ID, bookinghttp.UpdateBookingRequest{PaymentStatus: &done}, hostToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	})

	t.Run("inactive organization blocks mutations and preserves reads", func(t *testing.T) {
		var orgID string
		require.NoError(t, testPool.QueryRow(ctx, "SELECT organization_id FROM public.locations WHERE id=$1", locationID).Scan(&orgID))
		_, err := testPool.Exec(ctx, "UPDATE public.organizations SET is_active=false WHERE id=$1", orgID)
		require.NoError(t, err)
		w := executeRequest("PATCH", "/v1/locations/"+locationID, map[string]any{"name": "Modified inactive organization"}, hostToken)
		require.Equal(t, http.StatusForbidden, w.Code)
		require.Equal(t, http.StatusOK, executeRequest("GET", "/v1/locations/"+locationID, nil, hostToken).Code)
		var resourceID string
		require.NoError(t, testPool.QueryRow(ctx, "SELECT id FROM public.resources WHERE location_id=$1 LIMIT 1", locationID).Scan(&resourceID))
		start := time.Now().UTC().Truncate(24 * time.Hour).Add(56 * time.Hour)
		w = executeRequest("POST", "/v1/bookings", bookinghttp.CreateBookingRequest{ResourceID: resourceID, StartTime: start, EndTime: start.Add(time.Hour)}, bookerToken)
		require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
		admin := createTestUser(t, "inactive_org_admin@example.com", "password", true)
		w = executeRequest("PATCH", "/v1/locations/"+locationID, map[string]any{"name": "Administrator edit"}, generateToken(admin.ID))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	})
}

func resourceRepoGet(ctx context.Context, id string) (*resource.Resource, error) {
	return resource.NewPgxRepository(testPool).GetByID(ctx, id)
}

func resourceRepoDelete(ctx context.Context, id string) error {
	return resource.NewPgxRepository(testPool).Delete(ctx, id)
}

func TestReviewProfilePreservesPrivileges(t *testing.T) {
	clearTables()
	ctx := context.Background()
	account := createTestUser(t, "flag_account@example.com", "password", true)
	repo := user.NewPgxRepository(testPool)
	old, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.True(t, old.IsActive)
	require.True(t, old.IsSystemAdmin)
	_, err = testPool.Exec(ctx, "UPDATE public.users SET is_active=false, is_system_admin=false WHERE id=$1", account.ID)
	require.NoError(t, err)
	name := "Profile changed after revocation"
	require.NoError(t, repo.Update(ctx, old.ID, user.UpdateUserRequest{DisplayName: &name}))
	var fileID string
	require.NoError(t, testPool.QueryRow(ctx, "INSERT INTO public.files (user_id, filename, storage_path, content_type, size) VALUES ($1,'avatar.jpg','test-avatar.jpg','image/jpeg',1) RETURNING id", account.ID).Scan(&fileID))
	require.NoError(t, repo.UpdateAvatar(ctx, old.ID, &fileID))
	current, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.False(t, current.IsActive)
	require.False(t, current.IsSystemAdmin)
	require.Equal(t, &name, current.DisplayName)
	require.Equal(t, &fileID, current.Avatar)
}
