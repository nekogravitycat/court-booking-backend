package http

type FileUploadResponse struct {
	Message      string  `json:"message"`
	FileID       string  `json:"file_id"`
	URL          string  `json:"url"`
	ThumbnailURL *string `json:"thumbnail_url"`
}
