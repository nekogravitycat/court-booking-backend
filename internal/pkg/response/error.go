package response

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
)

// ErrorResponse defines the JSON structure for error responses.
type ErrorResponse struct {
	Error string `json:"error"`
}

// Error sends a JSON error response.
// It checks if the error is an AppError to determine the status code.
// If it's not an AppError, it defaults to 500 Internal Server Error.
func Error(c *gin.Context, err error) {
	var appErr *apperror.AppError
	if errors.As(err, &appErr) {
		c.JSON(appErr.Code, ErrorResponse{Error: appErr.Message})
		return
	}

	// Default to 500 for unknown errors. The details stay server-side only.
	log.Printf("internal error: %s %s: %v", c.Request.Method, c.Request.URL.Path, err)
	c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
}

// BadRequest sends a 400 response with the given message.
func BadRequest(c *gin.Context, msg string) {
	c.JSON(http.StatusBadRequest, ErrorResponse{Error: msg})
}
