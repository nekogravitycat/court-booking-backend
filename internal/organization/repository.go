package organization

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines methods for accessing organization data.
type Repository interface {
	// Organization methods
	Create(ctx context.Context, org *Organization) error
	GetByID(ctx context.Context, id string) (*Organization, error)
	List(ctx context.Context, filter OrganizationFilter) ([]*Organization, int, error)
	Update(ctx context.Context, org *Organization) error
	Delete(ctx context.Context, id string) error
	// Member methods
	GetMember(ctx context.Context, orgID string, userID string) (*Member, error)
	AddMember(ctx context.Context, orgID string, userID string, role string) error
	RemoveMember(ctx context.Context, orgID string, userID string) error
	UpdateMemberRole(ctx context.Context, orgID string, userID string, role string) error
	ListMembers(ctx context.Context, orgID string, filter MemberFilter) ([]*Member, int, error)
	// Location Manager methods
	AddLocationManager(ctx context.Context, locationID string, userID string) error
	RemoveLocationManager(ctx context.Context, locationID string, userID string) error
	IsLocationManager(ctx context.Context, locationID string, userID string) (bool, error)
	ListLocationManagers(ctx context.Context, locationID string) ([]string, error)
	GetOrgIDByLocationID(ctx context.Context, locationID string) (string, error)
	IsLocationManagerInOrg(ctx context.Context, orgID string, userID string) (bool, error)
	// Helpers
	RemoveAllLocationManagersForUser(ctx context.Context, userID string) error
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
		Columns("name", "is_active").
		Values(org.Name, org.IsActive).
		Suffix("RETURNING id, created_at").
		ToSql()
	if err != nil {
		return fmt.Errorf("build create organization query failed: %w", err)
	}

	// Default is_active to true if not handled by caller,
	// though DB default is also true.
	return r.pool.QueryRow(ctx, query, args...).
		Scan(&org.ID, &org.CreatedAt)
}

func (r *pgxRepository) GetByID(ctx context.Context, id string) (*Organization, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select("id", "name", "created_at", "is_active").
		From("public.organizations").
		Where(squirrel.Eq{"id": id}).
		Where(squirrel.Eq{"is_active": true}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get organization query failed: %w", err)
	}

	row := r.pool.QueryRow(ctx, query, args...)

	var org Organization
	if err := row.Scan(&org.ID, &org.Name, &org.CreatedAt, &org.IsActive); err != nil {
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
	queryBuilder := psql.Select("id", "name", "created_at", "is_active", "count(*) OVER() AS total_count").
		From("public.organizations").
		Where(squirrel.Eq{"is_active": true})

	orderBy := "id"
	if filter.SortBy != "" {
		orderBy = filter.SortBy
	}

	orderDir := "DESC"
	if filter.SortOrder != "" {
		orderDir = filter.SortOrder
	}

	queryBuilder = queryBuilder.OrderBy(orderBy + " " + orderDir)

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
		return nil, 0, fmt.Errorf("build list organizations query failed: %w", err)
	}

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("List failed: %w", err)
	}
	defer rows.Close()

	var orgs []*Organization
	var total int

	for rows.Next() {
		var o Organization
		if err := rows.Scan(&o.ID, &o.Name, &o.CreatedAt, &o.IsActive, &total); err != nil {
			return nil, 0, fmt.Errorf("scan failed: %w", err)
		}
		orgs = append(orgs, &o)
	}

	return orgs, total, nil
}

func (r *pgxRepository) Update(ctx context.Context, org *Organization) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Update("public.organizations").
		Set("name", org.Name).
		Set("is_active", org.IsActive).
		Where(squirrel.Eq{"id": org.ID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build update organization query failed: %w", err)
	}

	ct, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("Update failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrOrgNotFound
	}
	return nil
}

func (r *pgxRepository) Delete(ctx context.Context, id string) error {
	// Soft delete implementation
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

// ------------------------
//     Member methods
// ------------------------

// GetMember retrieves a member's details from organization_permissions.
// Returns ErrNotFound if the user is not a member of the organization.
func (r *pgxRepository) GetMember(ctx context.Context, orgID string, userID string) (*Member, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select(
		"u.id", "u.email", "u.display_name", "op.role",
	).
		From("public.organization_permissions op").
		Join("public.users u ON op.user_id = u.id").
		Where(squirrel.Eq{"op.organization_id": orgID}).
		Where(squirrel.Eq{"op.user_id": userID}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get member query failed: %w", err)
	}

	row := r.pool.QueryRow(ctx, query, args...)

	var m Member
	if err := row.Scan(&m.UserID, &m.Email, &m.DisplayName, &m.Role); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotMember // User is not a member of the organization
		}
		return nil, fmt.Errorf("GetMember failed: %w", err)
	}

	return &m, nil
}

// AddMember inserts a new record into organization_permissions.
func (r *pgxRepository) AddMember(ctx context.Context, orgID string, userID string, role string) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Insert("public.organization_permissions").
		Columns("organization_id", "user_id", "role").
		Values(orgID, userID, role).
		ToSql()
	if err != nil {
		return fmt.Errorf("build add member query failed: %w", err)
	}

	_, err = r.pool.Exec(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			// Check for unique constraint violation (already a member)
			if pgErr.Code == pgerrcode.UniqueViolation {
				return ErrUserAlreadyMember
			}
		}
		return fmt.Errorf("AddMember failed: %w", err)
	}
	return nil
}

// RemoveMember deletes a record from organization_permissions.
func (r *pgxRepository) RemoveMember(ctx context.Context, orgID string, userID string) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Delete("public.organization_permissions").
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

// UpdateMemberRole updates the role in organization_permissions.
func (r *pgxRepository) UpdateMemberRole(ctx context.Context, orgID string, userID string, role string) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Update("public.organization_permissions").
		Set("role", role).
		Where(squirrel.Eq{"organization_id": orgID}).
		Where(squirrel.Eq{"user_id": userID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build update member role query failed: %w", err)
	}

	ct, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("UpdateMemberRole failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrUserNotMember
	}
	return nil
}

// ListMembers retrieves members with their user details, including Owners, Org Admins, and Location Admins.
func (r *pgxRepository) ListMembers(ctx context.Context, orgID string, filter MemberFilter) ([]*Member, int, error) {
	// Defaults
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 20
	}
	offset := (filter.Page - 1) * filter.PageSize

	// Sort validation
	orderBy := "id"
	switch filter.SortBy {
	case "role":
		orderBy = "role"
	case "email":
		orderBy = "email"
	case "display_name":
		orderBy = "display_name"
	}

	orderDir := "DESC"
	if filter.SortOrder == "ASC" || filter.SortOrder == "asc" {
		orderDir = "ASC"
	}

	// Complex query with CTEs for Unified List and Total Count
	query := fmt.Sprintf(`
	WITH unified_members AS (
		SELECT u.id, u.email, u.display_name, op.role::text
		FROM public.organization_permissions op
		JOIN public.users u ON op.user_id = u.id
		WHERE op.organization_id = $1

		UNION

		SELECT u.id, u.email, u.display_name, 'location_manager' as role
		FROM public.location_admins la
		JOIN public.locations l ON la.location_id = l.id
		JOIN public.users u ON la.user_id = u.id
		WHERE l.organization_id = $1
	),
	total_count AS (
		SELECT count(*) as total FROM unified_members
	)
	SELECT id, email, display_name, role, (SELECT total FROM total_count)
	FROM unified_members
	ORDER BY "%s" %s
	LIMIT $2 OFFSET $3
	`, orderBy, orderDir)

	rows, err := r.pool.Query(ctx, query, orgID, filter.PageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("ListMembers failed: %w", err)
	}
	defer rows.Close()

	var members []*Member
	var total int

	for rows.Next() {
		var m Member
		// Scan total from the window function (simulated via subquery selection)
		if err := rows.Scan(&m.UserID, &m.Email, &m.DisplayName, &m.Role, &total); err != nil {
			return nil, 0, fmt.Errorf("scan member failed: %w", err)
		}
		members = append(members, &m)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows iteration failed: %w", err)
	}

	return members, total, nil
}

// ------------------------
//   Location Manager methods
// ------------------------

func (r *pgxRepository) AddLocationManager(ctx context.Context, locationID string, userID string) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Insert("public.location_admins").
		Columns("location_id", "user_id").
		Values(locationID, userID).
		Suffix("ON CONFLICT DO NOTHING").
		ToSql()
	if err != nil {
		return fmt.Errorf("build add location admin query failed: %w", err)
	}

	_, err = r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("AddLocationManager failed: %w", err)
	}
	return nil
}

func (r *pgxRepository) RemoveLocationManager(ctx context.Context, locationID string, userID string) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Delete("public.location_admins").
		Where(squirrel.Eq{"location_id": locationID}).
		Where(squirrel.Eq{"user_id": userID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build remove location admin query failed: %w", err)
	}

	_, err = r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("RemoveLocationManager failed: %w", err)
	}
	return nil
}

func (r *pgxRepository) IsLocationManager(ctx context.Context, locationID string, userID string) (bool, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select("1").
		From("public.location_admins").
		Where(squirrel.Eq{"location_id": locationID}).
		Where(squirrel.Eq{"user_id": userID}).
		ToSql()
	if err != nil {
		return false, fmt.Errorf("build check location admin query failed: %w", err)
	}

	var one int
	err = r.pool.QueryRow(ctx, query, args...).Scan(&one)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("IsLocationManager failed: %w", err)
	}
	return true, nil
}

func (r *pgxRepository) ListLocationManagers(ctx context.Context, locationID string) ([]string, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select("user_id").
		From("public.location_admins").
		Where(squirrel.Eq{"location_id": locationID}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build list location admins query failed: %w", err)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("ListLocationManagers failed: %w", err)
	}
	defer rows.Close()

	var userIDs []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}
		userIDs = append(userIDs, uid)
	}
	return userIDs, nil
}

func (r *pgxRepository) GetOrgIDByLocationID(ctx context.Context, locationID string) (string, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select("organization_id").
		From("public.locations").
		Where(squirrel.Eq{"id": locationID}).
		ToSql()
	if err != nil {
		return "", fmt.Errorf("build get org id by location id query failed: %w", err)
	}

	var orgID string
	err = r.pool.QueryRow(ctx, query, args...).Scan(&orgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", errors.New("location not found")
		}
		return "", fmt.Errorf("GetOrgIDByLocationID failed: %w", err)
	}
	return orgID, nil
}

func (r *pgxRepository) IsLocationManagerInOrg(ctx context.Context, orgID string, userID string) (bool, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select("1").
		From("public.location_admins la").
		Join("public.locations l ON la.location_id = l.id").
		Where(squirrel.Eq{"l.organization_id": orgID}).
		Where(squirrel.Eq{"la.user_id": userID}).
		ToSql()
	if err != nil {
		return false, fmt.Errorf("build check is location manager in org query failed: %w", err)
	}

	var one int
	err = r.pool.QueryRow(ctx, query, args...).Scan(&one)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("IsLocationManagerInOrg failed: %w", err)
	}
	return true, nil
}

func (r *pgxRepository) RemoveAllLocationManagersForUser(ctx context.Context, userID string) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Delete("public.location_admins").
		Where(squirrel.Eq{"user_id": userID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build remove all location admins query failed: %w", err)
	}

	_, err = r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("RemoveAllLocationManagersForUser failed: %w", err)
	}
	return nil
}
