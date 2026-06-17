package http

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/pickup"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

type Handler struct {
	service     pickup.Service
	userService user.Service
}

func NewHandler(service pickup.Service, userService user.Service) *Handler {
	return &Handler{
		service:     service,
		userService: userService,
	}
}

func (h *Handler) CreateGroup(c *gin.Context) {
	var body CreateGroupBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if err := body.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Only pickup hosts (or system admins) may create pickup groups.
	u, err := h.userService.GetByID(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, err)
		return
	}
	if !u.IsSystemAdmin && !u.IsPickupHost {
		c.JSON(http.StatusForbidden, gin.H{"error": "only pickup hosts can create pickup groups"})
		return
	}

	hostName := body.HostName
	hostPhone := body.HostPhone

	if hostName == "" {
		if u.DisplayName != nil {
			hostName = *u.DisplayName
		} else {
			hostName = u.Email
		}
	}
	if hostPhone == "" && u.Phone != nil {
		hostPhone = *u.Phone
	}

	enable := true
	if body.Enable != nil {
		enable = *body.Enable
	}

	req := pickup.CreateGroupRequest{
		HostID:     userID,
		Title:      body.Title,
		HostName:   hostName,
		HostPhone:  hostPhone,
		StartTime:  body.StartTime,
		EndTime:    body.EndTime,
		Fee:        body.Fee,
		Capacity:   body.Capacity,
		LocationID: body.LocationID,
		SkillLevel: body.SkillLevel,
		Enable:     enable,
	}

	group, err := h.service.CreateGroup(c.Request.Context(), req)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusCreated, NewPickupGroupResponse(group, nil))
}

// ListGroups returns the public, bookable-only list of pickup groups.
// No authentication is required and only a trimmed set of fields is exposed.
func (h *Handler) ListGroups(c *gin.Context) {
	var req ListGroupsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid query parameters", "details": err.Error()})
		return
	}

	sortOrder := strings.ToUpper(req.SortOrder)

	filter := pickup.GroupFilter{
		SkillLevel:   req.SkillLevel,
		BookableOnly: true,
		Page:         req.Page,
		PageSize:     req.PageSize,
		SortBy:       req.SortBy,
		SortOrder:    sortOrder,
	}

	groups, total, err := h.service.ListGroups(c.Request.Context(), filter)
	if err != nil {
		response.Error(c, err)
		return
	}

	items := make([]PickupGroupBrief, len(groups))
	for i, g := range groups {
		items[i] = NewPickupGroupBrief(g)
	}

	c.JSON(http.StatusOK, response.NewPageResponse(items, req.Page, req.PageSize, total))
}

// ListGroupsByHost returns the (trimmed) list of pickup groups hosted by a
// specific host. Public, no authentication required. Host phone is never
// included in the trimmed shape.
func (h *Handler) ListGroupsByHost(c *gin.Context) {
	var uri HostGroupsURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	var req ListGroupsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid query parameters", "details": err.Error()})
		return
	}

	sortOrder := strings.ToUpper(req.SortOrder)

	filter := pickup.GroupFilter{
		Status:     req.Status,
		SkillLevel: req.SkillLevel,
		HostID:     uri.HostID,
		Page:       req.Page,
		PageSize:   req.PageSize,
		SortBy:     req.SortBy,
		SortOrder:  sortOrder,
	}

	groups, total, err := h.service.ListGroups(c.Request.Context(), filter)
	if err != nil {
		response.Error(c, err)
		return
	}

	items := make([]PickupGroupBrief, len(groups))
	for i, g := range groups {
		items[i] = NewPickupGroupBrief(g)
	}

	c.JSON(http.StatusOK, response.NewPageResponse(items, req.Page, req.PageSize, total))
}

func (h *Handler) GetGroup(c *gin.Context) {
	var uri request.ByIDRequest
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	var query GetGroupQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid query parameters", "details": err.Error()})
		return
	}

	group, err := h.service.GetGroupByID(c.Request.Context(), uri.ID)
	if err != nil {
		response.Error(c, err)
		return
	}

	var orders []*pickup.PickupOrder
	if query.IncludeOrders {
		orders, err = h.service.GetOrdersByGroupID(c.Request.Context(), uri.ID)
		if err != nil {
			response.Error(c, err)
			return
		}
	}

	c.JSON(http.StatusOK, NewPickupGroupResponse(group, orders))
}

func (h *Handler) UpdateGroup(c *gin.Context) {
	var uri request.ByIDRequest
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	u, err := h.userService.GetByID(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, err)
		return
	}

	// System admins may update any group; a pickup host may update only their
	// own groups.
	if !u.IsSystemAdmin {
		group, err := h.service.GetGroupByID(c.Request.Context(), uri.ID)
		if err != nil {
			response.Error(c, err)
			return
		}
		if !u.IsPickupHost || group.HostID != userID {
			c.JSON(http.StatusForbidden, gin.H{"error": "only the pickup host or a system admin can update this pickup group"})
			return
		}
	}

	var body UpdateGroupBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	req := pickup.UpdateGroupRequest{
		Title:      body.Title,
		HostName:   body.HostName,
		HostPhone:  body.HostPhone,
		StartTime:  body.StartTime,
		EndTime:    body.EndTime,
		Fee:        body.Fee,
		Capacity:   body.Capacity,
		LocationID: body.LocationID,
		SkillLevel: body.SkillLevel,
		Status:     body.Status,
		Enable:     body.Enable,
	}

	group, err := h.service.UpdateGroup(c.Request.Context(), uri.ID, req)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, NewPickupGroupResponse(group, nil))
}

func (h *Handler) DeleteGroup(c *gin.Context) {
	var uri request.ByIDRequest
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	u, err := h.userService.GetByID(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, err)
		return
	}

	if !u.IsSystemAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "only system admin can delete pickup groups"})
		return
	}

	if err := h.service.DeleteGroup(c.Request.Context(), uri.ID); err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

func (h *Handler) CreateOrder(c *gin.Context) {
	var uri request.ByIDRequest
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	u, err := h.userService.GetByID(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, err)
		return
	}

	bookerName := u.Email
	if u.DisplayName != nil {
		bookerName = *u.DisplayName
	}
	bookerPhone := ""
	if u.Phone != nil {
		bookerPhone = *u.Phone
	}

	req := pickup.CreateOrderRequest{
		PickupGroupID: uri.ID,
		UserID:        userID,
		BookerName:    bookerName,
		BookerPhone:   bookerPhone,
	}

	order, err := h.service.CreateOrder(c.Request.Context(), req)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusCreated, NewPickupOrderResponse(order))
}

func (h *Handler) UpdateOrder(c *gin.Context) {
	var uri request.ByIDRequest
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	var body UpdateOrderBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	if body.Status == nil && body.PaymentStatus == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status or payment_status is required"})
		return
	}

	isSysAdmin := false
	if u, err := h.userService.GetByID(c.Request.Context(), userID); err == nil {
		isSysAdmin = u.IsSystemAdmin
	}

	req := pickup.UpdateOrderRequest{
		Status:        body.Status,
		PaymentStatus: body.PaymentStatus,
	}

	order, err := h.service.UpdateOrder(c.Request.Context(), uri.ID, req, userID, isSysAdmin)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, NewPickupOrderResponse(order))
}

func (h *Handler) ListGroupOrders(c *gin.Context) {
	var uri request.ByIDRequest
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	group, err := h.service.GetGroupByID(c.Request.Context(), uri.ID)
	if err != nil {
		response.Error(c, err)
		return
	}

	if group.HostID != userID {
		// System admins may also review enrollments.
		if u, err := h.userService.GetByID(c.Request.Context(), userID); err != nil || !u.IsSystemAdmin {
			c.JSON(http.StatusForbidden, gin.H{"error": "only group host or system admin can view orders"})
			return
		}
	}

	orders, err := h.service.GetOrdersByGroupID(c.Request.Context(), uri.ID)
	if err != nil {
		response.Error(c, err)
		return
	}

	items := make([]PickupOrderResponse, len(orders))
	for i, o := range orders {
		items[i] = NewPickupOrderResponse(o)
	}

	c.JSON(http.StatusOK, items)
}

func (h *Handler) ListMyOrders(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	orders, err := h.service.GetOrdersByUserID(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, err)
		return
	}

	items := make([]PickupOrderResponse, len(orders))
	for i, o := range orders {
		items[i] = NewPickupOrderResponse(o)
	}

	c.JSON(http.StatusOK, items)
}
