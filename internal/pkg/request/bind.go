package request

import (
	"github.com/gin-gonic/gin"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
)

// BindURI binds path parameters into dst. On failure it writes a 400 response and returns false.
func BindURI(c *gin.Context, dst any) bool {
	if err := c.ShouldBindUri(dst); err != nil {
		response.BadRequest(c, "invalid request")
		return false
	}
	return true
}

// BindJSON binds the JSON request body into dst. On failure it writes a 400 response and returns false.
func BindJSON(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		response.BadRequest(c, "invalid request body")
		return false
	}
	return true
}

// BindQuery binds the query string into dst. On failure it writes a 400 response and returns false.
func BindQuery(c *gin.Context, dst any) bool {
	if err := c.ShouldBindQuery(dst); err != nil {
		response.BadRequest(c, "invalid query parameters")
		return false
	}
	return true
}
