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
)

func TestPickupGroupCRUD(t *testing.T) {
	clearTables()

	host := createTestUser(t, "host@pickup.com", "pass", false)
	regularUser := createTestUser(t, "user@pickup.com", "pass", false)

	hostToken := generateToken(host.ID)
	regularUserToken := generateToken(regularUser.ID)
	noToken := ""

	var groupID string

	t.Run("Create Group: Success", func(t *testing.T) {
		payload := pickupHttp.CreateGroupBody{
			Title:      "Sunday Morning Badminton",
			StartTime:  time.Now().Add(24 * time.Hour),
			EndTime:    time.Now().Add(26 * time.Hour),
			Fee:        200,
			Capacity:   8,
			Location:   "Court A",
			SkillLevel: "B",
		}

		w := executeRequest("POST", "/v1/pickup-groups", payload, hostToken)
		require.Equal(t, http.StatusCreated, w.Code)

		var resp pickupHttp.PickupGroupResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.NotEmpty(t, resp.ID)
		assert.Equal(t, payload.Title, resp.Title)
		assert.Equal(t, host.ID, resp.HostID)
		assert.Equal(t, payload.Capacity, resp.Capacity)
		assert.Equal(t, "active", resp.Status)
		assert.Equal(t, 0, resp.CurrentEnrolled)

		groupID = resp.ID
	})

	t.Run("Create Group: Unauthorized (No Token)", func(t *testing.T) {
		payload := pickupHttp.CreateGroupBody{
			Title:      "Secret Group",
			StartTime:  time.Now().Add(24 * time.Hour),
			EndTime:    time.Now().Add(26 * time.Hour),
			Fee:        100,
			Capacity:   4,
			Location:   "Court B",
			SkillLevel: "C",
		}
		w := executeRequest("POST", "/v1/pickup-groups", payload, noToken)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Create Group: Validation Failure", func(t *testing.T) {
		// EndTime before StartTime
		payload := pickupHttp.CreateGroupBody{
			Title:      "Bad Time",
			StartTime:  time.Now().Add(26 * time.Hour),
			EndTime:    time.Now().Add(24 * time.Hour),
			Fee:        100,
			Capacity:   4,
			Location:   "Court B",
			SkillLevel: "A",
		}
		w := executeRequest("POST", "/v1/pickup-groups", payload, hostToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("List Groups: Success & Filtering", func(t *testing.T) {
		wAll := executeRequest("GET", "/v1/pickup-groups", nil, regularUserToken)
		assert.Equal(t, http.StatusOK, wAll.Code)

		var listResp response.PageResponse[pickupHttp.PickupGroupResponse]
		json.Unmarshal(wAll.Body.Bytes(), &listResp)
		assert.GreaterOrEqual(t, listResp.Total, 1)

		// Filter by skill level
		wFilter := executeRequest("GET", "/v1/pickup-groups?skill_level=B", nil, regularUserToken)
		assert.Equal(t, http.StatusOK, wFilter.Code)

		var filterResp response.PageResponse[pickupHttp.PickupGroupResponse]
		json.Unmarshal(wFilter.Body.Bytes(), &filterResp)
		assert.Equal(t, 1, filterResp.Total)
		assert.Equal(t, "B", filterResp.Items[0].SkillLevel)
	})

	t.Run("Get Group: Success", func(t *testing.T) {
		path := fmt.Sprintf("/v1/pickup-groups/%s", groupID)
		w := executeRequest("GET", path, nil, regularUserToken)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp pickupHttp.PickupGroupResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		assert.Equal(t, groupID, resp.ID)
	})
}

func TestPickupOrderAndCapacity(t *testing.T) {
	clearTables()

	host := createTestUser(t, "host2@pickup.com", "pass", false)
	user1 := createTestUser(t, "u1@pickup.com", "pass", false)
	user2 := createTestUser(t, "u2@pickup.com", "pass", false)
	user3 := createTestUser(t, "u3@pickup.com", "pass", false) // for overbooking test

	hostToken := generateToken(host.ID)
	user1Token := generateToken(user1.ID)
	user2Token := generateToken(user2.ID)
	user3Token := generateToken(user3.ID)

	// Create a group with capacity 2
	payload := pickupHttp.CreateGroupBody{
		Title:      "Small Group",
		StartTime:  time.Now().Add(24 * time.Hour),
		EndTime:    time.Now().Add(26 * time.Hour),
		Fee:        100,
		Capacity:   2,
		Location:   "Court C",
		SkillLevel: "C",
	}
	w := executeRequest("POST", "/v1/pickup-groups", payload, hostToken)
	require.Equal(t, http.StatusCreated, w.Code)

	var groupResp pickupHttp.PickupGroupResponse
	json.Unmarshal(w.Body.Bytes(), &groupResp)
	groupID := groupResp.ID

	var order1ID string

	t.Run("Create Order: Success", func(t *testing.T) {
		path := fmt.Sprintf("/v1/pickup-groups/%s/orders", groupID)
		w := executeRequest("POST", path, nil, user1Token)
		require.Equal(t, http.StatusCreated, w.Code)

		var resp pickupHttp.PickupOrderResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		assert.NotEmpty(t, resp.ID)
		assert.Equal(t, user1.ID, resp.UserID)
		assert.Equal(t, groupID, resp.PickupGroupID)
		assert.Equal(t, "pending", resp.PaymentStatus)

		order1ID = resp.ID
	})

	t.Run("Create Order: Duplicate Enrollment", func(t *testing.T) {
		path := fmt.Sprintf("/v1/pickup-groups/%s/orders", groupID)
		w := executeRequest("POST", path, nil, user1Token) // user1 again
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("Create Order: Success 2nd User", func(t *testing.T) {
		path := fmt.Sprintf("/v1/pickup-groups/%s/orders", groupID)
		w := executeRequest("POST", path, nil, user2Token)
		require.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("Create Order: Overbooking", func(t *testing.T) {
		// Group capacity is 2, and 2 users have enrolled
		path := fmt.Sprintf("/v1/pickup-groups/%s/orders", groupID)
		w := executeRequest("POST", path, nil, user3Token) // user3
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("Update Order: Success by Host", func(t *testing.T) {
		path := fmt.Sprintf("/v1/pickup-orders/%s", order1ID)
		payload := pickupHttp.UpdateOrderBody{
			PaymentStatus: "paid",
		}
		w := executeRequest("PATCH", path, payload, hostToken)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp pickupHttp.PickupOrderResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		assert.Equal(t, "paid", resp.PaymentStatus)
	})

	t.Run("Update Order: Permission Denied", func(t *testing.T) {
		path := fmt.Sprintf("/v1/pickup-orders/%s", order1ID)
		payload := pickupHttp.UpdateOrderBody{
			PaymentStatus: "cancelled",
		}
		w := executeRequest("PATCH", path, payload, user3Token) // User3 is not host and not booker
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("Update Order: Success by Booker & Capacity Freed", func(t *testing.T) {
		path := fmt.Sprintf("/v1/pickup-orders/%s", order1ID)
		payload := pickupHttp.UpdateOrderBody{
			PaymentStatus: "cancelled",
		}
		w := executeRequest("PATCH", path, payload, user1Token) // User1 cancels their own order
		assert.Equal(t, http.StatusOK, w.Code)

		// Check capacity is freed, user3 should be able to join now
		pathJoin := fmt.Sprintf("/v1/pickup-groups/%s/orders", groupID)
		wJoin := executeRequest("POST", pathJoin, nil, user3Token)
		assert.Equal(t, http.StatusCreated, wJoin.Code)
	})

	t.Run("Get Group with Orders", func(t *testing.T) {
		path := fmt.Sprintf("/v1/pickup-groups/%s?include_orders=true", groupID)
		w := executeRequest("GET", path, nil, hostToken)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp pickupHttp.PickupGroupResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		assert.Equal(t, 2, resp.CurrentEnrolled) // user2 and user3
		require.NotNil(t, resp.Orders)
		assert.GreaterOrEqual(t, len(*resp.Orders), 2)
	})
}

func TestPickupOrdersList(t *testing.T) {
	clearTables()

	host := createTestUser(t, "host_list@pickup.com", "pass", false)
	user1 := createTestUser(t, "user1_list@pickup.com", "pass", false)
	user2 := createTestUser(t, "user2_list@pickup.com", "pass", false)

	hostToken := generateToken(host.ID)
	user1Token := generateToken(user1.ID)
	user2Token := generateToken(user2.ID)

	// Create a group
	payload := pickupHttp.CreateGroupBody{
		Title:      "Listing Group",
		StartTime:  time.Now().Add(24 * time.Hour),
		EndTime:    time.Now().Add(26 * time.Hour),
		Fee:        150,
		Capacity:   4,
		Location:   "Court D",
		SkillLevel: "A",
	}
	w := executeRequest("POST", "/v1/pickup-groups", payload, hostToken)
	require.Equal(t, http.StatusCreated, w.Code)
	var groupResp pickupHttp.PickupGroupResponse
	json.Unmarshal(w.Body.Bytes(), &groupResp)
	groupID := groupResp.ID

	// Create orders
	w = executeRequest("POST", fmt.Sprintf("/v1/pickup-groups/%s/orders", groupID), nil, user1Token)
	require.Equal(t, http.StatusCreated, w.Code)

	w = executeRequest("POST", fmt.Sprintf("/v1/pickup-groups/%s/orders", groupID), nil, user2Token)
	require.Equal(t, http.StatusCreated, w.Code)

	t.Run("List Group Orders: Success by Host", func(t *testing.T) {
		path := fmt.Sprintf("/v1/pickup-groups/%s/orders", groupID)
		w := executeRequest("GET", path, nil, hostToken)
		assert.Equal(t, http.StatusOK, w.Code)

		var orders []pickupHttp.PickupOrderResponse
		err := json.Unmarshal(w.Body.Bytes(), &orders)
		require.NoError(t, err)
		assert.Len(t, orders, 2)
	})

	t.Run("List Group Orders: Forbidden by Non-Host", func(t *testing.T) {
		path := fmt.Sprintf("/v1/pickup-groups/%s/orders", groupID)
		w := executeRequest("GET", path, nil, user1Token)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("List My Orders: Success", func(t *testing.T) {
		path := "/v1/pickup-orders"
		w := executeRequest("GET", path, nil, user1Token)
		assert.Equal(t, http.StatusOK, w.Code)

		var orders []pickupHttp.PickupOrderResponse
		err := json.Unmarshal(w.Body.Bytes(), &orders)
		require.NoError(t, err)
		assert.Len(t, orders, 1)
		assert.Equal(t, user1.ID, orders[0].UserID)
		assert.Equal(t, groupID, orders[0].PickupGroupID)
	})

	t.Run("List My Orders: Empty", func(t *testing.T) {
		// Create another user with no orders
		user3 := createTestUser(t, "user3_list@pickup.com", "pass", false)
		user3Token := generateToken(user3.ID)

		path := "/v1/pickup-orders"
		w := executeRequest("GET", path, nil, user3Token)
		assert.Equal(t, http.StatusOK, w.Code)

		var orders []pickupHttp.PickupOrderResponse
		err := json.Unmarshal(w.Body.Bytes(), &orders)
		require.NoError(t, err)
		assert.Len(t, orders, 0)
	})
}
