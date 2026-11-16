package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/coach"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
)

type Handler struct {
	coaches coach.Service
	groups  coach.GroupService
}

func NewHandler(coaches coach.Service, groups coach.GroupService) *Handler {
	return &Handler{coaches: coaches, groups: groups}
}

func actor(c *gin.Context) coach.Actor {
	return coach.Actor{UserID: auth.GetUserID(c), IsAdmin: auth.IsSystemAdmin(c)}
}

// ---- Coach profiles ----

func (h *Handler) Apply(c *gin.Context) {
	var req ApplyRequest
	if !request.BindJSON(c, &req) {
		return
	}
	coachProfile, err := h.coaches.Apply(c.Request.Context(), auth.GetUserID(c), req.ToInput())
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, NewCoachResponse(coachProfile))
}

func (h *Handler) GetMe(c *gin.Context) {
	coachProfile, err := h.coaches.GetMine(c.Request.Context(), auth.GetUserID(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, NewCoachResponse(coachProfile))
}

func (h *Handler) UpdateMe(c *gin.Context) {
	var req UpdateCoachRequest
	if !request.BindJSON(c, &req) {
		return
	}
	coachProfile, err := h.coaches.UpdateMine(c.Request.Context(), auth.GetUserID(c), req.ToUpdate())
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, NewCoachResponse(coachProfile))
}

func (h *Handler) GetCoach(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	coachProfile, err := h.coaches.Get(c.Request.Context(), uri.ID, auth.GetUserID(c), auth.IsSystemAdmin(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, NewCoachResponse(coachProfile))
}

func (h *Handler) ListCoaches(c *gin.Context) {
	var req ListCoachesRequest
	if !request.BindQuery(c, &req) {
		return
	}
	items, total, err := h.coaches.List(c.Request.Context(), coach.CoachFilter{
		SportID: req.SportID, Keyword: req.Keyword, Page: req.Page, PageSize: req.PageSize,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	writeCoachPage(c, items, req.Page, req.PageSize, total)
}

func (h *Handler) ListApplications(c *gin.Context) {
	var req ListApplicationsRequest
	if !request.BindQuery(c, &req) {
		return
	}
	items, total, err := h.coaches.ListApplications(c.Request.Context(), coach.CoachFilter{
		Status: req.Status, Page: req.Page, PageSize: req.PageSize,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	writeCoachPage(c, items, req.Page, req.PageSize, total)
}

func writeCoachPage(c *gin.Context, items []*coach.Coach, page, pageSize, total int) {
	out := make([]CoachResponse, len(items))
	for i, it := range items {
		out[i] = NewCoachResponse(it)
	}
	c.JSON(http.StatusOK, response.NewPageResponse(out, page, pageSize, total))
}

func (h *Handler) ReviewApplication(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	var req ReviewRequest
	if !request.BindJSON(c, &req) {
		return
	}
	coachProfile, err := h.coaches.Review(c.Request.Context(), uri.ID, auth.GetUserID(c), req.Decision == "approve", req.Reason)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, NewCoachResponse(coachProfile))
}

// ---- Groups ----

func (h *Handler) CreateGroup(c *gin.Context) {
	var req CreateGroupRequest
	if !request.BindJSON(c, &req) {
		return
	}
	g, err := h.groups.Create(c.Request.Context(), auth.GetUserID(c), req.Name, req.AllowPrivateCourses)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, NewGroupResponse(g))
}

func (h *Handler) GetGroup(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	g, err := h.groups.Get(c.Request.Context(), uri.ID)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, NewGroupResponse(g))
}

func (h *Handler) GetMyGroup(c *gin.Context) {
	g, err := h.groups.GetMine(c.Request.Context(), auth.GetUserID(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, NewGroupResponse(g))
}

func (h *Handler) ListGroups(c *gin.Context) {
	var req ListGroupsRequest
	if !request.BindQuery(c, &req) {
		return
	}
	items, total, err := h.groups.List(c.Request.Context(), coach.GroupFilter{Keyword: req.Keyword, Page: req.Page, PageSize: req.PageSize})
	if err != nil {
		response.Error(c, err)
		return
	}
	out := make([]GroupResponse, len(items))
	for i, g := range items {
		out[i] = NewGroupResponse(g)
	}
	c.JSON(http.StatusOK, response.NewPageResponse(out, req.Page, req.PageSize, total))
}

func (h *Handler) UpdateGroup(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	var req UpdateGroupRequest
	if !request.BindJSON(c, &req) {
		return
	}
	g, err := h.groups.Update(c.Request.Context(), uri.ID, actor(c), req.Name, req.AllowPrivateCourses)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, NewGroupResponse(g))
}

func (h *Handler) DisbandGroup(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	if err := h.groups.Disband(c.Request.Context(), uri.ID, actor(c)); err != nil {
		response.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) Invite(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	var req InviteRequest
	if !request.BindJSON(c, &req) {
		return
	}
	inv, err := h.groups.Invite(c.Request.Context(), uri.ID, actor(c), req.UserID, req.Role)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, NewInvitationResponse(inv))
}

func (h *Handler) ListGroupInvitations(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	var req ListInvitationsRequest
	if !request.BindQuery(c, &req) {
		return
	}
	items, total, err := h.groups.ListGroupInvitations(c.Request.Context(), uri.ID, actor(c),
		coach.InvitationFilter{Status: req.Status, Page: req.Page, PageSize: req.PageSize})
	writeInvitationPage(c, items, req, total, err)
}

func (h *Handler) ListMyInvitations(c *gin.Context) {
	var req ListInvitationsRequest
	if !request.BindQuery(c, &req) {
		return
	}
	items, total, err := h.groups.ListMyInvitations(c.Request.Context(), auth.GetUserID(c),
		coach.InvitationFilter{Status: req.Status, Page: req.Page, PageSize: req.PageSize})
	writeInvitationPage(c, items, req, total, err)
}

func writeInvitationPage(c *gin.Context, items []*coach.Invitation, req ListInvitationsRequest, total int, err error) {
	if err != nil {
		response.Error(c, err)
		return
	}
	out := make([]InvitationResponse, len(items))
	for i, it := range items {
		out[i] = NewInvitationResponse(it)
	}
	c.JSON(http.StatusOK, response.NewPageResponse(out, req.Page, req.PageSize, total))
}

func (h *Handler) CancelInvitation(c *gin.Context) {
	var uri GroupInvitationURI
	if !request.BindURI(c, &uri) {
		return
	}
	if err := h.groups.CancelInvitation(c.Request.Context(), uri.GroupID, uri.InvitationID, actor(c)); err != nil {
		response.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) AcceptInvitation(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	g, err := h.groups.Accept(c.Request.Context(), uri.ID, auth.GetUserID(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, NewGroupResponse(g))
}

func (h *Handler) DeclineInvitation(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	if err := h.groups.Decline(c.Request.Context(), uri.ID, auth.GetUserID(c)); err != nil {
		response.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) UpdateMember(c *gin.Context) {
	var uri GroupMemberURI
	if !request.BindURI(c, &uri) {
		return
	}
	var req UpdateMemberRequest
	if !request.BindJSON(c, &req) {
		return
	}
	m, err := h.groups.UpdateMember(c.Request.Context(), uri.GroupID, uri.UserID, actor(c),
		coach.MemberUpdate{Role: req.Role, Status: req.Status})
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, NewMemberResponse(m))
}

func (h *Handler) RemoveMember(c *gin.Context) {
	var uri GroupMemberURI
	if !request.BindURI(c, &uri) {
		return
	}
	if err := h.groups.RemoveMember(c.Request.Context(), uri.GroupID, uri.UserID, actor(c)); err != nil {
		response.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
