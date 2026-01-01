package location

import (
	"context"
	"strings"
	"time"

	"errors"

	"github.com/nekogravitycat/court-booking-backend/internal/organization"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

// CreateLocationRequest carries data to create a location.
type CreateLocationRequest struct {
	OrganizationID    string
	Name              string
	Capacity          int64
	OpeningHoursStart string
	OpeningHoursEnd   string
	LocationInfo      string
	Opening           bool
	Rule              string
	Facility          string
	Description       string
	Longitude         float64
	Latitude          float64
}

// UpdateLocationRequest carries data for partial updates.
type UpdateLocationRequest struct {
	Name              *string
	Capacity          *int64
	OpeningHoursStart *string
	OpeningHoursEnd   *string
	LocationInfo      *string
	Opening           *bool
	Rule              *string
	Facility          *string
	Description       *string
	Longitude         *float64
	Latitude          *float64
}

type Service interface {
	Create(ctx context.Context, req CreateLocationRequest) (*Location, error)
	GetByID(ctx context.Context, id string) (*Location, error)
	List(ctx context.Context, filter LocationFilter) ([]*Location, int, error)
	Update(ctx context.Context, id string, req UpdateLocationRequest) (*Location, error)
	Delete(ctx context.Context, id string) error
	// Location Manager management
	AddLocationManager(ctx context.Context, locationID string, userID string) error
	RemoveLocationManager(ctx context.Context, locationID string, userID string) error
	ListLocationManagers(ctx context.Context, locationID string) ([]string, error)
	// Permission methods
	CheckLocationPermission(ctx context.Context, orgID string, locationID string, userID string) (bool, error)
}

type service struct {
	repo        Repository
	orgService  organization.Service
	userService user.Service
}

func NewService(repo Repository, orgService organization.Service, userService user.Service) Service {
	return &service{repo: repo, orgService: orgService, userService: userService}
}

// validateLocation checks the logical rules for a Location struct.
func validateLocation(loc *Location) error {
	// 1. Validate capacity
	if loc.Capacity <= 0 {
		return ErrCapacityInvalid
	}

	// 2. Validate coordinates
	// Latitude: -90 to 90, Longitude: -180 to 180
	if loc.Latitude < -90 || loc.Latitude > 90 || loc.Longitude < -180 || loc.Longitude > 180 {
		return ErrInvalidGeo
	}

	// 3. Validate opening hours (format and logic)
	// Assumes format is HH:MM:SS or HH:MM
	layout := "15:04:05"
	t1, err1 := time.Parse(layout, loc.OpeningHoursStart)
	t2, err2 := time.Parse(layout, loc.OpeningHoursEnd)

	// Fallback: try short format if long format fails
	if err1 != nil {
		t1, err1 = time.Parse("15:04", loc.OpeningHoursStart)
	}
	if err2 != nil {
		t2, err2 = time.Parse("15:04", loc.OpeningHoursEnd)
	}

	// If format is invalid
	if err1 != nil || err2 != nil {
		return ErrInvalidOpeningHours
	}

	// End time must be after start time
	// (Logic assumes single-day operation hours)
	if t1.After(t2) || t1.Equal(t2) {
		return ErrInvalidOpeningHours
	}

	return nil
}

func (s *service) Create(ctx context.Context, req CreateLocationRequest) (*Location, error) {
	if req.OrganizationID == "" {
		return nil, ErrOrgIDRequired
	}
	if strings.TrimSpace(req.Name) == "" {
		return nil, ErrNameRequired
	}

	// Verify that the organization exists.
	if _, err := s.orgService.GetByID(ctx, req.OrganizationID); err != nil {
		return nil, ErrOrgNotFound
	}

	loc := &Location{
		OrganizationID:    req.OrganizationID,
		Name:              req.Name,
		Capacity:          req.Capacity,
		OpeningHoursStart: req.OpeningHoursStart,
		OpeningHoursEnd:   req.OpeningHoursEnd,
		LocationInfo:      req.LocationInfo,
		Opening:           req.Opening,
		Rule:              req.Rule,
		Facility:          req.Facility,
		Description:       req.Description,
		Longitude:         req.Longitude,
		Latitude:          req.Latitude,
	}

	// Validate logical rules
	if err := validateLocation(loc); err != nil {
		return nil, err
	}

	if err := s.repo.Create(ctx, loc); err != nil {
		return nil, err
	}
	return loc, nil
}

func (s *service) GetByID(ctx context.Context, id string) (*Location, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *service) List(ctx context.Context, filter LocationFilter) ([]*Location, int, error) {
	return s.repo.List(ctx, filter)
}

func (s *service) Update(ctx context.Context, id string, req UpdateLocationRequest) (*Location, error) {
	loc, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Apply non-nil fields
	if req.Name != nil {
		loc.Name = *req.Name
	}
	if req.Capacity != nil {
		loc.Capacity = *req.Capacity
	}
	if req.OpeningHoursStart != nil {
		loc.OpeningHoursStart = *req.OpeningHoursStart
	}
	if req.OpeningHoursEnd != nil {
		loc.OpeningHoursEnd = *req.OpeningHoursEnd
	}
	if req.LocationInfo != nil {
		loc.LocationInfo = *req.LocationInfo
	}
	if req.Opening != nil {
		loc.Opening = *req.Opening
	}
	if req.Rule != nil {
		loc.Rule = *req.Rule
	}
	if req.Facility != nil {
		loc.Facility = *req.Facility
	}
	if req.Description != nil {
		loc.Description = *req.Description
	}
	if req.Longitude != nil {
		loc.Longitude = *req.Longitude
	}
	if req.Latitude != nil {
		loc.Latitude = *req.Latitude
	}

	// Validate logical rules
	if err := validateLocation(loc); err != nil {
		return nil, err
	}

	if err := s.repo.Update(ctx, loc); err != nil {
		return nil, err
	}
	return loc, nil
}

func (s *service) Delete(ctx context.Context, id string) error {
	// Check existence
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

// ------------------------
//   Location Manager methods
// ------------------------

// AddLocationManager assigns a manager to a location
func (s *service) AddLocationManager(ctx context.Context, locationID string, userID string) error {
	// 1. Get Location & OrgID
	loc, err := s.repo.GetByID(ctx, locationID)
	if err != nil {
		return err
	}

	// 2. Verify User Exists
	if _, err := s.userService.GetByID(ctx, userID); err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return ErrUserNotFound
		}
		return err
	}

	// 3. Mutual Exclusion Check: User cannot be Org Admin/Owner and Location Admin
	isOrgStaff, err := s.orgService.CheckPermission(ctx, loc.OrganizationID, userID)
	if err != nil {
		return err
	}
	if isOrgStaff {
		return apperror.New(409, "user is already an organization manager or owner; cannot add as location manager")
	}

	return s.repo.AddLocationManager(ctx, locationID, userID)
}

// RemoveLocationManager removes a manager from a location
func (s *service) RemoveLocationManager(ctx context.Context, locationID string, userID string) error {
	// Verify location exists
	if _, err := s.repo.GetByID(ctx, locationID); err != nil {
		return err
	}
	return s.repo.RemoveLocationManager(ctx, locationID, userID)
}

// ListLocationManagers lists users who are managers of a location
func (s *service) ListLocationManagers(ctx context.Context, locationID string) ([]string, error) {
	// Verify location exists
	if _, err := s.repo.GetByID(ctx, locationID); err != nil {
		return nil, err
	}
	return s.repo.ListLocationManagers(ctx, locationID)
}

// CheckLocationPermission verifies if the user has permission for a specific location.
// Owner/OrgManager: Has access to all locations.
// LocationManager: Has access only if assigned to the location.
func (s *service) CheckLocationPermission(ctx context.Context, orgID string, locationID string, userID string) (bool, error) {
	if userID == "" {
		return false, nil
	}

	// 1. Check System Admin
	u, err := s.userService.GetByID(ctx, userID)
	if err != nil {
		return false, err
	}
	if u.IsSystemAdmin {
		return true, nil
	}

	// 2. Check Org Level Permission (Owner or Admin)
	// Owners and Org Admins have full access to all locations.
	isOrgStaff, err := s.orgService.CheckPermission(ctx, orgID, userID)
	if err != nil {
		return false, err
	}
	if isOrgStaff {
		return true, nil
	}

	// 3. Check Location Manager
	// If not Org Staff, check if assigned specifically to this location.
	if locationID == "" {
		return false, nil
	}
	isAdmin, err := s.repo.IsLocationManager(ctx, locationID, userID)
	if err != nil {
		return false, err
	}
	return isAdmin, nil
}
