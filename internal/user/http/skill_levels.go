package http

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

type SkillLevelURI struct {
	SportID string `uri:"sport_id" binding:"required,uuid"`
}

type SetSkillLevelBody struct {
	SkillLevel int `json:"skill_level" binding:"required,min=1,max=100"`
}

type SportSkillLevelResponse struct {
	SportID    string    `json:"sport_id"`
	SportName  string    `json:"sport_name"`
	SkillLevel int       `json:"skill_level"`
	Label      string    `json:"label"`
	IsActive   bool      `json:"is_active"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func newSportSkillLevelResponse(level *user.SportSkillLevel) SportSkillLevelResponse {
	return SportSkillLevelResponse{
		SportID:    level.SportID,
		SportName:  level.SportName,
		SkillLevel: level.SkillLevel,
		Label:      level.Label,
		IsActive:   level.IsActive,
		UpdatedAt:  level.UpdatedAt,
	}
}

func (h *UserHandler) ListSkillLevels(c *gin.Context) {
	levels, err := h.userService.ListSkillLevels(c.Request.Context(), auth.GetUserID(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	items := make([]SportSkillLevelResponse, len(levels))
	for i, level := range levels {
		items[i] = newSportSkillLevelResponse(level)
	}
	// The collection is bounded by the sport catalog and returned in one page.
	c.JSON(http.StatusOK, response.NewPageResponse(items, 1, max(1, len(items)), len(items)))
}

func (h *UserHandler) GetSkillLevel(c *gin.Context) {
	var uri SkillLevelURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sport UUID"})
		return
	}
	level, err := h.userService.GetSkillLevel(c.Request.Context(), auth.GetUserID(c), uri.SportID)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, newSportSkillLevelResponse(level))
}

func (h *UserHandler) SetSkillLevel(c *gin.Context) {
	var uri SkillLevelURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sport UUID"})
		return
	}
	var body SetSkillLevelBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	level, err := h.userService.SetSkillLevel(c.Request.Context(), auth.GetUserID(c), uri.SportID, body.SkillLevel)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, newSportSkillLevelResponse(level))
}

func (h *UserHandler) DeleteSkillLevel(c *gin.Context) {
	var uri SkillLevelURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sport UUID"})
		return
	}
	if err := h.userService.DeleteSkillLevel(c.Request.Context(), auth.GetUserID(c), uri.SportID); err != nil {
		response.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
