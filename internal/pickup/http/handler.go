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
	if !request.BindJSON(c, &body) {
		return
	}

	if err := body.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := auth.GetUserID(c)
	enable := true
	if body.Enable != nil {
		enable = *body.Enable
	}

	req := pickup.CreateGroupRequest{
		HostID:               userID,
		Title:                body.Title,
		Description:          body.Description,
		StartTime:            body.StartTime,
		RegistrationDeadline: body.RegistrationDeadline,
		EndTime:              body.EndTime,
		Fee:                  body.Fee,
		Capacity:             body.Capacity,
		LocationID:           body.LocationID,
		SportID:              body.SportID,
		MinSkillLevel:        body.MinSkillLevel,
		MaxSkillLevel:        body.MaxSkillLevel,
		Enable:               enable,
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
	if !request.BindQuery(c, &req) {
		return
	}
	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sortOrder := strings.ToUpper(req.SortOrder)

	filter := pickup.GroupFilter{
		SportID:             req.SportID,
		MinSkillLevel:       req.MinSkillLevel,
		MaxSkillLevel:       req.MaxSkillLevel,
		FeeMin:              req.FeeMin,
		FeeMax:              req.FeeMax,
		FollowedOnly:        req.FollowedOnly,
		Latitude:            req.Latitude,
		Longitude:           req.Longitude,
		PubliclyVisibleOnly: true,
		ViewerUserID:        auth.GetUserID(c),
		Page:                req.Page,
		PageSize:            req.PageSize,
		SortBy:              req.SortBy,
		SortOrder:           sortOrder,
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
	if !request.BindURI(c, &uri) {
		return
	}

	var req ListGroupsRequest
	if !request.BindQuery(c, &req) {
		return
	}
	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sortOrder := strings.ToUpper(req.SortOrder)

	// Hidden (enable=false) groups are only listed for the host themself and
	// system admins; everyone else, including anonymous callers, never sees them.
	viewerID := auth.GetUserID(c)
	canSeeHidden := viewerID != "" && (viewerID == uri.HostID || auth.IsSystemAdmin(c))

	filter := pickup.GroupFilter{
		Status:        req.Status,
		SportID:       req.SportID,
		MinSkillLevel: req.MinSkillLevel,
		MaxSkillLevel: req.MaxSkillLevel,
		FeeMin:        req.FeeMin,
		FeeMax:        req.FeeMax,
		FollowedOnly:  req.FollowedOnly,
		Latitude:      req.Latitude,
		Longitude:     req.Longitude,
		HostID:        uri.HostID,
		ViewerUserID:  viewerID,
		EnabledOnly:   !canSeeHidden,
		Page:          req.Page,
		PageSize:      req.PageSize,
		SortBy:        req.SortBy,
		SortOrder:     sortOrder,
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
	if !request.BindURI(c, &uri) {
		return
	}

	var query GetGroupQuery
	if !request.BindQuery(c, &query) {
		return
	}

	group, err := h.service.GetGroupByID(c.Request.Context(), uri.ID)
	if err != nil {
		response.Error(c, err)
		return
	}

	var orders []*pickup.PickupOrder
	if query.IncludeOrders {
		orders, err = h.service.GetOrdersByGroupID(c.Request.Context(), uri.ID, auth.GetUserID(c), auth.IsSystemAdmin(c))
		if err != nil {
			response.Error(c, err)
			return
		}
	}

	// The host's phone is private: only the host, admins, and confirmed
	// participants may see it.
	canSeePhone, err := h.service.CanViewHostPhone(c.Request.Context(), group, auth.GetUserID(c), auth.IsSystemAdmin(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	if !canSeePhone {
		masked := *group
		masked.HostPhone = nil
		group = &masked
	}

	c.JSON(http.StatusOK, NewPickupGroupResponse(group, orders))
}

func (h *Handler) UpdateGroup(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}

	userID := auth.GetUserID(c)
	// System admins may update any group; its creator may update only their
	// own groups.
	if !auth.IsSystemAdmin(c) {
		group, err := h.service.GetGroupByID(c.Request.Context(), uri.ID)
		if err != nil {
			response.Error(c, err)
			return
		}
		if group.HostID != userID {
			c.JSON(http.StatusForbidden, gin.H{"error": "only the pickup host or a system admin can update this pickup group"})
			return
		}
	}

	var body UpdateGroupBody
	if !request.BindJSON(c, &body) {
		return
	}

	req := pickup.UpdateGroupRequest{
		Title:                body.Title,
		Description:          body.Description,
		StartTime:            body.StartTime,
		RegistrationDeadline: body.RegistrationDeadline,
		EndTime:              body.EndTime,
		Fee:                  body.Fee,
		Capacity:             body.Capacity,
		LocationID:           body.LocationID,
		SportID:              body.SportID,
		MinSkillLevel:        body.MinSkillLevel,
		MaxSkillLevel:        body.MaxSkillLevel,
		Status:               body.Status,
		Enable:               body.Enable,
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
	if !request.BindURI(c, &uri) {
		return
	}

	if !auth.IsSystemAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "only system admin can delete pickup groups"})
		return
	}

	if err := h.service.DeleteGroup(c.Request.Context(), uri.ID); err != nil {
		response.Error(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *Handler) CreateOrder(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}

	userID := auth.GetUserID(c)
	u, err := h.userService.GetByID(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, err)
		return
	}

	bookerName := u.Username
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
	if !request.BindURI(c, &uri) {
		return
	}

	var body UpdateOrderBody
	if !request.BindJSON(c, &body) {
		return
	}

	userID := auth.GetUserID(c)
	if body.Status == nil && body.PaymentStatus == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status or payment_status is required"})
		return
	}

	isSysAdmin := auth.IsSystemAdmin(c)

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

// DeleteOrder hard-deletes a pickup order. Only a system admin may perform this
// operation; a host removes a participant by rejecting the order instead
// (PATCH with status=rejected).
func (h *Handler) DeleteOrder(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}

	isSysAdmin := auth.IsSystemAdmin(c)

	if err := h.service.DeleteOrder(c.Request.Context(), uri.ID, isSysAdmin); err != nil {
		response.Error(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *Handler) ListGroupOrders(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}

	userID := auth.GetUserID(c)
	orders, err := h.service.GetOrdersByGroupID(c.Request.Context(), uri.ID, userID, auth.IsSystemAdmin(c))
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

// CreatePartyOrder enrolls several people under one order. The caller is the
// organizer; the other seats are anonymous members described only by gender and
// skill level. The whole party must fit within the group's remaining capacity.
func (h *Handler) CreatePartyOrder(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}

	var body CreatePartyOrderBody
	if !request.BindJSON(c, &body) {
		return
	}

	userID := auth.GetUserID(c)
	u, err := h.userService.GetByID(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, err)
		return
	}

	bookerPhone := ""
	if u.Phone != nil {
		bookerPhone = *u.Phone
	}

	members := make([]pickup.OrderMember, len(body.Members))
	for i, m := range body.Members {
		members[i] = pickup.OrderMember{Gender: m.Gender, SkillLevel: m.SkillLevel}
	}

	order, err := h.service.CreatePartyOrder(c.Request.Context(), pickup.CreatePartyOrderRequest{
		PickupGroupID: uri.ID,
		UserID:        userID,
		OrganizerName: body.OrganizerName,
		BookerPhone:   bookerPhone,
		PartySize:     body.PartySize,
		Members:       members,
	})
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusCreated, NewPickupOrderResponse(order))
}

// GetParticipantStats returns the anonymous gender / age / skill-level breakdown
// of a group's enrolled seats. Public: it exposes counts only, never identities.
func (h *Handler) GetParticipantStats(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}

	stats, err := h.service.GetParticipantStats(c.Request.Context(), uri.ID)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, NewParticipantStatsResponse(stats))
}
