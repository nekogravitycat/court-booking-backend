package file

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/storage"
)

type Service interface {
	Upload(ctx context.Context, header *multipart.FileHeader, userID string) (*File, error)
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

func (s *service) Upload(ctx context.Context, header *multipart.FileHeader, userID string) (*File, error) {
	// Validate file
	src, err := header.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open uploaded file: %w", err)
	}
	defer src.Close()

	// Read content to buffer for multiple reads (hashing, processing, saving)
	// For very large files, this might be an issue, but for images it's fine.
	// Limit read size if needed.
	fileBytes, err := io.ReadAll(src)
	if err != nil {
		return nil, fmt.Errorf("failed to read file content: %w", err)
	}

	contentType := header.Header.Get("Content-Type")
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext == "" {
		// Fallback guessing from content type if needed, or just keep empty
	}

	// Generate UUID
	fileID := uuid.New().String()

	// Sharding path: upload/ab/UUID.ext
	shard := fileID[:2]
	storagePath := fmt.Sprintf("upload/%s/%s%s", shard, fileID, ext)

	// Save original file
	if err := s.storage.Save(ctx, storagePath, bytes.NewReader(fileBytes)); err != nil {
		return nil, fmt.Errorf("failed to save file to storage: %w", err)
	}

	var thumbnailPath *string
	// Generate thumbnail if image
	if strings.HasPrefix(contentType, "image/") {
		thumbReader, err := s.imgProc.GenerateThumbnail(bytes.NewReader(fileBytes), 200, 200)
		if err == nil {
			tPath := fmt.Sprintf("upload/%s/%s_thumb.jpg", shard, fileID)
			if err := s.storage.Save(ctx, tPath, thumbReader); err == nil {
				thumbnailPath = &tPath
			}
		}
		// Log error if thumbnail generation fails but don't fail upload
	}

	f := &File{
		ID:            fileID,
		UserID:        userID,
		Filename:      header.Filename,
		StoragePath:   storagePath,
		ThumbnailPath: thumbnailPath,
		ContentType:   contentType,
		Size:          header.Size,
		CreatedAt:     time.Now(),
	}

	if err := s.repo.Create(ctx, f); err != nil {
		// Cleanup storage if db fails
		_ = s.storage.Delete(ctx, storagePath)
		if thumbnailPath != nil {
			_ = s.storage.Delete(ctx, *thumbnailPath)
		}
		return nil, err
	}

	return f, nil
}

func (s *service) Delete(ctx context.Context, id string) error {
	f, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// Delete from storage
	if err := s.storage.Delete(ctx, f.StoragePath); err != nil {
		// Log error but continue to delete from DB?
		// Or fail? Usually better to try best effort cleanup.
	}

	if f.ThumbnailPath != nil {
		_ = s.storage.Delete(ctx, *f.ThumbnailPath)
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
		return nil, nil, fmt.Errorf("failed to retrieve file from storage: %w", err)
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
		return nil, nil, fmt.Errorf("thumbnail not available for this file")
	}

	// Get thumbnail stream from storage
	stream, err := s.storage.Get(ctx, *f.ThumbnailPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to retrieve thumbnail from storage: %w", err)
	}

	return stream, f, nil
}
