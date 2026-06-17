package http

import (
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/nekogravitycat/court-booking-backend/internal/file"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
)

// sanitizeContentDispositionFilename strips characters that could break the
// Content-Disposition header (quotes, backslashes, and control characters such
// as CR/LF), preventing header injection via the user-supplied filename.
func sanitizeContentDispositionFilename(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r < 0x20 || r == 0x7f || r == '"' || r == '\\' {
			continue
		}
		b.WriteRune(r)
	}
	cleaned := b.String()
	if cleaned == "" {
		return "download"
	}
	return cleaned
}

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
		response.Error(c, err)
		return
	}
	defer stream.Close()

	// Set headers. nosniff prevents browsers from MIME-sniffing the response
	// into an executable type (defense-in-depth against stored XSS).
	c.Header("Content-Type", fileInfo.ContentType)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Disposition", "inline; filename=\""+sanitizeContentDispositionFilename(fileInfo.Filename)+"\"")

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
		response.Error(c, err)
		return
	}
	defer stream.Close()

	// Set headers (thumbnails are always JPEG)
	c.Header("Content-Type", "image/jpeg")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Disposition", "inline; filename=\""+sanitizeContentDispositionFilename(fileInfo.Filename)+"_thumb.jpg\"")

	// Stream thumbnail to response
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, stream); err != nil {
		// Log error, but response already started
		return
	}
}
