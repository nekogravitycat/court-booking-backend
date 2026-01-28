package file

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	Create(ctx context.Context, file *File) error
	GetByID(ctx context.Context, id string) (*File, error)
	Delete(ctx context.Context, id string) error
}

type repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &repository{db: db}
}

func (r *repository) Create(ctx context.Context, f *File) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Insert("files").
		Columns("id", "user_id", "filename", "storage_path", "thumbnail_path", "content_type", "size", "created_at").
		Values(f.ID, f.UserID, f.Filename, f.StoragePath, f.ThumbnailPath, f.ContentType, f.Size, f.CreatedAt).
		ToSql()
	if err != nil {
		return fmt.Errorf("failed to build query: %w", err)
	}

	_, err = r.db.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to create file record: %w", err)
	}
	return nil
}

func (r *repository) GetByID(ctx context.Context, id string) (*File, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select("id", "user_id", "filename", "storage_path", "thumbnail_path", "content_type", "size", "created_at").
		From("files").
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	f := &File{}
	var thumbnailPath sql.NullString

	err = r.db.QueryRow(ctx, query, args...).Scan(
		&f.ID,
		&f.UserID,
		&f.Filename,
		&f.StoragePath,
		&thumbnailPath,
		&f.ContentType,
		&f.Size,
		&f.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get file: %w", err)
	}

	if thumbnailPath.Valid {
		f.ThumbnailPath = &thumbnailPath.String
	}

	return f, nil
}

func (r *repository) Delete(ctx context.Context, id string) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Delete("files").
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("failed to build query: %w", err)
	}

	_, err = r.db.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to delete file record: %w", err)
	}
	return nil
}
