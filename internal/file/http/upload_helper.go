package http

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/file"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
)

// FileUploadConfig defines the configuration for generic file uploads
type FileUploadConfig struct {
	FormFieldName string                                         // The name of the form field containing the file (default: "file")
	MaxSizeBytes  int64                                          // The maximum file size in bytes (0 = no limit)
	AllowedTypes  []string                                       // The list of allowed MIME types (empty = allow all)
	ResizeImage   bool                                           // If true, validates file is an image and resizes to 1000x1000 max in .jpg format
	AfterUpload   func(ctx context.Context, fileID string) error // Called after successful file upload (optional)
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
		c.JSON(http.StatusBadRequest, gin.H{"error": fieldName + " is required"})
		return
	}

	// Upload file to storage + DB
	f, err := h.fileService.Upload(c.Request.Context(), file.UploadInput{
		FileHeader:   fileHeader,
		UserID:       userID,
		MaxSizeBytes: config.MaxSizeBytes,
		AllowedTypes: config.AllowedTypes,
		ResizeImage:  config.ResizeImage,
	})

	if err != nil {
		response.Error(c, err)
		return
	}

	// After upload hook (e.g., update entity reference)
	if config.AfterUpload != nil {
		if err := config.AfterUpload(c.Request.Context(), f.ID); err != nil {
			// Rollback: delete file from storage and DB
			_ = h.fileService.Delete(c.Request.Context(), f.ID)
			response.Error(c, err)
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
