package file

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/storage"
)

type UploadInput struct {
	FileHeader   *multipart.FileHeader
	UserID       string   // User ID of the uploader
	MaxSizeBytes int64    // Maximum file size in bytes (0 = no limit)
	AllowedTypes []string // Allowed MIME types (empty = allow all)
	ResizeImage  bool     // If true, validates file is an image and resizes to 1000x1000 max in .jpg format
}

type Service interface {
	Upload(ctx context.Context, input UploadInput) (*File, error)
	Delete(ctx context.Context, id string) error
	Get(ctx context.Context, id string) (*File, error)
	Download(ctx context.Context, id string) (io.ReadCloser, *File, error)
	DownloadThumbnail(ctx context.Context, id string) (io.ReadCloser, *File, error)
}

type service struct {
	repo    Repository
	storage storage.Storage
	imgProc *storage.ImageProcessor
}

func NewService(repo Repository, store storage.Storage) Service {
	return &service{
		repo:    repo,
		storage: store,
		imgProc: storage.NewImageProcessor(),
	}
}

// inferExtensionFromContentType maps common MIME types to file extensions
func inferExtensionFromContentType(contentType string) string {
	// Map of common MIME types to extensions
	extensionMap := map[string]string{
		"image/jpeg":       ".jpg",
		"image/jpg":        ".jpg",
		"image/png":        ".png",
		"image/gif":        ".gif",
		"image/webp":       ".webp",
		"image/svg+xml":    ".svg",
		"application/pdf":  ".pdf",
		"text/plain":       ".txt",
		"text/html":        ".html",
		"text/css":         ".css",
		"text/javascript":  ".js",
		"application/json": ".json",
		"application/xml":  ".xml",
	}

	if ext, ok := extensionMap[contentType]; ok {
		return ext
	}

	// If not found, return empty string (no extension)
	return ""
}

// Upload handles file upload, validation, and storage.
// It validates file size, content type, and generates a thumbnail if the file is an image.
func (s *service) Upload(ctx context.Context, input UploadInput) (*File, error) {
	header := input.FileHeader

	// Open file
	src, err := header.Open()
	if err != nil {
		return nil, ErrFileOpenFailed
	}
	defer src.Close()

	// Read content to buffer
	// TODO: For very large files, this might be an issue
	fileBytes, err := io.ReadAll(src)
	if err != nil {
		return nil, ErrFileReadFailed
	}

	// === Validate file size ===

	// Get actual file size
	actualSize := int64(len(fileBytes))

	// Verify claimed size matches actual size
	if header.Size != actualSize {
		return nil, ErrSizeMismatch
	}

	// Validate file size based on actual file content
	if input.MaxSizeBytes > 0 && actualSize > input.MaxSizeBytes {
		return nil, ErrSizeExceeded
	}

	// === Validate file content type ===

	// Detect actual content type from file bytes
	actualContentType := http.DetectContentType(fileBytes)

	// If ResizeImage is true, ensure it's an image
	if input.ResizeImage && !strings.HasPrefix(actualContentType, "image/") {
		return nil, ErrNotImage
	}

	// Ensure claimed type matches actual type
	if header.Header.Get("Content-Type") != actualContentType {
		return nil, ErrContentTypeMismatch
	}

	// Check if actual type is in allowed list
	if len(input.AllowedTypes) > 0 && !slices.Contains(input.AllowedTypes, actualContentType) {
		return nil, ErrContentTypeNotAllowed
	}

	// === Process file based on type ===

	// Determine which reader to use (original or resized image)
	var reader io.Reader
	if input.ResizeImage {
		// For image uploads with ResizeImage=true, resize before saving
		resizedReader, err := s.imgProc.GenerateThumbnail(bytes.NewReader(fileBytes), 1000, 1000)
		if err != nil {
			return nil, ErrImageResizeFailed
		}
		reader = resizedReader
		// Update content type to jpg
		actualContentType = "image/jpeg"
	} else {
		// Use original file as-is
		reader = bytes.NewReader(fileBytes)
	}

	// === Generate file ID and storage path, then save file to storage ===

	// Generate UUID and file path
	fileID := uuid.New().String()
	shard := fileID[:2]
	ext := inferExtensionFromContentType(actualContentType)
	storagePath := fmt.Sprintf("upload/%s/%s%s", shard, fileID, ext)

	// Save to storage
	if err := s.storage.Save(ctx, storagePath, reader); err != nil {
		return nil, ErrStorageSaveFailed
	}

	// Generate thumbnail if supported
	thumbnailPath := s.generateAndSaveThumbnail(ctx, fileBytes, actualContentType, fileID, shard)

	// === Create file in database ===

	f := &File{
		ID:            fileID,
		UserID:        input.UserID,
		Filename:      header.Filename,
		StoragePath:   storagePath,
		ThumbnailPath: thumbnailPath,
		ContentType:   actualContentType,
		Size:          actualSize,
		CreatedAt:     time.Now(),
	}

	if err := s.repo.Create(ctx, f); err != nil {
		// Cleanup storage if db fails
		// Log error if storage delete fails but don't fail upload
		if err = s.storage.Delete(ctx, storagePath); err != nil {
			log.Printf("failed to delete file: %v", err)
		}
		if thumbnailPath != nil {
			if err = s.storage.Delete(ctx, *thumbnailPath); err != nil {
				log.Printf("failed to delete thumbnail: %v", err)
			}
		}
		return nil, err
	}

	return f, nil
}

// generateAndSaveThumbnail generates and saves a thumbnail for supported file types.
// Returns the thumbnail path if successful, or nil if generation fails or is not supported.
// This function is designed to be extensible for other file types in the future (e.g., video, PDF).
func (s *service) generateAndSaveThumbnail(ctx context.Context, fileBytes []byte, contentType, fileID, shard string) *string {
	// Check if content type supports thumbnail generation
	if !s.supportsThumbnail(contentType) {
		return nil
	}

	// Generate thumbnail based on content type
	thumbReader, err := s.generateThumbnailForType(fileBytes, contentType)
	if err != nil {
		// Log error if thumbnail generation fails but don't fail upload
		log.Printf("failed to generate thumbnail for %s: %v", contentType, err)
		return nil
	}

	// Save thumbnail to storage
	thumbnailPath := fmt.Sprintf("upload/%s/%s_thumb.jpg", shard, fileID)
	if err := s.storage.Save(ctx, thumbnailPath, thumbReader); err != nil {
		log.Printf("failed to save thumbnail: %v", err)
		return nil
	}

	return &thumbnailPath
}

// supportsThumbnail checks if the given content type supports thumbnail generation.
// This can be extended to support more file types in the future.
func (s *service) supportsThumbnail(contentType string) bool {
	// Currently only images are supported
	// Future: add support for "video/*", "application/pdf", etc.
	return strings.HasPrefix(contentType, "image/")
}

// generateThumbnailForType generates a thumbnail based on the content type.
// This method can be extended to handle different file types differently.
func (s *service) generateThumbnailForType(fileBytes []byte, contentType string) (io.Reader, error) {
	// Currently only handle images
	if strings.HasPrefix(contentType, "image/") {
		return s.imgProc.GenerateThumbnail(bytes.NewReader(fileBytes), 200, 200)
	}

	// Future implementations:
	// - For videos: extract first frame and resize
	// - For PDFs: render first page and resize
	// - For office docs: generate preview

	return nil, fmt.Errorf("thumbnail generation not supported for content type: %s", contentType)
}

func (s *service) Delete(ctx context.Context, id string) error {
	f, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// Delete from storage
	// Log error if storage delete fails but don't fail delete
	if err := s.storage.Delete(ctx, f.StoragePath); err != nil {
		log.Printf("failed to delete file: %v", err)
	}
	if f.ThumbnailPath != nil {
		if err := s.storage.Delete(ctx, *f.ThumbnailPath); err != nil {
			log.Printf("failed to delete thumbnail: %v", err)
		}
	}

	// Delete from DB
	return s.repo.Delete(ctx, id)
}

func (s *service) Get(ctx context.Context, id string) (*File, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *service) Download(ctx context.Context, id string) (io.ReadCloser, *File, error) {
	// Get file metadata
	f, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	// Get file stream from storage
	stream, err := s.storage.Get(ctx, f.StoragePath)
	if err != nil {
		return nil, nil, ErrStorageGetFailed
	}

	return stream, f, nil
}

func (s *service) DownloadThumbnail(ctx context.Context, id string) (io.ReadCloser, *File, error) {
	// Get file metadata
	f, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	// Check if thumbnail exists
	if f.ThumbnailPath == nil {
		return nil, nil, ErrThumbnailNotAvailable
	}

	// Get thumbnail stream from storage
	stream, err := s.storage.Get(ctx, *f.ThumbnailPath)
	if err != nil {
		return nil, nil, ErrStorageGetFailed
	}

	return stream, f, nil
}
