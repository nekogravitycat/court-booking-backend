package file

import (
	"net/http"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
)

var (
	ErrNotFound              = apperror.New(http.StatusNotFound, "file not found")
	ErrSizeMismatch          = apperror.New(http.StatusBadRequest, "file size mismatch")
	ErrSizeExceeded          = apperror.New(http.StatusBadRequest, "file size exceeds maximum allowed size")
	ErrContentTypeMismatch   = apperror.New(http.StatusBadRequest, "content type mismatch")
	ErrContentTypeNotAllowed = apperror.New(http.StatusBadRequest, "file content type is not allowed")
	ErrNotImage              = apperror.New(http.StatusBadRequest, "file is not an image")
	ErrThumbnailNotAvailable = apperror.New(http.StatusNotFound, "thumbnail not available for this file")
	ErrFileOpenFailed        = apperror.New(http.StatusBadRequest, "failed to open uploaded file")
	ErrFileReadFailed        = apperror.New(http.StatusBadRequest, "failed to read file content")
	ErrImageResizeFailed     = apperror.New(http.StatusInternalServerError, "failed to resize image")
	ErrStorageSaveFailed     = apperror.New(http.StatusInternalServerError, "failed to save file to storage")
	ErrStorageGetFailed      = apperror.New(http.StatusInternalServerError, "failed to retrieve file from storage")
)

// File represents a file object in the system
type File struct {
	ID            string
	UserID        string
	Filename      string
	StoragePath   string
	ThumbnailPath *string
	ContentType   string
	Size          int64
	CreatedAt     time.Time
}

// FileURL returns the public URL for accessing a file by its ID.
func FileURL(id string) string {
	return "/files/" + id
}

// ThumbnailURL returns the public URL for accessing a file's thumbnail by its ID.
func ThumbnailURL(id string) string {
	return "/files/" + id + "/thumbnail"
}
