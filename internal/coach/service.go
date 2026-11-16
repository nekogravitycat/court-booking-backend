package coach

import (
	"context"
	"errors"
	"log"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/nekogravitycat/court-booking-backend/internal/file"
	"github.com/nekogravitycat/court-booking-backend/internal/notification"
)

// Service manages coach profiles and the application workflow.
type Service interface {
	// Apply submits (or re-submits after a rejection) a coach application.
	Apply(ctx context.Context, userID string, in ProfileInput) (*Coach, error)
	// Get returns a coach profile. Only approved coaches are visible to
	// everyone; other statuses are visible to the owner and system admins.
	Get(ctx context.Context, id, viewerID string, viewerIsAdmin bool) (*Coach, error)
	GetMine(ctx context.Context, userID string) (*Coach, error)
	UpdateMine(ctx context.Context, userID string, u ProfileUpdate) (*Coach, error)
	List(ctx context.Context, filter CoachFilter) ([]*Coach, int, error)
	// ListApplications lists coaches by status for admin review (default pending).
	ListApplications(ctx context.Context, filter CoachFilter) ([]*Coach, int, error)
	Review(ctx context.Context, coachID, reviewerID string, approve bool, reason string) (*Coach, error)
	// RequireApproved returns ErrNotCoach / ErrCoachSuspended unless the user is
	// an approved coach. Other modules call it before coach-only actions.
	RequireApproved(ctx context.Context, userID string) error
}

type service struct {
	repo     Repository
	files    file.Service
	notifier notification.Service
}

func NewService(repo Repository, files file.Service, notifier notification.Service) Service {
	return &service{repo: repo, files: files, notifier: notifier}
}

// notify delivers a notification best-effort: a failed delivery must never
// undo or fail the action that triggered it.
func notify(ctx context.Context, svc notification.Service, n *notification.Notification) {
	if err := svc.Notify(ctx, n); err != nil {
		log.Printf("warning: failed to send %s notification: %v", n.Type, err)
	}
}

func ref(kind, id string) (*string, *string) { return &kind, &id }

func newNotification(userID, typ, title, content, refType, refID string) *notification.Notification {
	rt, ri := ref(refType, refID)
	return &notification.Notification{UserID: userID, Type: typ, Title: title, Content: content, RefType: rt, RefID: ri}
}

func runes(s string) int { return utf8.RuneCountInString(s) }

// cleanText trims a field and checks it against its limits.
func cleanText(s string, required bool, max int) (string, error) {
	s = strings.TrimSpace(s)
	if (required && s == "") || runes(s) > max {
		return "", ErrInvalidProfile
	}
	return s, nil
}

// dedupeSportIDs validates and de-duplicates sport IDs.
func dedupeSportIDs(ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, ErrSportsRequired
	}
	if len(ids) > MaxSports {
		return nil, ErrTooManySports
	}
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		parsed, err := uuid.Parse(id)
		if err != nil {
			return nil, ErrSportNotFound
		}
		key := parsed.String()
		if !seen[key] {
			seen[key] = true
			out = append(out, key)
		}
	}
	return out, nil
}

func (s *service) checkSports(ctx context.Context, ids []string) error {
	n, err := s.repo.CountActiveSports(ctx, ids)
	if err != nil {
		return err
	}
	if n != len(ids) {
		return ErrSportNotFound
	}
	return nil
}

// checkFile verifies the file exists, belongs to the user, and has an allowed type.
func (s *service) checkFile(ctx context.Context, userID, fileID string, allowPDF bool) error {
	if _, err := uuid.Parse(fileID); err != nil {
		return ErrInvalidFile
	}
	f, err := s.files.Get(ctx, fileID)
	if err != nil {
		if errors.Is(err, file.ErrNotFound) {
			return ErrInvalidFile
		}
		return err
	}
	if f.UserID != userID {
		return ErrInvalidFile
	}
	isImage := strings.HasPrefix(f.ContentType, "image/")
	if !isImage && !(allowPDF && f.ContentType == "application/pdf") {
		return ErrInvalidFile
	}
	return nil
}

func (s *service) checkCertificates(ctx context.Context, userID string, in []CertificateInput) ([]CertificateInput, error) {
	if len(in) > MaxCertificates {
		return nil, ErrTooManyCertificates
	}
	out := make([]CertificateInput, 0, len(in))
	seen := map[string]bool{}
	for _, c := range in {
		title, err := cleanText(c.Title, true, MaxCertTitle)
		if err != nil {
			return nil, err
		}
		if seen[c.FileID] {
			return nil, ErrInvalidFile
		}
		seen[c.FileID] = true
		if err := s.checkFile(ctx, userID, c.FileID, true); err != nil {
			return nil, err
		}
		out = append(out, CertificateInput{FileID: c.FileID, Title: title})
	}
	return out, nil
}

func (s *service) release(ctx context.Context, c Cleanup) {
	for _, id := range c.FileIDs {
		file.ReleaseReplaced(ctx, s.files, &id, "")
	}
}

func (s *service) Apply(ctx context.Context, userID string, in ProfileInput) (*Coach, error) {
	var err error
	if in.Bio, err = cleanText(in.Bio, true, MaxBio); err != nil {
		return nil, err
	}
	if in.TeachingContent, err = cleanText(in.TeachingContent, true, MaxTeachingContent); err != nil {
		return nil, err
	}
	if in.TargetAudience, err = cleanText(in.TargetAudience, true, MaxTargetAudience); err != nil {
		return nil, err
	}
	if in.Experience, err = cleanText(in.Experience, false, MaxExperience); err != nil {
		return nil, err
	}
	if in.FeeInfo, err = cleanText(in.FeeInfo, true, MaxFeeInfo); err != nil {
		return nil, err
	}
	if in.SportIDs, err = dedupeSportIDs(in.SportIDs); err != nil {
		return nil, err
	}
	if err := s.checkSports(ctx, in.SportIDs); err != nil {
		return nil, err
	}
	if in.PhotoFileID != nil {
		if err := s.checkFile(ctx, userID, *in.PhotoFileID, false); err != nil {
			return nil, err
		}
	}
	if in.Certificates, err = s.checkCertificates(ctx, userID, in.Certificates); err != nil {
		return nil, err
	}

	cleanup, err := s.repo.Apply(ctx, userID, in)
	if err != nil {
		return nil, err
	}
	s.release(ctx, cleanup)

	c, err := s.repo.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	s.notifyAdmins(ctx, c)
	return c, nil
}

func (s *service) notifyAdmins(ctx context.Context, c *Coach) {
	admins, err := s.repo.AdminIDs(ctx)
	if err != nil {
		log.Printf("warning: failed to list admins for coach application: %v", err)
		return
	}
	for _, id := range admins {
		notify(ctx, s.notifier, newNotification(id, NotifApplicationSubmitted,
			"New coach application", c.DisplayName+" applied to become a coach.", RefCoach, c.UserID))
	}
}

func (s *service) Get(ctx context.Context, id, viewerID string, viewerIsAdmin bool) (*Coach, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, ErrNotFound
	}
	c, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if c.Status != StatusApproved && !viewerIsAdmin && viewerID != id {
		return nil, ErrNotFound
	}
	return c, nil
}

func (s *service) GetMine(ctx context.Context, userID string) (*Coach, error) {
	return s.repo.Get(ctx, userID)
}

func (s *service) UpdateMine(ctx context.Context, userID string, u ProfileUpdate) (*Coach, error) {
	var err error
	set := false
	clean := func(p **string, required bool, max int) error {
		if *p == nil {
			return nil
		}
		set = true
		v, err := cleanText(**p, required, max)
		if err != nil {
			return err
		}
		*p = &v
		return nil
	}
	if err = clean(&u.Bio, true, MaxBio); err != nil {
		return nil, err
	}
	if err = clean(&u.TeachingContent, true, MaxTeachingContent); err != nil {
		return nil, err
	}
	if err = clean(&u.TargetAudience, true, MaxTargetAudience); err != nil {
		return nil, err
	}
	if err = clean(&u.Experience, false, MaxExperience); err != nil {
		return nil, err
	}
	if err = clean(&u.FeeInfo, true, MaxFeeInfo); err != nil {
		return nil, err
	}
	if u.SportIDs != nil {
		set = true
		ids, err := dedupeSportIDs(*u.SportIDs)
		if err != nil {
			return nil, err
		}
		if err := s.checkSports(ctx, ids); err != nil {
			return nil, err
		}
		u.SportIDs = &ids
	}
	if u.PhotoFileID != nil {
		set = true
		if err := s.checkFile(ctx, userID, *u.PhotoFileID, false); err != nil {
			return nil, err
		}
	}
	if u.ClearPhoto {
		set = true
	}
	if u.Certificates != nil {
		set = true
		certs, err := s.checkCertificates(ctx, userID, *u.Certificates)
		if err != nil {
			return nil, err
		}
		u.Certificates = &certs
	}
	if !set {
		return nil, ErrNothingToUpdate
	}

	cleanup, err := s.repo.Update(ctx, userID, u)
	if err != nil {
		return nil, err
	}
	s.release(ctx, cleanup)
	return s.repo.Get(ctx, userID)
}

func (s *service) List(ctx context.Context, filter CoachFilter) ([]*Coach, int, error) {
	filter.Status = StatusApproved
	return s.repo.List(ctx, filter)
}

func (s *service) ListApplications(ctx context.Context, filter CoachFilter) ([]*Coach, int, error) {
	if filter.Status == "" {
		filter.Status = StatusPending
	}
	return s.repo.List(ctx, filter)
}

func (s *service) Review(ctx context.Context, coachID, reviewerID string, approve bool, reason string) (*Coach, error) {
	reason = strings.TrimSpace(reason)
	if !approve && reason == "" {
		return nil, ErrRejectReasonNeeded
	}
	if runes(reason) > 500 {
		return nil, ErrInvalidProfile
	}
	if err := s.repo.Review(ctx, coachID, reviewerID, approve, reason); err != nil {
		return nil, err
	}
	c, err := s.repo.Get(ctx, coachID)
	if err != nil {
		return nil, err
	}
	if approve {
		notify(ctx, s.notifier, newNotification(coachID, NotifApplicationApproved,
			"Coach application approved", "Your coach application was approved. You can now create coach groups and courses.",
			RefCoach, coachID))
	} else {
		notify(ctx, s.notifier, newNotification(coachID, NotifApplicationRejected,
			"Coach application rejected", "Your coach application was rejected: "+reason+". You may update your details and apply again.",
			RefCoach, coachID))
	}
	return c, nil
}

func (s *service) RequireApproved(ctx context.Context, userID string) error {
	status, err := s.repo.GetStatus(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotCoach
		}
		return err
	}
	switch status {
	case StatusApproved:
		return nil
	case StatusSuspended:
		return ErrCoachSuspended
	default:
		return ErrNotCoach
	}
}
