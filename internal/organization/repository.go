package organization

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nekogravitycat/court-booking-backend/internal/db"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/pagination"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

// Repository defines methods for accessing organization data.
type Repository interface {
	// Organization methods
	Create(ctx context.Context, org *Organization) error
	GetByID(ctx context.Context, id string) (*Organization, error)
	List(ctx context.Context, filter OrganizationFilter) ([]*Organization, int, error)
	SetCover(ctx context.Context, id string, cover *string) (*string, error)
	UpdateDetails(ctx context.Context, id string, req UpdateOrganizationRequest) error
	Delete(ctx context.Context, id string) error
	// Organization Manager methods
	AddOrganizationManager(ctx context.Context, orgID string, userID string) error
	RemoveOrganizationManager(ctx context.Context, orgID string, userID string) error
	IsOrganizationManager(ctx context.Context, orgID string, userID string) (bool, error)
	ListOrganizationManagers(ctx context.Context, orgID string, filter ManagerFilter) ([]*user.User, int, error)
	// Organization Member methods
	AddMember(ctx context.Context, orgID string, userID string) error
	RemoveMember(ctx context.Context, orgID string, userID string) error
	IsMember(ctx context.Context, orgID string, userID string) (bool, error)
	ListMembers(ctx context.Context, orgID string, filter ManagerFilter) ([]*user.User, int, error)
}

type pgxRepository struct {
	pool *pgxpool.Pool
}

// NewPgxRepository creates a new organization repository.
func NewPgxRepository(pool *pgxpool.Pool) Repository {
	return &pgxRepository{pool: pool}
}

// ------------------------
//   Organization methods
// ------------------------

func (r *pgxRepository) Create(ctx context.Context, org *Organization) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Insert("public.organizations").
		Columns("name", "owner_id", "cover", "is_active").
		Values(org.Name, org.OwnerID, org.Cover, org.IsActive).
		Suffix("RETURNING id, created_at").
		ToSql()
	if err != nil {
		return fmt.Errorf("build create organization query failed: %w", err)
	}

	return r.pool.QueryRow(ctx, query, args...).
		Scan(&org.ID, &org.CreatedAt)
}

func (r *pgxRepository) GetByID(ctx context.Context, id string) (*Organization, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select("id", "name", "owner_id", "cover", "created_at", "is_active").
		From("public.organizations").
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get organization query failed: %w", err)
	}

	row := r.pool.QueryRow(ctx, query, args...)

	var org Organization
	if err := row.Scan(&org.ID, &org.Name, &org.OwnerID, &org.Cover, &org.CreatedAt, &org.IsActive); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrgNotFound
		}
		return nil, fmt.Errorf("GetByID failed: %w", err)
	}
	return &org, nil
}

func (r *pgxRepository) List(ctx context.Context, filter OrganizationFilter) ([]*Organization, int, error) {
	// Base query with window function for total count
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	queryBuilder := psql.Select("id", "name", "owner_id", "cover", "created_at", "is_active", "count(*) OVER() AS total_count").
		From("public.organizations").Where(squirrel.Eq{"is_active": true})

	orderBy := "id"
	if filter.SortBy != "" {
		orderBy = filter.SortBy
	}

	orderDir := "DESC"
	if filter.SortOrder != "" {
		orderDir = filter.SortOrder
	}

	queryBuilder = queryBuilder.OrderBy(orderBy+" "+orderDir, "id ASC")

	return pagination.Collect(ctx, r.pool, queryBuilder, filter.Page, filter.PageSize, "organization", func(rows pgx.Rows, total *int) (*Organization, error) {
		var o Organization
		if err := rows.Scan(&o.ID, &o.Name, &o.OwnerID, &o.Cover, &o.CreatedAt, &o.IsActive, total); err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}
		return &o, nil
	})
}

// SetCover replaces only the cover reference (so it cannot overwrite concurrent
// edits to other columns) and returns the previous cover.
func (r *pgxRepository) SetCover(ctx context.Context, id string, cover *string) (*string, error) {
	var old *string
	err := r.pool.QueryRow(ctx,
		`UPDATE public.organizations t SET cover = $2
		 FROM (SELECT cover AS old_cover FROM public.organizations WHERE id = $1 FOR UPDATE) o
		 WHERE t.id = $1
		 RETURNING o.old_cover`,
		id, cover).Scan(&old)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrgNotFound
		}
		return nil, fmt.Errorf("set organization cover failed: %w", err)
	}
	return old, nil
}

func (r *pgxRepository) Delete(ctx context.Context, id string) error {
	// Soft delete implementation
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Update("public.organizations").
		Set("is_active", false).
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build delete (soft) organization query failed: %w", err)
	}

	ct, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("Delete (soft) failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrOrgNotFound
	}
	return nil
}

// -----------------------------
//   Organization Manager methods
// -----------------------------

func (r *pgxRepository) AddOrganizationManager(ctx context.Context, orgID string, userID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var ownerID string
	if err := tx.QueryRow(ctx, "SELECT owner_id FROM public.organizations WHERE id = $1 FOR UPDATE", orgID).Scan(&ownerID); err != nil {
		return err
	}
	if ownerID == userID {
		return ErrOwnerRoleConflict
	}
	var conflict, member bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM public.location_managers WHERE organization_id = $1 AND user_id = $2), EXISTS(SELECT 1 FROM public.organization_members WHERE organization_id = $1 AND user_id = $2)", orgID, userID).Scan(&conflict, &member); err != nil {
		return err
	}
	if conflict {
		return ErrLocationRoleConflict
	}
	if !member {
		return ErrMemberRequired
	}
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Insert("public.organization_managers").
		Columns("organization_id", "user_id").
		Values(orgID, userID).
		ToSql()
	if err != nil {
		return fmt.Errorf("build add org manager query failed: %w", err)
	}

	_, err = tx.Exec(ctx, query, args...)
	if err != nil {
		if db.IsUniqueViolation(err) {
			return ErrUserAlreadyMember
		}
		return fmt.Errorf("AddOrganizationManager failed: %w", err)
	}
	return tx.Commit(ctx)
}

func (r *pgxRepository) RemoveOrganizationManager(ctx context.Context, orgID string, userID string) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Delete("public.organization_managers").
		Where(squirrel.Eq{"organization_id": orgID}).
		Where(squirrel.Eq{"user_id": userID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build remove org manager query failed: %w", err)
	}

	ct, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("RemoveOrganizationManager failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrUserNotMember
	}
	return nil
}

func (r *pgxRepository) IsOrganizationManager(ctx context.Context, orgID string, userID string) (bool, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select("1").
		From("public.organization_managers").
		Where(squirrel.Eq{"organization_id": orgID}).
		Where(squirrel.Eq{"user_id": userID}).
		ToSql()
	if err != nil {
		return false, fmt.Errorf("build check org manager query failed: %w", err)
	}

	var one int
	err = r.pool.QueryRow(ctx, query, args...).Scan(&one)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("IsOrganizationManager failed: %w", err)
	}
	return true, nil
}

func (r *pgxRepository) ListOrganizationManagers(ctx context.Context, orgID string, filter ManagerFilter) ([]*user.User, int, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	queryBuilder := psql.Select(
		"u.id", "u.email", "u.display_name", "u.created_at", "u.is_active", "count(*) OVER() AS total_count",
	).
		From("public.organization_managers om").
		Join("public.users u ON om.user_id = u.id").
		Where(squirrel.Eq{"om.organization_id": orgID})

	orderBy := "u.display_name"

	switch filter.SortBy {
	case "name":
		orderBy = "u.display_name"
	case "email":
		orderBy = "u.email"
	case "created_at":
		orderBy = "u.created_at"
	default:
		orderBy = "u.display_name"
	}

	orderDir := "ASC"
	if filter.SortOrder != "" {
		orderDir = filter.SortOrder
	}

	queryBuilder = queryBuilder.OrderBy(orderBy+" "+orderDir, "u.id ASC")

	return pagination.Collect(ctx, r.pool, queryBuilder, filter.Page, filter.PageSize, "org manager", func(rows pgx.Rows, total *int) (*user.User, error) {
		var u user.User
		if err := rows.Scan(&u.ID, &u.Email, &u.DisplayName, &u.CreatedAt, &u.IsActive, total); err != nil {
			return nil, fmt.Errorf("scan org manager failed: %w", err)
		}
		return &u, nil
	})
}

// -----------------------------
//   Organization Member methods
// -----------------------------

func (r *pgxRepository) AddMember(ctx context.Context, orgID string, userID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var ownerID string
	if err := tx.QueryRow(ctx, "SELECT owner_id FROM public.organizations WHERE id = $1 FOR UPDATE", orgID).Scan(&ownerID); err != nil {
		return err
	}
	if ownerID == userID {
		return ErrOwnerRoleConflict
	}
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Insert("public.organization_members").
		Columns("organization_id", "user_id").
		Values(orgID, userID).
		Suffix("ON CONFLICT DO NOTHING").
		ToSql()
	if err != nil {
		return fmt.Errorf("build add member query failed: %w", err)
	}

	_, err = tx.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("AddMember failed: %w", err)
	}
	return tx.Commit(ctx)
}

func (r *pgxRepository) RemoveMember(ctx context.Context, orgID string, userID string) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Delete("public.organization_members").
		Where(squirrel.Eq{"organization_id": orgID}).
		Where(squirrel.Eq{"user_id": userID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build remove member query failed: %w", err)
	}

	ct, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("RemoveMember failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrUserNotMember
	}
	return nil
}

func (r *pgxRepository) IsMember(ctx context.Context, orgID string, userID string) (bool, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select("1").
		From("public.organization_members").
		Where(squirrel.Eq{"organization_id": orgID}).
		Where(squirrel.Eq{"user_id": userID}).
		ToSql()
	if err != nil {
		return false, fmt.Errorf("build check member query failed: %w", err)
	}

	var one int
	err = r.pool.QueryRow(ctx, query, args...).Scan(&one)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("IsMember failed: %w", err)
	}
	return true, nil
}

func (r *pgxRepository) ListMembers(ctx context.Context, orgID string, filter ManagerFilter) ([]*user.User, int, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	queryBuilder := psql.Select(
		"u.id", "u.email", "u.display_name", "u.created_at", "u.is_active", "count(*) OVER() AS total_count",
	).
		From("public.organization_members om").
		Join("public.users u ON om.user_id = u.id").
		Where(squirrel.Eq{"om.organization_id": orgID})

	orderBy := "u.display_name"
	switch filter.SortBy {
	case "name":
		orderBy = "u.display_name"
	case "email":
		orderBy = "u.email"
	case "created_at":
		orderBy = "u.created_at"
	default:
		orderBy = "u.display_name"
	}

	orderDir := "ASC"
	if filter.SortOrder != "" {
		orderDir = filter.SortOrder
	}

	queryBuilder = queryBuilder.OrderBy(orderBy+" "+orderDir, "u.id ASC")

	return pagination.Collect(ctx, r.pool, queryBuilder, filter.Page, filter.PageSize, "member", func(rows pgx.Rows, total *int) (*user.User, error) {
		var u user.User
		if err := rows.Scan(&u.ID, &u.Email, &u.DisplayName, &u.CreatedAt, &u.IsActive, total); err != nil {
			return nil, fmt.Errorf("scan member failed: %w", err)
		}
		return &u, nil
	})
}

// UpdateDetails serializes owner transfers with member and manager assignment.
func (r *pgxRepository) UpdateDetails(ctx context.Context, id string, req UpdateOrganizationRequest) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var ownerID string
	if err := tx.QueryRow(ctx, "SELECT owner_id FROM public.organizations WHERE id = $1 FOR UPDATE", id).Scan(&ownerID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrOrgNotFound
		}
		return err
	}
	query := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar).Update("public.organizations").Where(squirrel.Eq{"id": id})
	changed := false
	if req.Name != nil {
		query = query.Set("name", *req.Name)
		changed = true
	}
	if req.IsActive != nil {
		query = query.Set("is_active", *req.IsActive)
		changed = true
	}
	if req.OwnerID != nil {
		var conflict bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM public.location_managers WHERE organization_id = $1 AND user_id = $2)", id, *req.OwnerID).Scan(&conflict); err != nil {
			return err
		}
		if conflict {
			return ErrLocationRoleConflict
		}
		if _, err := tx.Exec(ctx, "DELETE FROM public.organization_members WHERE organization_id = $1 AND user_id = $2", id, *req.OwnerID); err != nil {
			return err
		}
		query = query.Set("owner_id", *req.OwnerID)
		changed = true
	}
	if changed {
		sql, args, err := query.ToSql()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
