package http

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nekogravitycat/court-booking-backend/internal/file"
)

type Handler struct {
	fileService file.Service
}

func NewHandler(fileService file.Service) *Handler {
	return &Handler{
		fileService: fileService,
	}
}

// ServeFile serves the file content by ID
func (h *Handler) ServeFile(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file ID is required"})
		return
	}

	// Download file stream and metadata
	stream, fileInfo, err := h.fileService.Download(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}
	defer stream.Close()

	// Set headers
	c.Header("Content-Type", fileInfo.ContentType)
	c.Header("Content-Disposition", "inline; filename=\""+fileInfo.Filename+"\"")

	// Stream file to response
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, stream); err != nil {
		// Log error, but response already started
		return
	}
}

// ServeThumbnail serves the thumbnail image by file ID
func (h *Handler) ServeThumbnail(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file ID is required"})
		return
	}

	// Download thumbnail stream and metadata
	stream, fileInfo, err := h.fileService.DownloadThumbnail(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "thumbnail not found"})
		return
	}
	defer stream.Close()

	// Set headers (thumbnails are always JPEG)
	c.Header("Content-Type", "image/jpeg")
	c.Header("Content-Disposition", "inline; filename=\""+fileInfo.Filename+"_thumb.jpg\"")

	// Stream thumbnail to response
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, stream); err != nil {
		// Log error, but response already started
		return
	}
}
