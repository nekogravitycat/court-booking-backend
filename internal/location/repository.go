package location

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

// Repository defines data access methods for locations.
type Repository interface {
	Create(ctx context.Context, loc *Location) error
	GetByID(ctx context.Context, id string) (*Location, error)
	List(ctx context.Context, filter LocationFilter) ([]*Location, int, error)
	Update(ctx context.Context, loc *Location) error
	Delete(ctx context.Context, id string) error
	// Manager methods
	AddLocationManager(ctx context.Context, locationID string, userID string) error
	RemoveLocationManager(ctx context.Context, locationID string, userID string) error
	IsLocationManager(ctx context.Context, locationID string, userID string) (bool, error)
	ListLocationManagers(ctx context.Context, locationID string, params request.ListParams) ([]*user.User, int, error)
	IsLocationManagerInOrg(ctx context.Context, orgID string, userID string) (bool, error)
	// Utility methods
	GetOrganizationID(ctx context.Context, locationID string) (string, error)
}

type pgxRepository struct {
	pool *pgxpool.Pool
}

func NewPgxRepository(pool *pgxpool.Pool) Repository {
	return &pgxRepository{pool: pool}
}

func (r *pgxRepository) Create(ctx context.Context, loc *Location) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Insert("public.locations").
		Columns(
			"organization_id", "name", "capacity", "opening_hours_start", "opening_hours_end",
			"location_info", "opening", "rule", "facility", "description", "longitude", "latitude",
		).
		Values(
			loc.OrganizationID, loc.Name, loc.Capacity, loc.OpeningHoursStart, loc.OpeningHoursEnd,
			loc.LocationInfo, loc.Opening, loc.Rule, loc.Facility, loc.Description, loc.Longitude, loc.Latitude,
		).
		Suffix("RETURNING id, created_at").
		ToSql()
	if err != nil {
		return fmt.Errorf("build create location query failed: %w", err)
	}

	// Note: Postgres handles casting string "HH:MM:SS" to TIME automatically in most cases.
	err = r.pool.QueryRow(ctx, query, args...).Scan(&loc.ID, &loc.CreatedAt)

	if err != nil {
		return fmt.Errorf("create location failed: %w", err)
	}
	return nil
}

func (r *pgxRepository) GetByID(ctx context.Context, id string) (*Location, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select(
		"l.id", "l.organization_id", "o.name", "l.name", "l.created_at", "l.capacity",
		"l.opening_hours_start::text", "l.opening_hours_end::text",
		"l.location_info", "l.opening", "l.rule", "l.facility", "l.description", "l.longitude", "l.latitude",
	).
		From("public.locations l").
		Join("public.organizations o ON l.organization_id = o.id").
		Where(squirrel.Eq{"l.id": id}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get location query failed: %w", err)
	}

	// We cast TIME to ::text to scan into string easily.

	row := r.pool.QueryRow(ctx, query, args...)

	var l Location
	err = row.Scan(
		&l.ID, &l.OrganizationID, &l.OrganizationName, &l.Name, &l.CreatedAt, &l.Capacity,
		&l.OpeningHoursStart, &l.OpeningHoursEnd,
		&l.LocationInfo, &l.Opening, &l.Rule, &l.Facility, &l.Description, &l.Longitude, &l.Latitude,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrLocNotFound
		}
		return nil, fmt.Errorf("get location failed: %w", err)
	}
	return &l, nil
}

func (r *pgxRepository) List(ctx context.Context, filter LocationFilter) ([]*Location, int, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query := psql.Select(
		"l.id", "l.organization_id", "o.name", "l.name", "l.created_at", "l.capacity",
		"l.opening_hours_start::text", "l.opening_hours_end::text",
		"l.location_info", "l.opening", "l.rule", "l.facility", "l.description", "l.longitude", "l.latitude",
		"count(*) OVER() as total_count",
	).
		From("public.locations l").
		Join("public.organizations o ON l.organization_id = o.id")

	// Dynamic Filtering
	if filter.OrganizationID != "" {
		query = query.Where(squirrel.Eq{"l.organization_id": filter.OrganizationID})
	}
	if filter.Name != "" {
		query = query.Where(squirrel.ILike{"l.name": "%" + filter.Name + "%"})
	}
	if filter.Opening != nil {
		query = query.Where(squirrel.Eq{"l.opening": filter.Opening})
	}
	if filter.CapacityMin != nil {
		query = query.Where(squirrel.GtOrEq{"l.capacity": filter.CapacityMin})
	}
	if filter.CapacityMax != nil {
		query = query.Where(squirrel.LtOrEq{"l.capacity": filter.CapacityMax})
	}
	if filter.OpeningHoursStartMin != "" {
		query = query.Where(squirrel.GtOrEq{"l.opening_hours_start": filter.OpeningHoursStartMin})
	}
	if filter.OpeningHoursStartMax != "" {
		query = query.Where(squirrel.LtOrEq{"l.opening_hours_start": filter.OpeningHoursStartMax})
	}
	if filter.OpeningHoursEndMin != "" {
		query = query.Where(squirrel.GtOrEq{"l.opening_hours_end": filter.OpeningHoursEndMin})
	}
	if filter.OpeningHoursEndMax != "" {
		query = query.Where(squirrel.LtOrEq{"l.opening_hours_end": filter.OpeningHoursEndMax})
	}
	if !filter.CreatedAtFrom.IsZero() {
		query = query.Where(squirrel.GtOrEq{"l.created_at": filter.CreatedAtFrom})
	}
	if !filter.CreatedAtTo.IsZero() {
		query = query.Where(squirrel.LtOrEq{"l.created_at": filter.CreatedAtTo})
	}

	orderBy := "l.created_at"
	if filter.SortBy != "" {
		// Safe to prepend l. as we only allow specific fields in the handler validation
		orderBy = "l." + filter.SortBy
	}

	orderDir := "DESC"
	if filter.SortOrder == "ASC" {
		orderDir = "ASC"
	}

	query = query.OrderBy(orderBy + " " + orderDir)

	// Pagination
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 20
	}
	offset := (filter.Page - 1) * filter.PageSize

	query = query.Limit(uint64(filter.PageSize)).Offset(uint64(offset))

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("build list locations query failed: %w", err)
	}

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list locations failed: %w", err)
	}
	defer rows.Close()

	var locations []*Location
	var total int

	for rows.Next() {
		var l Location
		if err := rows.Scan(
			&l.ID, &l.OrganizationID, &l.OrganizationName, &l.Name, &l.CreatedAt, &l.Capacity,
			&l.OpeningHoursStart, &l.OpeningHoursEnd,
			&l.LocationInfo, &l.Opening, &l.Rule, &l.Facility, &l.Description, &l.Longitude, &l.Latitude,
			&total,
		); err != nil {
			return nil, 0, fmt.Errorf("scan location failed: %w", err)
		}
		locations = append(locations, &l)
	}

	return locations, total, nil
}

func (r *pgxRepository) Update(ctx context.Context, loc *Location) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Update("public.locations").
		Set("name", loc.Name).
		Set("capacity", loc.Capacity).
		Set("opening_hours_start", loc.OpeningHoursStart).
		Set("opening_hours_end", loc.OpeningHoursEnd).
		Set("location_info", loc.LocationInfo).
		Set("opening", loc.Opening).
		Set("rule", loc.Rule).
		Set("facility", loc.Facility).
		Set("description", loc.Description).
		Set("longitude", loc.Longitude).
		Set("latitude", loc.Latitude).
		Where(squirrel.Eq{"id": loc.ID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build update location query failed: %w", err)
	}

	ct, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update location failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrLocNotFound
	}
	return nil
}

func (r *pgxRepository) Delete(ctx context.Context, id string) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Delete("public.locations").
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build delete location query failed: %w", err)
	}

	ct, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("delete location failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrLocNotFound
	}
	return nil
}

// ------------------------
//   Location Manager methods
// ------------------------

func (r *pgxRepository) AddLocationManager(ctx context.Context, locationID string, userID string) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Insert("public.location_managers").
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
	query, args, err := psql.Delete("public.location_managers").
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
		From("public.location_managers").
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

func (r *pgxRepository) ListLocationManagers(ctx context.Context, locationID string, params request.ListParams) ([]*user.User, int, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query := psql.Select("u.id", "u.email", "u.display_name", "u.created_at", "u.is_active", "count(*) OVER() as total_count").
		From("public.location_managers lm").
		Join("public.users u ON lm.user_id = u.id").
		Where(squirrel.Eq{"lm.location_id": locationID})

	// Sorting
	orderDir := "ASC"
	if params.SortOrder == "DESC" {
		orderDir = "DESC"
	} else if params.SortOrder == "ASC" {
		orderDir = "ASC"
	}
	query = query.OrderBy("u.display_name " + orderDir)

	// Pagination
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 {
		params.PageSize = 20
	}
	offset := (params.Page - 1) * params.PageSize
	query = query.Limit(uint64(params.PageSize)).Offset(uint64(offset))

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("build list location admins query failed: %w", err)
	}

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("ListLocationManagers failed: %w", err)
	}
	defer rows.Close()

	var users []*user.User
	var total int

	for rows.Next() {
		var u user.User
		if err := rows.Scan(&u.ID, &u.Email, &u.DisplayName, &u.CreatedAt, &u.IsActive, &total); err != nil {
			return nil, 0, fmt.Errorf("scan failed: %w", err)
		}
		users = append(users, &u)
	}
	return users, total, nil
}

func (r *pgxRepository) IsLocationManagerInOrg(ctx context.Context, orgID string, userID string) (bool, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select("1").
		From("public.location_managers la").
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

// ------------------------
//     Utility methods
// ------------------------

func (r *pgxRepository) GetOrganizationID(ctx context.Context, locationID string) (string, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select("organization_id").
		From("public.locations").
		Where(squirrel.Eq{"id": locationID}).
		ToSql()
	if err != nil {
		return "", fmt.Errorf("build get organization id query failed: %w", err)
	}

	var orgID string
	err = r.pool.QueryRow(ctx, query, args...).Scan(&orgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrLocNotFound
		}
		return "", fmt.Errorf("GetOrganizationID failed: %w", err)
	}
	return orgID, nil
}
