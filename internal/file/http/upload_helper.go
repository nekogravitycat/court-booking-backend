package http

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/file"
)

// FileUploadConfig defines the configuration for generic file uploads
type FileUploadConfig struct {
	// FormFieldName is the name of the form field containing the file (default: "file")
	FormFieldName string

	// AfterUpload is called after successful file upload (optional)
	// Can be used to update entity references, create associations, etc.
	AfterUpload func(ctx context.Context, fileID string) error

	// Error messages (optional)
	FileRequiredMsg string
}

// HandleFileUpload is a generic reusable handler for file uploads.
// It handles file upload, optional after-upload hook, and rollback on hook failure.
func (h *Handler) HandleFileUpload(c *gin.Context, config FileUploadConfig) {
	userID := auth.GetUserID(c)

	// Get file from form
	fieldName := config.FormFieldName
	if fieldName == "" {
		fieldName = "file"
	}

	fileHeader, err := c.FormFile(fieldName)
	if err != nil {
		msg := config.FileRequiredMsg
		if msg == "" {
			msg = fieldName + " is required"
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}

	// Upload file to storage + DB
	f, err := h.fileService.Upload(c.Request.Context(), fileHeader, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// After upload hook (e.g., update entity reference)
	if config.AfterUpload != nil {
		if err := config.AfterUpload(c.Request.Context(), f.ID); err != nil {
			// Rollback: delete file from storage and DB
			_ = h.fileService.Delete(c.Request.Context(), f.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	// Build response
	url := file.FileURL(f.ID)
	var thumbURL *string
	if f.ThumbnailPath != nil {
		t := file.ThumbnailURL(f.ID)
		thumbURL = &t
	}

	response := FileUploadResponse{
		Message:      "file uploaded successfully",
		FileID:       f.ID,
		URL:          url,
		ThumbnailURL: thumbURL,
	}

	c.JSON(http.StatusOK, response)
}
