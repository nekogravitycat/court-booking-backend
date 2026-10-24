package http

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/file"
	filehttp "github.com/nekogravitycat/court-booking-backend/internal/file/http"
	"github.com/nekogravitycat/court-booking-backend/internal/lineauth"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
	"github.com/nekogravitycat/court-booking-backend/internal/session"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

type UserHandler struct {
	userService    user.Service
	sessionService session.Service
	sessionCookie  auth.SessionCookie
	jwtManager     *auth.JWTManager
	fileService    file.Service
	fileHandler    *filehttp.Handler
	lineClient     lineauth.Client // nil when LINE login is disabled
}

func NewHandler(userService user.Service, sessionService session.Service, sessionCookie auth.SessionCookie, jwtManager *auth.JWTManager, fileService file.Service, fileHandler *filehttp.Handler, lineClient lineauth.Client) *UserHandler {
	return &UserHandler{
		lineClient:     lineClient,
		userService:    userService,
		sessionService: sessionService,
		sessionCookie:  sessionCookie,
		jwtManager:     jwtManager,
		fileService:    fileService,
		fileHandler:    fileHandler,
	}
}

// Register handles the user registration process.
// It validates the payload and creates a new user if the email is unique.
func (h *UserHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}

	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()

	birthDate, err := parseOptionalBirthDate(req.BirthDate)
	if err != nil {
		response.Error(c, err)
		return
	}

	u, err := h.userService.Register(ctx, user.RegisterRequest{
		Email:       req.Email,
		Username:    req.Username,
		Password:    req.Password,
		DisplayName: req.DisplayName,
		Gender:      req.Gender,
		BirthDate:   birthDate,
		LineID:      req.LineID,
	})
	if err != nil {
		response.Error(c, err)
		return
	}

	resp := MeResponse{
		User: NewUserResponse(u),
	}

	c.JSON(http.StatusCreated, resp)
}

// Login authenticates a user using email and password.
// On success, it returns a JWT access token and the user profile.
func (h *UserHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}

	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()

	u, err := h.userService.Login(ctx, req.Email, req.Password)
	if err != nil {
		// For security reasons, map specific errors to InvalidCredentials
		if errors.Is(err, user.ErrNotFound) || errors.Is(err, user.ErrInactiveUser) {
			response.Error(c, user.ErrInvalidCredentials)
			return
		}
		response.Error(c, err)
		return
	}

	// Start a server-side session; its cookie is how the other frontends sharing
	// this API pick up the login.
	sess, rawToken, err := h.sessionService.Create(ctx, u.ID)
	if err != nil {
		response.Error(c, err)
		return
	}
	h.sessionCookie.Set(c, rawToken)

	h.respondWithAccessToken(c, u, sess.ID, nil)
}

// respondWithAccessToken issues an access token bound to the session and writes the login response.
func (h *UserHandler) respondWithAccessToken(c *gin.Context, u *user.User, sessionID string, isNewUser *bool) {
	token, err := h.jwtManager.GenerateAccessToken(u.ID, sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to generate token",
		})
		return
	}

	c.JSON(http.StatusOK, LoginResponse{
		AccessToken: token,
		User:        NewUserResponse(u),
		IsNewUser:   isNewUser,
	})
}

// lineProfile exchanges the LINE authorization result for an identity profile.
// It writes the error response and returns false on failure.
func (h *UserHandler) lineProfile(c *gin.Context) (user.IdentityProfile, bool) {
	if h.lineClient == nil {
		response.Error(c, user.ErrLineLoginDisabled)
		return user.IdentityProfile{}, false
	}

	var req LineAuthRequest
	if !request.BindJSON(c, &req) {
		return user.IdentityProfile{}, false
	}

	ctx := c.Request.Context()
	profile, err := h.lineClient.Exchange(ctx, lineauth.ExchangeInput{
		Code:         req.Code,
		RedirectURI:  req.RedirectURI,
		CodeVerifier: req.CodeVerifier,
		Nonce:        req.Nonce,
	})
	if err != nil {
		response.Error(c, err)
		return user.IdentityProfile{}, false
	}

	p := user.IdentityProfile{
		Provider:    user.ProviderLine,
		Subject:     profile.Subject,
		DisplayName: profile.Name,
		Email:       profile.Email,
	}
	if profile.PictureURL != "" {
		p.FetchAvatar = func(ctx context.Context) ([]byte, error) {
			return h.lineClient.FetchPicture(ctx, profile.PictureURL)
		}
	}
	return p, true
}

// LineLogin signs in with a LINE authorization code, creating the account on
// the first login. The response matches Login, plus is_new_user.
func (h *UserHandler) LineLogin(c *gin.Context) {
	p, ok := h.lineProfile(c)
	if !ok {
		return
	}

	ctx := c.Request.Context()
	u, created, err := h.userService.LoginWithIdentity(ctx, p)
	if err != nil {
		response.Error(c, err)
		return
	}

	sess, rawToken, err := h.sessionService.Create(ctx, u.ID)
	if err != nil {
		response.Error(c, err)
		return
	}
	h.sessionCookie.Set(c, rawToken)

	h.respondWithAccessToken(c, u, sess.ID, &created)
}

// LinkLine attaches the caller's LINE account to their existing account.
func (h *UserHandler) LinkLine(c *gin.Context) {
	userID := auth.GetUserID(c)
	p, ok := h.lineProfile(c)
	if !ok {
		return
	}

	ctx := c.Request.Context()
	if err := h.userService.LinkIdentity(ctx, userID, p); err != nil {
		response.Error(c, err)
		return
	}

	u, err := h.userService.GetByID(ctx, userID)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, MeResponse{User: NewUserResponse(u)})
}

// UnlinkLine detaches LINE from the caller's account.
func (h *UserHandler) UnlinkLine(c *gin.Context) {
	if err := h.userService.UnlinkIdentity(c.Request.Context(), auth.GetUserID(c), user.ProviderLine); err != nil {
		response.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// SetPassword adds a password to a password-less account, such as one created
// through LINE.
func (h *UserHandler) SetPassword(c *gin.Context) {
	var req SetPasswordRequest
	if !request.BindJSON(c, &req) {
		return
	}

	if err := h.userService.SetPassword(c.Request.Context(), auth.GetUserID(c), req.Email, req.Password); err != nil {
		response.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// Refresh exchanges the session cookie for a fresh access token. A frontend
// calls it on startup (and when its access token expires) to pick up a login
// made on any other frontend sharing this API.
func (h *UserHandler) Refresh(c *gin.Context) {
	rawToken := h.sessionCookie.Token(c)
	if rawToken == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not logged in"})
		return
	}

	ctx := c.Request.Context()

	sess, err := h.sessionService.Resolve(ctx, rawToken)
	if err != nil {
		if errors.Is(err, session.ErrInvalidSession) {
			h.sessionCookie.Clear(c)
		}
		response.Error(c, err)
		return
	}

	u, err := h.userService.GetByID(ctx, sess.UserID)
	if err != nil && !errors.Is(err, user.ErrNotFound) {
		response.Error(c, err)
		return
	}
	if err != nil || !u.IsActive {
		h.sessionCookie.Clear(c)
		response.Error(c, session.ErrInvalidSession)
		return
	}

	h.respondWithAccessToken(c, u, sess.ID, nil)
}

// Logout revokes the caller's session, which also invalidates its access
// tokens on every frontend, and clears the session cookie. It is idempotent.
func (h *UserHandler) Logout(c *gin.Context) {
	ctx := c.Request.Context()

	if rawToken := h.sessionCookie.Token(c); rawToken != "" {
		if sess, err := h.sessionService.Resolve(ctx, rawToken); err == nil {
			if err := h.sessionService.Revoke(ctx, sess.ID); err != nil {
				response.Error(c, err)
				return
			}
		}
	}

	// A Bearer client without the cookie can still end its own session.
	if header := c.GetHeader("Authorization"); strings.HasPrefix(strings.ToLower(header), "bearer ") {
		if claims, err := h.jwtManager.ParseAndValidate(header[len("bearer "):]); err == nil {
			if err := h.sessionService.Revoke(ctx, claims.SessionID); err != nil {
				response.Error(c, err)
				return
			}
		}
	}

	h.sessionCookie.Clear(c)
	c.Status(http.StatusNoContent)
}

// Me retrieves the profile of the currently authenticated user.
// It relies on the user ID extracted from the JWT context.
func (h *UserHandler) Me(c *gin.Context) {
	// Assuming auth.GetUserID extracts the ID from context
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Validate UUID format
	if _, err := uuid.Parse(userID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID"})
		return
	}

	ctx := c.Request.Context()

	u, err := h.userService.GetByID(ctx, userID)
	if err != nil {
		response.Error(c, err)
		return
	}

	resp := MeResponse{
		User: NewUserResponse(u),
	}

	c.JSON(http.StatusOK, resp)
}

// List retrieves a paginated list of users with optional filtering.
// Access Control: System Admin only.
func (h *UserHandler) List(c *gin.Context) {
	var req ListUsersRequest
	if !request.BindQuery(c, &req) {
		return
	}

	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	filter := user.UserFilter{
		Page:        req.Page,
		PageSize:    req.PageSize,
		SortBy:      req.SortBy,
		SortOrder:   req.SortOrder,
		Email:       req.Email,
		IDs:         req.IDs,
		DisplayName: req.DisplayName,
		IsActive:    req.IsActive,
	}

	if filter.SortBy == "" {
		filter.SortBy = "created_at"
	}
	if filter.SortOrder == "" {
		filter.SortOrder = "DESC"
	} else {
		filter.SortOrder = strings.ToUpper(filter.SortOrder)
	}

	users, total, err := h.userService.List(c.Request.Context(), filter)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Convert domain users to DTOs
	items := make([]UserResponse, len(users))
	for i, u := range users {
		items[i] = NewUserResponse(u)
	}

	resp := response.NewPageResponse(items, req.Page, req.PageSize, total)

	c.JSON(http.StatusOK, resp)
}

// Get retrieves a specific user by their ID.
// Access Control: System Admin only.
func (h *UserHandler) Get(c *gin.Context) {
	var req request.ByIDRequest
	if !request.BindURI(c, &req) {
		return
	}

	targetUser, err := h.userService.GetByID(c.Request.Context(), req.ID)
	if err != nil {
		response.Error(c, err)
		return
	}

	resp := MeResponse{
		User: NewUserResponse(targetUser),
	}

	c.JSON(http.StatusOK, resp)
}

// Update modifies specific attributes of a user.
// Access Control: System Admin only.
func (h *UserHandler) Update(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}

	var body UpdateUserRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		response.BadRequest(c, "invalid body")
		return
	}

	if err := body.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	birthDate, err := parseOptionalBirthDate(body.BirthDate)
	if err != nil {
		response.Error(c, err)
		return
	}

	req := user.UpdateUserRequest{
		DisplayName:   body.DisplayName,
		Phone:         body.Phone,
		Gender:        body.Gender,
		BirthDate:     birthDate,
		LineID:        body.LineID,
		IsActive:      body.IsActive,
		IsSystemAdmin: body.IsSystemAdmin,
	}

	updatedUser, err := h.userService.Update(c.Request.Context(), uri.ID, req, auth.GetUserID(c))
	if err != nil {
		response.Error(c, err)
		return
	}

	resp := MeResponse{
		User: NewUserResponse(updatedUser),
	}

	c.JSON(http.StatusOK, resp)
}

// isSelfOrSysAdmin verifies if the current user is the target user or a system admin.
func (h *UserHandler) isSelfOrSysAdmin(c *gin.Context, targetUserID string) bool {
	currentUserID := auth.GetUserID(c)
	if currentUserID == targetUserID {
		return true
	}

	return auth.IsSystemAdmin(c)
}

// Delete performs a soft delete on a user.
// Access Control: System Admin only.
func (h *UserHandler) Delete(c *gin.Context) {
	var req request.ByIDRequest
	if !request.BindURI(c, &req) {
		return
	}

	if err := h.userService.Delete(c.Request.Context(), req.ID, auth.GetUserID(c)); err != nil {
		response.Error(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// ListPickupHosts lists all users who hold the pickup host role.
// Access Control: System Admin only.
func (h *UserHandler) ListPickupHosts(c *gin.Context) {
	var req ListUsersRequest
	if !request.BindQuery(c, &req) {
		return
	}

	filter := user.UserFilter{
		Page:        req.Page,
		PageSize:    req.PageSize,
		SortBy:      req.SortBy,
		SortOrder:   req.SortOrder,
		Email:       req.Email,
		IDs:         req.IDs,
		DisplayName: req.DisplayName,
		IsActive:    req.IsActive,
	}

	if filter.SortBy == "" {
		filter.SortBy = "created_at"
	}
	if filter.SortOrder == "" {
		filter.SortOrder = "DESC"
	} else {
		filter.SortOrder = strings.ToUpper(filter.SortOrder)
	}

	users, total, err := h.userService.ListPickupHosts(c.Request.Context(), filter)
	if err != nil {
		response.Error(c, err)
		return
	}

	items := make([]UserResponse, len(users))
	for i, u := range users {
		items[i] = NewUserResponse(u)
	}

	c.JSON(http.StatusOK, response.NewPageResponse(items, req.Page, req.PageSize, total))
}

// AddPickupHost grants the pickup host role to a user.
// Access Control: System Admin only.
func (h *UserHandler) AddPickupHost(c *gin.Context) {
	var body AddPickupHostRequest
	if !request.BindJSON(c, &body) {
		return
	}

	if err := h.userService.AddPickupHost(c.Request.Context(), body.UserID); err != nil {
		response.Error(c, err)
		return
	}

	c.Status(http.StatusCreated)
}

// RemovePickupHost revokes the pickup host role from a user.
// Access Control: System Admin only.
func (h *UserHandler) RemovePickupHost(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}

	if err := h.userService.RemovePickupHost(c.Request.Context(), uri.ID); err != nil {
		response.Error(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// UploadAvatar uploads an avatar image for a user.
func (h *UserHandler) UploadAvatar(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}

	// Permission check: Only the user themselves or system admin can upload their avatar
	if !h.isSelfOrSysAdmin(c, uri.ID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: you can only upload your own avatar"})
		return
	}

	h.fileHandler.HandleFileUpload(c, filehttp.FileUploadConfig{
		MaxSizeBytes: 5 * 1024 * 1024, // 5MB
		AllowedTypes: []string{"image/jpeg", "image/png"},
		ResizeImage:  true,
		AfterUpload: func(ctx context.Context, fileID string) error {
			return h.userService.UpdateAvatar(ctx, uri.ID, fileID)
		},
	})
}

// RemoveAvatar removes the avatar image from a user.
func (h *UserHandler) RemoveAvatar(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}

	// Permission check: Only the user themselves or system admin can remove their avatar
	if !h.isSelfOrSysAdmin(c, uri.ID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: you can only remove your own avatar"})
		return
	}

	if err := h.userService.RemoveAvatar(c.Request.Context(), uri.ID); err != nil {
		response.Error(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// parseOptionalBirthDate converts an optional "YYYY-MM-DD" string into a date.
func parseOptionalBirthDate(s *string) (*time.Time, error) {
	if s == nil {
		return nil, nil
	}
	d, err := user.ParseBirthDate(*s)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// UpdateMe lets the authenticated user edit their own profile (display name,
// phone, gender, birth date). Privileged fields are not accepted here.
func (h *UserHandler) UpdateMe(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var body UpdateMeRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		response.BadRequest(c, "invalid body")
		return
	}

	birthDate, err := parseOptionalBirthDate(body.BirthDate)
	if err != nil {
		response.Error(c, err)
		return
	}

	updated, err := h.userService.Update(c.Request.Context(), userID, user.UpdateUserRequest{
		DisplayName: body.DisplayName,
		Phone:       body.Phone,
		Gender:      body.Gender,
		BirthDate:   birthDate,
		LineID:      body.LineID,
	}, userID)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, MeResponse{User: NewUserResponse(updated)})
}
