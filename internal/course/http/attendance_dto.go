package http

import (
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/course"
	"github.com/nekogravitycat/court-booking-backend/internal/file"
)

type LeaveRequestBody struct {
	Reason string `json:"reason" binding:"max=200"`
}

type MakeupSelectionBody struct {
	SessionID string `json:"session_id" binding:"required,uuid"`
}

type AttendanceRecordBody struct {
	EnrollmentID string `json:"enrollment_id" binding:"required,uuid"`
	Status       string `json:"status" binding:"required,oneof=present absent pending"`
}

// MarkAttendanceRequest is the body of PATCH /sessions/{id}/attendance.
type MarkAttendanceRequest struct {
	Records []AttendanceRecordBody `json:"records" binding:"required,min=1,max=200,dive"`
}

type MakeupSessionRef struct {
	SessionID   string     `json:"session_id"`
	CourseTitle *string    `json:"course_title"`
	StartAt     *time.Time `json:"start_at"`
}

type LeaveResponse struct {
	ID             string            `json:"id"`
	SessionID      string            `json:"session_id"`
	EnrollmentID   string            `json:"enrollment_id"`
	Student        PersonRef         `json:"student"`
	CourseID       string            `json:"course_id"`
	CourseTitle    string            `json:"course_title"`
	SessionStartAt time.Time         `json:"session_start_at"`
	SessionEndAt   time.Time         `json:"session_end_at"`
	Reason         string            `json:"reason"`
	Status         string            `json:"status"`
	Makeup         *MakeupSessionRef `json:"makeup"`
	CreatedAt      time.Time         `json:"created_at"`
}

func NewLeaveResponse(l *course.LeaveRequest) LeaveResponse {
	resp := LeaveResponse{
		ID: l.ID, SessionID: l.SessionID, EnrollmentID: l.EnrollmentID, Student: PersonRef{ID: l.StudentID, Name: l.StudentName},
		CourseID: l.CourseID, CourseTitle: l.CourseTitle, SessionStartAt: l.SessionStartAt.UTC(), SessionEndAt: l.SessionEndAt.UTC(),
		Reason: l.Reason, Status: l.Status, CreatedAt: l.CreatedAt.UTC(),
	}
	if l.MakeupSessionID != nil {
		resp.Makeup = &MakeupSessionRef{SessionID: *l.MakeupSessionID, CourseTitle: l.MakeupCourse, StartAt: utcPtr(l.MakeupStartAt)}
	}
	return resp
}

type MakeupOptionResponse struct {
	SessionID      string    `json:"session_id"`
	CourseID       string    `json:"course_id"`
	CourseTitle    string    `json:"course_title"`
	OwnerName      string    `json:"owner_name"`
	StartAt        time.Time `json:"start_at"`
	EndAt          time.Time `json:"end_at"`
	Timezone       string    `json:"timezone"`
	VenueName      string    `json:"venue_name"`
	RemainingSeats int       `json:"remaining_seats"`
}

type MakeupOptionsResponse struct {
	Eligible         bool                   `json:"eligible"`
	Reason           string                 `json:"reason,omitempty"`
	RemainingMakeups *int                   `json:"remaining_makeups"`
	ValidUntil       *time.Time             `json:"valid_until"`
	Options          []MakeupOptionResponse `json:"options"`
}

func NewMakeupOptionsResponse(el course.MakeupEligibility, opts []*course.MakeupOption) MakeupOptionsResponse {
	resp := MakeupOptionsResponse{
		Eligible: el.Eligible, Reason: el.Reason, RemainingMakeups: el.Remaining, ValidUntil: utcPtr(el.Until),
		Options: make([]MakeupOptionResponse, len(opts)),
	}
	for i, o := range opts {
		resp.Options[i] = MakeupOptionResponse{
			SessionID: o.SessionID, CourseID: o.CourseID, CourseTitle: o.CourseTitle, OwnerName: o.OwnerName,
			StartAt: o.StartAt.UTC(), EndAt: o.EndAt.UTC(), Timezone: o.Timezone, VenueName: o.VenueName, RemainingSeats: o.RemainingSeats,
		}
	}
	return resp
}

type RosterEntryResponse struct {
	EnrollmentID    string     `json:"enrollment_id"`
	UserID          string     `json:"user_id"`
	DisplayName     string     `json:"display_name"`
	Avatar          *string    `json:"avatar"`
	AvatarThumbnail *string    `json:"avatar_thumbnail"`
	Phone           *string    `json:"phone"`
	LineID          *string    `json:"line_id"`
	SkillLevel      *int       `json:"skill_level"`
	SkillLabel      *string    `json:"skill_label"`
	Kind            string     `json:"kind"`
	Status          string     `json:"status"`
	MarkedAt        *time.Time `json:"marked_at"`
	FromCourse      *string    `json:"from_course"`
}

type SessionRefResponse struct {
	ID          string    `json:"id"`
	CourseID    string    `json:"course_id"`
	CourseTitle string    `json:"course_title"`
	StartAt     time.Time `json:"start_at"`
	EndAt       time.Time `json:"end_at"`
	Status      string    `json:"status"`
}

type RosterResponse struct {
	Session SessionRefResponse    `json:"session"`
	Items   []RosterEntryResponse `json:"items"`
	Total   int                   `json:"total"`
}

func NewRosterResponse(s *course.SessionInfo, entries []*course.RosterEntry) RosterResponse {
	resp := RosterResponse{
		Session: SessionRefResponse{ID: s.ID, CourseID: s.CourseID, CourseTitle: s.CourseTitle, StartAt: s.StartAt.UTC(), EndAt: s.EndAt.UTC(), Status: s.Status},
		Items:   make([]RosterEntryResponse, len(entries)),
		Total:   len(entries),
	}
	for i, e := range entries {
		avatar, thumb := file.URLs(e.Avatar)
		resp.Items[i] = RosterEntryResponse{
			EnrollmentID: e.EnrollmentID, UserID: e.UserID, DisplayName: e.DisplayName, Avatar: avatar, AvatarThumbnail: thumb,
			Phone: e.Phone, LineID: e.LineID, SkillLevel: e.SkillLevel, SkillLabel: e.SkillLabel, Kind: e.Kind, Status: e.Status,
			MarkedAt: utcPtr(e.MarkedAt), FromCourse: e.FromCourse,
		}
	}
	return resp
}

type AttendanceItemResponse struct {
	SessionID     string    `json:"session_id"`
	CourseID      string    `json:"course_id"`
	CourseTitle   string    `json:"course_title"`
	StartAt       time.Time `json:"start_at"`
	EndAt         time.Time `json:"end_at"`
	SessionStatus string    `json:"session_status"`
	Kind          string    `json:"kind"`
	Status        string    `json:"status"`
}

type AttendanceSummaryResponse struct {
	Present int `json:"present"`
	Absent  int `json:"absent"`
	Leave   int `json:"leave"`
}

type EnrollmentAttendanceResponse struct {
	Summary AttendanceSummaryResponse `json:"summary"`
	Items   []AttendanceItemResponse  `json:"items"`
	Total   int                       `json:"total"`
}

func NewEnrollmentAttendanceResponse(items []*course.AttendanceItem, sum course.AttendanceSummary) EnrollmentAttendanceResponse {
	resp := EnrollmentAttendanceResponse{
		Summary: AttendanceSummaryResponse{Present: sum.Present, Absent: sum.Absent, Leave: sum.Leave},
		Items:   make([]AttendanceItemResponse, len(items)),
		Total:   len(items),
	}
	for i, it := range items {
		resp.Items[i] = AttendanceItemResponse{
			SessionID: it.SessionID, CourseID: it.CourseID, CourseTitle: it.CourseTitle, StartAt: it.StartAt.UTC(), EndAt: it.EndAt.UTC(),
			SessionStatus: it.SessionStatus, Kind: it.Kind, Status: it.Status,
		}
	}
	return resp
}
