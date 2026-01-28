package file

import (
	"time"
)

// File represents a file object in the system
type File struct {
	ID            string    `json:"id"`
	UserID        string    `json:"user_id"`
	Filename      string    `json:"filename"`
	StoragePath   string    `json:"-"` // Internal path
	ThumbnailPath *string   `json:"-"` // Internal path
	ContentType   string    `json:"content_type"`
	Size          int64     `json:"size"`
	CreatedAt     time.Time `json:"created_at"`
}

// FileURL returns the public URL for accessing a file by its ID.
func FileURL(id string) string {
	return "/files/" + id
}

// ThumbnailURL returns the public URL for accessing a file's thumbnail by its ID.
func ThumbnailURL(id string) string {
	return "/files/" + id + "/thumbnail"
}
