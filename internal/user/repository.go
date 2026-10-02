package user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	"log"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/pagination"
)

// Repository defines methods for accessing user data from storage.
type Repository interface {
	GetSkillLevel(ctx context.Context, userID, sportID string) (*SportSkillLevel, error)
	ListSkillLevels(ctx context.Context, userID string) ([]*SportSkillLevel, error)
	SetSkillLevel(ctx context.Context, userID, sportID string, level int) (*SportSkillLevel, error)
	DeleteSkillLevel(ctx context.Context, userID, sportID string) error
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetByID(ctx context.Context, id string) (*User, error)
	Create(ctx context.Context, u *User) error
	UpdateLastLogin(ctx context.Context, id string, t time.Time) error
	List(ctx context.Context, filter UserFilter) ([]*User, int, error)
	Update(ctx context.Context, id string, req UpdateUserRequest) error
	UpdateAvatar(ctx context.Context, id string, avatar *string) error
	Delete(ctx context.Context, id string) error
	// CountActiveSystemAdminsExcept counts active system admins other than excludeID.
	CountActiveSystemAdminsExcept(ctx context.Context, excludeID string) (int, error)

	// Pickup host role management
	IsPickupHost(ctx context.Context, userID string) (bool, error)
	AddPickupHost(ctx context.Context, userID string) error
	RemovePickupHost(ctx context.Context, userID string) error
	ListPickupHosts(ctx context.Context, filter UserFilter) ([]*User, int, error)
}

type pgxUserRepository struct {
	pool *pgxpool.Pool
}

// NewPgxRepository creates a new Repository implementation using pgxpool.
func NewPgxRepository(pool *pgxpool.Pool) Repository {
	return &pgxUserRepository{
		pool: pool,
	}
}

func (r *pgxUserRepository) GetByEmail(ctx context.Context, email string) (*User, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select(
		"u.id", "u.email", "u.username", "u.password_hash", "u.display_name", "u.phone", "u.gender", "u.birth_date", "u.line_id", "u.avatar", "u.created_at",
		"u.last_login_at", "u.is_active", "u.is_system_admin",
		"EXISTS(SELECT 1 FROM public.pickup_hosts ph WHERE ph.user_id = u.id) AS is_pickup_host",
		`COALESCE(
				(
					SELECT json_agg(json_build_object(
						'id', o.id,
						'name', o.name,
						'owner', (o.owner_id = u.id),
						'organization_manager', EXISTS(SELECT 1 FROM public.organization_managers om WHERE om.organization_id = o.id AND om.user_id = u.id),
						'location_manager', COALESCE((SELECT json_agg(lm.location_id) FROM public.location_managers lm WHERE lm.organization_id = o.id AND lm.user_id = u.id), '[]'::json)
					))
					FROM public.organizations o
					WHERE (o.owner_id = u.id OR o.id IN (
						SELECT organization_id FROM public.organization_members WHERE user_id = u.id
					)) AND o.is_active = true
				),
				'[]'::json
			) AS organizations`,
	).
		From("public.users u").
		Where(squirrel.Eq{"u.email": email}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get user by email query failed: %w", err)
	}

	row := r.pool.QueryRow(ctx, query, args...)

	var u User
	var orgsJSON []byte

	if err := row.Scan(
		&u.ID,
		&u.Email,
		&u.Username,
		&u.PasswordHash,
		&u.DisplayName,
		&u.Phone,
		&u.Gender,
		&u.BirthDate,
		&u.LineID,
		&u.Avatar,
		&u.CreatedAt,
		&u.LastLoginAt,
		&u.IsActive,
		&u.IsSystemAdmin,
		&u.IsPickupHost,
		&orgsJSON, // Scan JSON for organizations
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("GetByEmail query failed: %w", err)
	}

	// Try parse the organizations JSON into the slice
	if len(orgsJSON) > 0 {
		if err := json.Unmarshal(orgsJSON, &u.Organizations); err != nil {
			log.Printf("warning: failed to unmarshal organizations for user %s: %v", u.ID, err)
		}
	}

	return &u, nil
}

func (r *pgxUserRepository) GetByID(ctx context.Context, id string) (*User, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select(
		"u.id", "u.email", "u.username", "u.password_hash", "u.display_name", "u.phone", "u.gender", "u.birth_date", "u.line_id", "u.avatar", "u.created_at",
		"u.last_login_at", "u.is_active", "u.is_system_admin",
		"EXISTS(SELECT 1 FROM public.pickup_hosts ph WHERE ph.user_id = u.id) AS is_pickup_host",
		`COALESCE(
				(
					SELECT json_agg(json_build_object(
						'id', o.id,
						'name', o.name,
						'owner', (o.owner_id = u.id),
						'organization_manager', EXISTS(SELECT 1 FROM public.organization_managers om WHERE om.organization_id = o.id AND om.user_id = u.id),
						'location_manager', COALESCE((SELECT json_agg(lm.location_id) FROM public.location_managers lm WHERE lm.organization_id = o.id AND lm.user_id = u.id), '[]'::json)
					))
					FROM public.organizations o
					WHERE (o.owner_id = u.id OR o.id IN (
						SELECT organization_id FROM public.organization_members WHERE user_id = u.id
					)) AND o.is_active = true
				),
				'[]'::json
			) AS organizations`,
	).
		From("public.users u").
		Where(squirrel.Eq{"u.id": id}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get user by id query failed: %w", err)
	}

	row := r.pool.QueryRow(ctx, query, args...)

	var u User
	var orgsJSON []byte

	if err := row.Scan(
		&u.ID,
		&u.Email,
		&u.Username,
		&u.PasswordHash,
		&u.DisplayName,
		&u.Phone,
		&u.Gender,
		&u.BirthDate,
		&u.LineID,
		&u.Avatar,
		&u.CreatedAt,
		&u.LastLoginAt,
		&u.IsActive,
		&u.IsSystemAdmin,
		&u.IsPickupHost,
		&orgsJSON, // Scan JSON for organizations
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("GetByID query failed: %w", err)
	}

	// Try parse the organizations JSON into the slice
	if len(orgsJSON) > 0 {
		if err := json.Unmarshal(orgsJSON, &u.Organizations); err != nil {
			log.Printf("warning: failed to unmarshal organizations for user %s: %v", u.ID, err)
		}
	}

	return &u, nil
}

func (r *pgxUserRepository) Create(ctx context.Context, u *User) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Insert("public.users").
		Columns("email", "username", "password_hash", "display_name", "gender", "birth_date", "line_id", "is_active", "is_system_admin").
		Values(u.Email, u.Username, u.PasswordHash, u.DisplayName, u.Gender, u.BirthDate, u.LineID, u.IsActive, u.IsSystemAdmin).
		Suffix("RETURNING id, created_at").
		ToSql()
	if err != nil {
		return fmt.Errorf("build create user query failed: %w", err)
	}

	if err := r.pool.QueryRow(ctx, query, args...).Scan(&u.ID, &u.CreatedAt); err != nil {
		var e *pgconn.PgError
		if errors.As(err, &e) && e.Code == pgerrcode.UniqueViolation {
			if e.ConstraintName == "users_username_key" {
				return ErrUsernameAlreadyUsed
			}
			return ErrEmailAlreadyUsed
		}
		return fmt.Errorf("Create user failed: %w", err)
	}

	return nil
}

func (r *pgxUserRepository) UpdateLastLogin(ctx context.Context, id string, t time.Time) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Update("public.users").
		Set("last_login_at", t).
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build update last login query failed: %w", err)
	}

	ct, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("UpdateLastLogin failed: %w", err)
	}

	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

func (r *pgxUserRepository) List(ctx context.Context, filter UserFilter) ([]*User, int, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	queryBuilder := psql.Select(
		"u.id", "u.email", "u.username", "u.password_hash", "u.display_name", "u.phone", "u.gender", "u.birth_date", "u.line_id", "u.avatar", "u.created_at",
		"u.last_login_at", "u.is_active", "u.is_system_admin",
		"EXISTS(SELECT 1 FROM public.pickup_hosts ph WHERE ph.user_id = u.id) AS is_pickup_host",
		"count(*) OVER() AS total_count",
		`COALESCE(
				(
					SELECT json_agg(json_build_object(
						'id', o.id,
						'name', o.name,
						'owner', (o.owner_id = u.id),
						'organization_manager', EXISTS(SELECT 1 FROM public.organization_managers om WHERE om.organization_id = o.id AND om.user_id = u.id),
						'location_manager', COALESCE((SELECT json_agg(lm.location_id) FROM public.location_managers lm WHERE lm.organization_id = o.id AND lm.user_id = u.id), '[]'::json)
					))
					FROM public.organizations o
					WHERE (o.owner_id = u.id OR o.id IN (
						SELECT organization_id FROM public.organization_members WHERE user_id = u.id
					)) AND o.is_active = true
				),
				'[]'::json
			) AS organizations`,
	).From("public.users u")

	// Dynamic filtering
	if len(filter.IDs) > 0 {
		queryBuilder = queryBuilder.Where(squirrel.Eq{"u.id": filter.IDs})
	}
	if filter.Email != "" {
		queryBuilder = queryBuilder.Where(squirrel.ILike{"email": "%" + request.EscapeLike(filter.Email) + "%"})
	}
	if filter.DisplayName != "" {
		queryBuilder = queryBuilder.Where(squirrel.ILike{"display_name": "%" + request.EscapeLike(filter.DisplayName) + "%"})
	}
	if filter.IsActive != nil {
		queryBuilder = queryBuilder.Where(squirrel.Eq{"is_active": *filter.IsActive})
	}
	if filter.PickupHostsOnly {
		queryBuilder = queryBuilder.Where("EXISTS(SELECT 1 FROM public.pickup_hosts ph2 WHERE ph2.user_id = u.id)")
	}

	// Sorting
	orderBy := "u.created_at"
	switch filter.SortBy {
	case "name":
		orderBy = "u.display_name"
	case "email":
		orderBy = "u.email"
	}

	orderDir := "DESC"
	if filter.SortOrder != "" {
		orderDir = filter.SortOrder
	}

	queryBuilder = queryBuilder.OrderBy(orderBy+" "+orderDir, "u.id ASC")

	// Pagination
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 20
	}
	offset := (filter.Page - 1) * filter.PageSize

	queryBuilder = queryBuilder.Limit(uint64(filter.PageSize)).Offset(uint64(offset))

	sql, args, err := queryBuilder.ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("build list users query failed: %w", err)
	}

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list users failed: %w", err)
	}
	defer rows.Close()

	var users []*User
	var total int

	for rows.Next() {
		var u User
		var orgsJSON []byte

		if err := rows.Scan(
			&u.ID,
			&u.Email,
			&u.Username,
			&u.PasswordHash,
			&u.DisplayName,
			&u.Phone,
			&u.Gender,
			&u.BirthDate,
			&u.LineID,
			&u.Avatar,
			&u.CreatedAt,
			&u.LastLoginAt,
			&u.IsActive,
			&u.IsSystemAdmin,
			&u.IsPickupHost,
			&total,    // Scan the window function result
			&orgsJSON, // Scan the JSON result for organizations
		); err != nil {
			return nil, 0, fmt.Errorf("scan user failed: %w", err)
		}

		// Parse the organizations JSON into the slice
		// pgx can actually scan directly into structs if setup correctly,
		// but using json.Unmarshal is safer and simpler for this specific case without extra config.
		if len(orgsJSON) > 0 {
			if err := json.Unmarshal(orgsJSON, &u.Organizations); err != nil {
				// Log the error but continue; we don't want one bad record to fail the whole list.
				log.Printf("warning: failed to unmarshal organizations for user %s: %v", u.ID, err)
			}
		}

		users = append(users, &u)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	rows.Close()
	if total == 0 {
		total, err = pagination.Count(ctx, r.pool, queryBuilder)
		if err != nil {
			return nil, 0, err
		}
	}
	return users, total, nil
}

func (r *pgxUserRepository) Update(ctx context.Context, id string, req UpdateUserRequest) error {
	query := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar).Update("public.users").Where(squirrel.Eq{"id": id})
	changed := false
	if req.DisplayName != nil {
		query = query.Set("display_name", req.DisplayName)
		changed = true
	}
	if req.LineID != nil {
		changed = true
		if *req.LineID == "" {
			query = query.Set("line_id", nil)
		} else {
			query = query.Set("line_id", *req.LineID)
		}
	}
	if req.Phone != nil {
		// An empty string clears the phone number.
		if *req.Phone == "" {
			query = query.Set("phone", nil)
		} else {
			query = query.Set("phone", *req.Phone)
		}
		changed = true
	}
	if req.Gender != nil {
		query = query.Set("gender", req.Gender)
		changed = true
	}
	if req.BirthDate != nil {
		query = query.Set("birth_date", req.BirthDate)
		changed = true
	}
	if req.IsActive != nil {
		query = query.Set("is_active", *req.IsActive)
		changed = true
	}
	if req.IsSystemAdmin != nil {
		query = query.Set("is_system_admin", *req.IsSystemAdmin)
		changed = true
	}
	if !changed {
		return nil
	}
	sql, args, err := query.ToSql()
	if err != nil {
		return err
	}
	ct, err := r.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("update user failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *pgxUserRepository) UpdateAvatar(ctx context.Context, id string, avatar *string) error {
	ct, err := r.pool.Exec(ctx, "UPDATE public.users SET avatar = $2 WHERE id = $1", id, avatar)
	if err != nil {
		return fmt.Errorf("update avatar failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *pgxUserRepository) IsPickupHost(ctx context.Context, userID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM public.pickup_hosts WHERE user_id = $1)",
		userID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check pickup host failed: %w", err)
	}
	return exists, nil
}

func (r *pgxUserRepository) AddPickupHost(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx,
		"INSERT INTO public.pickup_hosts (user_id) VALUES ($1)",
		userID,
	)
	if err != nil {
		var e *pgconn.PgError
		if errors.As(err, &e) {
			switch e.Code {
			case pgerrcode.UniqueViolation:
				return ErrAlreadyPickupHost
			case pgerrcode.ForeignKeyViolation:
				return ErrNotFound
			}
		}
		return fmt.Errorf("add pickup host failed: %w", err)
	}
	return nil
}

func (r *pgxUserRepository) RemovePickupHost(ctx context.Context, userID string) error {
	ct, err := r.pool.Exec(ctx,
		"DELETE FROM public.pickup_hosts WHERE user_id = $1",
		userID,
	)
	if err != nil {
		return fmt.Errorf("remove pickup host failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotPickupHost
	}
	return nil
}

func (r *pgxUserRepository) ListPickupHosts(ctx context.Context, filter UserFilter) ([]*User, int, error) {
	filter.PickupHostsOnly = true
	return r.List(ctx, filter)
}

func (r *pgxUserRepository) Delete(ctx context.Context, id string) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Update("public.users").
		Set("is_active", false).
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build delete user query failed: %w", err)
	}

	ct, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("delete user failed: %w", err)
	}

	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

func (r *pgxUserRepository) CountActiveSystemAdminsExcept(ctx context.Context, excludeID string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		"SELECT count(*) FROM public.users WHERE is_system_admin = true AND is_active = true AND id <> $1",
		excludeID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count active system admins failed: %w", err)
	}
	return n, nil
}
