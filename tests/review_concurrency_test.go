package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/location"
	"github.com/nekogravitycat/court-booking-backend/internal/organization"
	"github.com/nekogravitycat/court-booking-backend/internal/pickup"
	pickuphttp "github.com/nekogravitycat/court-booking-backend/internal/pickup/http"
	"github.com/stretchr/testify/require"
)

// runTogether holds both callers at a barrier before starting the operations.
func runTogether(first, second func() error) [2]error {
	ready := make(chan struct{})
	var wg sync.WaitGroup
	var result [2]error
	for i, fn := range []func() error{first, second} {
		wg.Add(1)
		go func(i int, fn func() error) { defer wg.Done(); <-ready; result[i] = fn() }(i, fn)
	}
	close(ready)
	wg.Wait()
	return result
}

func TestReviewConcurrentEnrollment(t *testing.T) {
	clearTables()
	ctx := context.Background()
	host := createTestUser(t, "concurrent_host@example.com", "password", false)
	player := createTestUser(t, "concurrent_player@example.com", "password", false)
	other := createTestUser(t, "concurrent_other@example.com", "password", false)
	hostToken := generateToken(host.ID)
	locationID := setupTestLocation(t, hostToken, host.ID)
	sportID, level := getSportSkill(t, "BADMINTON", "A")
	repo := pickup.NewPgxRepository(testPool)
	start := time.Now().Add(72 * time.Hour)

	t.Run("same user cannot join overlapping groups concurrently", func(t *testing.T) {
		a := createGroup(t, hostToken, locationID, sportID, level, 5, 0, start, start.Add(time.Hour))
		b := createGroup(t, hostToken, locationID, sportID, level, 5, 0, start, start.Add(time.Hour))
		result := runTogether(func() error {
			return repo.CreateOrder(ctx, &pickup.PickupOrder{PickupGroupID: a.ID, UserID: player.ID, BookerName: "Player", PartySize: 1, SkillLevel: level, Status: pickup.OrderStatusPending, PaymentStatus: pickup.PaymentStatusPending})
		},
			func() error {
				return repo.CreateOrder(ctx, &pickup.PickupOrder{PickupGroupID: b.ID, UserID: player.ID, BookerName: "Player", PartySize: 1, SkillLevel: level, Status: pickup.OrderStatusPending, PaymentStatus: pickup.PaymentStatusPending})
			})
		successes := 0
		for _, err := range result {
			if err == nil {
				successes++
			} else {
				require.ErrorIs(t, err, pickup.ErrTimeConflict)
			}
		}
		require.Equal(t, 1, successes)
	})

	t.Run("capacity shrink and enrollment remain consistent", func(t *testing.T) {
		begin := start.Add(3 * time.Hour)
		group := createGroup(t, hostToken, locationID, sportID, level, 2, 0, begin, begin.Add(time.Hour))
		enroll(t, generateToken(player.ID), group.ID, level)
		capacity := 1
		var editStatus int
		result := runTogether(func() error {
			w := executeRequest("PATCH", "/v1/pickup-groups/"+group.ID, pickuphttp.UpdateGroupBody{Capacity: &capacity}, hostToken)
			editStatus = w.Code
			return nil
		}, func() error {
			return repo.CreateOrder(ctx, &pickup.PickupOrder{PickupGroupID: group.ID, UserID: other.ID, BookerName: "Other", PartySize: 1, SkillLevel: level, Status: pickup.OrderStatusPending, PaymentStatus: pickup.PaymentStatusPending})
		})
		require.NoError(t, result[0])
		if editStatus == http.StatusOK {
			require.ErrorIs(t, result[1], pickup.ErrGroupFullyBooked)
		} else {
			require.Equal(t, http.StatusConflict, editStatus)
			require.NoError(t, result[1])
		}
		current, err := repo.GetGroupByID(ctx, group.ID)
		require.NoError(t, err)
		require.LessOrEqual(t, current.CurrentEnrolled, current.Capacity)
	})

	t.Run("payment and rejection cannot overwrite each other", func(t *testing.T) {
		begin := start.Add(6 * time.Hour)
		group := createGroup(t, hostToken, locationID, sportID, level, 1, 0, begin, begin.Add(time.Hour))
		order := enroll(t, generateToken(player.ID), group.ID, level)
		rejected, done := "rejected", "done"
		var status [2]int
		runTogether(func() error {
			status[0] = executeRequest("PATCH", "/v1/pickup-orders/"+order.ID, pickuphttp.UpdateOrderBody{Status: &rejected}, hostToken).Code
			return nil
		},
			func() error {
				status[1] = executeRequest("PATCH", "/v1/pickup-orders/"+order.ID, pickuphttp.UpdateOrderBody{PaymentStatus: &done}, hostToken).Code
				return nil
			})
		require.Equal(t, [2]int{http.StatusOK, http.StatusOK}, status)
		current, err := repo.GetOrderByID(ctx, order.ID)
		require.NoError(t, err)
		require.Equal(t, pickup.OrderStatusRejected, current.Status)
		require.Equal(t, pickup.PaymentStatusDone, current.PaymentStatus)
		enroll(t, generateToken(other.ID), group.ID, level)
	})

	t.Run("sport and time changes preserve existing participants", func(t *testing.T) {
		begin := start.Add(9 * time.Hour)
		a := createGroup(t, hostToken, locationID, sportID, level, 5, 0, begin, begin.Add(time.Hour))
		b := createGroup(t, hostToken, locationID, sportID, level, 5, 0, begin.Add(2*time.Hour), begin.Add(3*time.Hour))
		enroll(t, generateToken(player.ID), a.ID, level)
		enroll(t, generateToken(player.ID), b.ID, level)
		tennis, tennisLevel := getSportSkill(t, "BASKETBALL", "A")
		w := executeRequest("PATCH", "/v1/pickup-groups/"+a.ID, pickuphttp.UpdateGroupBody{SportID: &tennis, MinSkillLevel: &tennisLevel, MaxSkillLevel: &tennisLevel}, hostToken)
		require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
		beginB, endB := begin.Add(2*time.Hour), begin.Add(3*time.Hour)
		w = executeRequest("PATCH", "/v1/pickup-groups/"+a.ID, pickuphttp.UpdateGroupBody{StartTime: &beginB, EndTime: &endB}, hostToken)
		require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
		var current pickuphttp.PickupGroupResponse
		w = executeRequest("GET", "/v1/pickup-groups/"+a.ID, nil, hostToken)
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &current))
		require.Equal(t, sportID, current.Sport.ID)
		require.WithinDuration(t, begin, current.StartTime, time.Microsecond)
	})
}

func TestReviewRoleTransactions(t *testing.T) {
	clearTables()
	ctx := context.Background()
	host := createTestUser(t, "roles_host@example.com", "password", false)
	candidate := createTestUser(t, "roles_candidate@example.com", "password", false)
	hostToken := generateToken(host.ID)
	locationID := setupTestLocation(t, hostToken, host.ID)
	var orgID string
	require.NoError(t, testPool.QueryRow(ctx, "SELECT organization_id FROM public.locations WHERE id=$1", locationID).Scan(&orgID))
	orgRepo := organization.NewPgxRepository(testPool)
	locRepo := location.NewPgxRepository(testPool)
	require.NoError(t, orgRepo.AddMember(ctx, orgID, candidate.ID))
	results := runTogether(func() error { return orgRepo.AddOrganizationManager(ctx, orgID, candidate.ID) }, func() error { return locRepo.AddLocationManager(ctx, locationID, candidate.ID) })
	successes := 0
	for _, err := range results {
		if err == nil {
			successes++
		} else {
			require.Error(t, err)
		}
	}
	require.Equal(t, 1, successes)
	var locationManager bool
	require.NoError(t, testPool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM public.location_managers WHERE organization_id=$1 AND user_id=$2)", orgID, candidate.ID).Scan(&locationManager))
	if locationManager {
		require.NoError(t, locRepo.RemoveLocationManager(ctx, locationID, candidate.ID))
	}
	require.NoError(t, orgRepo.UpdateDetails(ctx, orgID, organization.UpdateOrganizationRequest{OwnerID: &candidate.ID}))
	current, err := orgRepo.GetByID(ctx, orgID)
	require.NoError(t, err)
	require.Equal(t, candidate.ID, current.OwnerID)
	member, err := orgRepo.IsMember(ctx, orgID, candidate.ID)
	require.NoError(t, err)
	require.False(t, member)
	manager, err := orgRepo.IsOrganizationManager(ctx, orgID, candidate.ID)
	require.NoError(t, err)
	require.False(t, manager)
	require.ErrorIs(t, locRepo.AddLocationManager(ctx, locationID, candidate.ID), location.ErrOrganizationRoleConflict)
	require.ErrorIs(t, orgRepo.AddMember(ctx, orgID, candidate.ID), organization.ErrOwnerRoleConflict)
}
