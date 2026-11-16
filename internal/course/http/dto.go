package http

import (
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/course"
	"github.com/nekogravitycat/court-booking-backend/internal/file"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
)

// ---- Requests ----

type ScheduleRequest struct {
	StartDate string   `json:"start_date" binding:"required"`
	EndDate   string   `json:"end_date" binding:"required"`
	Weekdays  []int    `json:"weekdays" binding:"required,min=1,max=7,dive,min=0,max=6"`
	StartTime string   `json:"start_time" binding:"required"`
	EndTime   string   `json:"end_time" binding:"required"`
	SkipDates []string `json:"skip_dates" binding:"omitempty,max=100"`
}

type SessionRangeRequest struct {
	StartAt time.Time `json:"start_at" binding:"required"`
	EndAt   time.Time `json:"end_at" binding:"required"`
}

// CreateCourseRequest is the body of POST /courses. Provide exactly one of
// schedule (a weekly pattern) or sessions (an explicit list).
type CreateCourseRequest struct {
	GroupID             *string               `json:"group_id" binding:"omitempty,uuid"`
	CoachIDs            []string              `json:"coach_ids" binding:"omitempty,max=10,dive,uuid"`
	Title               string                `json:"title" binding:"required"`
	Description         string                `json:"description"`
	TeachingContent     string                `json:"teaching_content"`
	SportID             string                `json:"sport_id" binding:"required,uuid"`
	CourseType          string                `json:"course_type" binding:"required,oneof=trial single recurring"`
	Capacity            int                   `json:"capacity" binding:"required,min=1,max=500"`
	VenueName           string                `json:"venue_name"`
	LocationID          *string               `json:"location_id" binding:"omitempty,uuid"`
	Timezone            string                `json:"timezone"`
	Price               int                   `json:"price" binding:"min=0,max=1000000"`
	VenueFee            int                   `json:"venue_fee" binding:"min=0,max=1000000"`
	PaymentMethods      []string              `json:"payment_methods" binding:"omitempty,max=2,dive,oneof=onsite transfer"`
	BankCode            *string               `json:"bank_code"`
	BankAccount         *string               `json:"bank_account"`
	BankAccountName     *string               `json:"bank_account_name"`
	AllowLeave          bool                  `json:"allow_leave"`
	LeaveDeadlineHours  *int                  `json:"leave_deadline_hours"`
	AllowMakeup         bool                  `json:"allow_makeup"`
	MakeupLimit         *int                  `json:"makeup_limit"`
	MakeupValidDays     *int                  `json:"makeup_valid_days"`
	AllowCancel         bool                  `json:"allow_cancel"`
	CancelDeadlineHours *int                  `json:"cancel_deadline_hours"`
	RulesLeave          string                `json:"rules_leave"`
	RulesMakeup         string                `json:"rules_makeup"`
	RulesCancellation   string                `json:"rules_cancellation"`
	RulesRefund         string                `json:"rules_refund"`
	RulesNoShow         string                `json:"rules_noshow"`
	EnrollmentDeadline  *time.Time            `json:"enrollment_deadline"`
	Schedule            *ScheduleRequest      `json:"schedule"`
	Sessions            []SessionRangeRequest `json:"sessions" binding:"omitempty,max=100,dive"`
}

func (r CreateCourseRequest) ToInput() course.CreateInput {
	c := course.Course{
		Title: r.Title, Description: r.Description, TeachingContent: r.TeachingContent,
		SportID: r.SportID, Type: r.CourseType, Capacity: r.Capacity, VenueName: r.VenueName,
		LocationID: r.LocationID, Timezone: r.Timezone, Price: r.Price, VenueFee: r.VenueFee,
		PaymentMethods: r.PaymentMethods, BankCode: r.BankCode, BankAccount: r.BankAccount, BankAccountName: r.BankAccountName,
		AllowLeave: r.AllowLeave, LeaveDeadlineHrs: r.LeaveDeadlineHours, AllowMakeup: r.AllowMakeup,
		MakeupLimit: r.MakeupLimit, MakeupValidDays: r.MakeupValidDays, AllowCancel: r.AllowCancel,
		CancelDeadline: r.CancelDeadlineHours, RulesLeave: r.RulesLeave, RulesMakeup: r.RulesMakeup,
		RulesCancel: r.RulesCancellation, RulesRefund: r.RulesRefund, RulesNoShow: r.RulesNoShow,
		EnrollDeadline: r.EnrollmentDeadline,
	}
	in := course.CreateInput{Course: c, GroupID: r.GroupID, CoachIDs: r.CoachIDs}
	if r.Schedule != nil {
		in.Schedule = &course.Schedule{
			StartDate: r.Schedule.StartDate, EndDate: r.Schedule.EndDate, Weekdays: r.Schedule.Weekdays,
			StartTime: r.Schedule.StartTime, EndTime: r.Schedule.EndTime, SkipDates: r.Schedule.SkipDates,
		}
	}
	for _, s := range r.Sessions {
		in.Sessions = append(in.Sessions, course.TimeRange{Start: s.StartAt, End: s.EndAt})
	}
	return in
}

// UpdateCourseRequest is the body of PATCH /courses/{id}. Nullable fields take
// null to clear. Setting status to "cancelled" must not be combined with other fields.
type UpdateCourseRequest struct {
	Title               *string                     `json:"title"`
	Description         *string                     `json:"description"`
	TeachingContent     *string                     `json:"teaching_content"`
	Capacity            *int                        `json:"capacity" binding:"omitempty,min=1,max=500"`
	VenueName           *string                     `json:"venue_name"`
	LocationID          request.Nullable[string]    `json:"location_id"`
	Price               *int                        `json:"price" binding:"omitempty,min=0,max=1000000"`
	VenueFee            *int                        `json:"venue_fee" binding:"omitempty,min=0,max=1000000"`
	PaymentMethods      *[]string                   `json:"payment_methods" binding:"omitempty,max=2,dive,oneof=onsite transfer"`
	BankCode            *string                     `json:"bank_code"`
	BankAccount         *string                     `json:"bank_account"`
	BankAccountName     *string                     `json:"bank_account_name"`
	AllowLeave          *bool                       `json:"allow_leave"`
	LeaveDeadlineHours  request.Nullable[int]       `json:"leave_deadline_hours"`
	AllowMakeup         *bool                       `json:"allow_makeup"`
	MakeupLimit         request.Nullable[int]       `json:"makeup_limit"`
	MakeupValidDays     request.Nullable[int]       `json:"makeup_valid_days"`
	AllowCancel         *bool                       `json:"allow_cancel"`
	CancelDeadlineHours request.Nullable[int]       `json:"cancel_deadline_hours"`
	RulesLeave          *string                     `json:"rules_leave"`
	RulesMakeup         *string                     `json:"rules_makeup"`
	RulesCancellation   *string                     `json:"rules_cancellation"`
	RulesRefund         *string                     `json:"rules_refund"`
	RulesNoShow         *string                     `json:"rules_noshow"`
	EnrollmentDeadline  request.Nullable[time.Time] `json:"enrollment_deadline"`
	Status              *string                     `json:"status" binding:"omitempty,oneof=open closed cancelled"`
	CoachIDs            *[]string                   `json:"coach_ids" binding:"omitempty,max=10,dive,uuid"`
}

func (r UpdateCourseRequest) ToInput() course.UpdateInput {
	in := course.UpdateInput{
		Title: r.Title, Description: r.Description, TeachingContent: r.TeachingContent, Capacity: r.Capacity,
		VenueName: r.VenueName, Price: r.Price, VenueFee: r.VenueFee, PaymentMethods: r.PaymentMethods,
		BankCode: r.BankCode, BankAccount: r.BankAccount, BankAccountName: r.BankAccountName,
		AllowLeave: r.AllowLeave, AllowMakeup: r.AllowMakeup, AllowCancel: r.AllowCancel,
		RulesLeave: r.RulesLeave, RulesMakeup: r.RulesMakeup, RulesCancel: r.RulesCancellation,
		RulesRefund: r.RulesRefund, RulesNoShow: r.RulesNoShow, Status: r.Status, CoachIDs: r.CoachIDs,
		LeaveDeadlineHrs: optInt(r.LeaveDeadlineHours), MakeupLimit: optInt(r.MakeupLimit),
		MakeupValidDays: optInt(r.MakeupValidDays), CancelDeadline: optInt(r.CancelDeadlineHours),
		EnrollDeadline: course.OptTime{Set: r.EnrollmentDeadline.Set, Value: r.EnrollmentDeadline.Value},
	}
	if r.LocationID.Set {
		if r.LocationID.Value == nil {
			in.ClearLocation = true
		} else {
			in.LocationID = r.LocationID.Value
		}
	}
	return in
}

func optInt(n request.Nullable[int]) course.OptInt { return course.OptInt{Set: n.Set, Value: n.Value} }

type AddSessionRequest struct {
	StartAt   time.Time `json:"start_at" binding:"required"`
	EndAt     time.Time `json:"end_at" binding:"required"`
	VenueNote string    `json:"venue_note"`
}

// UpdateSessionRequest reschedules (start_at and end_at together), edits the
// venue note, or cancels (cancel=true with an optional reason) one session.
type UpdateSessionRequest struct {
	StartAt   *time.Time `json:"start_at"`
	EndAt     *time.Time `json:"end_at"`
	VenueNote *string    `json:"venue_note"`
	Cancel    bool       `json:"cancel"`
	Reason    string     `json:"reason"`
}

type EnrollRequest struct {
	PaymentMethod *string `json:"payment_method" binding:"omitempty,oneof=onsite transfer"`
}

type ListCoursesRequest struct {
	request.ListParams
	SportID      string     `form:"sport_id" binding:"omitempty,uuid"`
	CourseType   string     `form:"course_type" binding:"omitempty,oneof=trial single recurring"`
	CoachID      string     `form:"coach_id" binding:"omitempty,uuid"`
	GroupID      string     `form:"group_id" binding:"omitempty,uuid"`
	LocationID   string     `form:"location_id" binding:"omitempty,uuid"`
	Keyword      string     `form:"keyword" binding:"omitempty,max=50"`
	Status       string     `form:"status" binding:"omitempty,oneof=open closed cancelled"`
	FreeOnly     bool       `form:"free_only"`
	HasSeats     bool       `form:"has_seats"`
	IncludeEnded bool       `form:"include_ended"`
	StartFrom    *time.Time `form:"start_from" time_format:"2006-01-02T15:04:05Z07:00"`
	StartTo      *time.Time `form:"start_to" time_format:"2006-01-02T15:04:05Z07:00"`
	Managed      bool       `form:"managed"`
	Enrolled     bool       `form:"enrolled"`
	SortBy       string     `form:"sort_by" binding:"omitempty,oneof=next_session created_at price"`
}

type ListEnrollmentsRequest struct {
	request.ListParams
	Status        string `form:"status" binding:"omitempty,oneof=active cancelled"`
	PaymentStatus string `form:"payment_status" binding:"omitempty,oneof=not_required unpaid reported paid refunded"`
}

type SessionURI struct {
	CourseID  string `uri:"id" binding:"required,uuid"`
	SessionID string `uri:"session_id" binding:"required,uuid"`
}

// ---- Responses ----

type TeacherResponse struct {
	UserID          string  `json:"user_id"`
	DisplayName     string  `json:"display_name"`
	Avatar          *string `json:"avatar"`
	AvatarThumbnail *string `json:"avatar_thumbnail"`
}

type SessionResponse struct {
	ID           string    `json:"id"`
	Seq          int       `json:"seq"`
	StartAt      time.Time `json:"start_at"`
	EndAt        time.Time `json:"end_at"`
	Status       string    `json:"status"`
	VenueNote    string    `json:"venue_note"`
	CancelReason *string   `json:"cancel_reason"`
}

func NewSessionResponse(s *course.Session) SessionResponse {
	return SessionResponse{
		ID: s.ID, Seq: s.Seq, StartAt: s.StartAt.UTC(), EndAt: s.EndAt.UTC(), Status: s.Status,
		VenueNote: s.VenueNote, CancelReason: s.CancelReason,
	}
}

type CountResponse struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

type LevelCountResponse struct {
	Level int    `json:"level"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

// ParticipantStatsResponse is the anonymous roster summary (no names).
type ParticipantStatsResponse struct {
	Total       int                  `json:"total"`
	Genders     []CountResponse      `json:"genders"`
	AgeGroups   []CountResponse      `json:"age_groups"`
	SkillLevels []LevelCountResponse `json:"skill_levels"`
}

type CourseResponse struct {
	ID                  string                    `json:"id"`
	OwnerType           string                    `json:"owner_type"`
	CoachID             *string                   `json:"coach_id"`
	GroupID             *string                   `json:"group_id"`
	OwnerName           string                    `json:"owner_name"`
	Title               string                    `json:"title"`
	Description         string                    `json:"description"`
	TeachingContent     string                    `json:"teaching_content"`
	SportID             string                    `json:"sport_id"`
	SportName           string                    `json:"sport_name"`
	CourseType          string                    `json:"course_type"`
	Capacity            int                       `json:"capacity"`
	EnrolledCount       int                       `json:"enrolled_count"`
	RemainingSeats      int                       `json:"remaining_seats"`
	VenueName           string                    `json:"venue_name"`
	LocationID          *string                   `json:"location_id"`
	Timezone            string                    `json:"timezone"`
	IsFree              bool                      `json:"is_free"`
	Price               int                       `json:"price"`
	VenueFee            int                       `json:"venue_fee"`
	TotalFee            int                       `json:"total_fee"`
	PaymentMethods      []string                  `json:"payment_methods"`
	BankCode            *string                   `json:"bank_code"`
	BankAccount         *string                   `json:"bank_account"`
	BankAccountName     *string                   `json:"bank_account_name"`
	BankInfoAvailable   bool                      `json:"bank_info_available"`
	AllowLeave          bool                      `json:"allow_leave"`
	LeaveDeadlineHours  *int                      `json:"leave_deadline_hours"`
	AllowMakeup         bool                      `json:"allow_makeup"`
	MakeupLimit         *int                      `json:"makeup_limit"`
	MakeupValidDays     *int                      `json:"makeup_valid_days"`
	AllowCancel         bool                      `json:"allow_cancel"`
	CancelDeadlineHours *int                      `json:"cancel_deadline_hours"`
	RulesLeave          string                    `json:"rules_leave"`
	RulesMakeup         string                    `json:"rules_makeup"`
	RulesCancellation   string                    `json:"rules_cancellation"`
	RulesRefund         string                    `json:"rules_refund"`
	RulesNoShow         string                    `json:"rules_noshow"`
	EnrollmentDeadline  *time.Time                `json:"enrollment_deadline"`
	Status              string                    `json:"status"`
	Teachers            []TeacherResponse         `json:"teachers"`
	SessionCount        int                       `json:"session_count"`
	FirstSessionAt      *time.Time                `json:"first_session_at"`
	LastSessionAt       *time.Time                `json:"last_session_at"`
	NextSessionAt       *time.Time                `json:"next_session_at"`
	Sessions            []SessionResponse         `json:"sessions,omitempty"`
	CanManage           bool                      `json:"can_manage,omitempty"`
	MyEnrollmentID      *string                   `json:"my_enrollment_id,omitempty"`
	ParticipantStats    *ParticipantStatsResponse `json:"participant_stats,omitempty"`
	CreatedAt           time.Time                 `json:"created_at"`
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func NewCourseResponse(c *course.Course) CourseResponse {
	owner := "coach"
	if c.GroupID != nil {
		owner = "group"
	}
	remaining := max(c.Capacity-c.EnrolledCount, 0)
	teachers := make([]TeacherResponse, len(c.Teachers))
	for i, t := range c.Teachers {
		avatar, thumb := file.URLs(t.Avatar)
		teachers[i] = TeacherResponse{UserID: t.UserID, DisplayName: t.DisplayName, Avatar: avatar, AvatarThumbnail: thumb}
	}
	resp := CourseResponse{
		ID: c.ID, OwnerType: owner, CoachID: c.CoachID, GroupID: c.GroupID, OwnerName: c.OwnerName,
		Title: c.Title, Description: c.Description, TeachingContent: c.TeachingContent,
		SportID: c.SportID, SportName: c.SportName, CourseType: c.Type, Capacity: c.Capacity,
		EnrolledCount: c.EnrolledCount, RemainingSeats: remaining, VenueName: c.VenueName, LocationID: c.LocationID,
		Timezone: c.Timezone, IsFree: c.IsFree(), Price: c.Price, VenueFee: c.VenueFee, TotalFee: c.Price + c.VenueFee,
		PaymentMethods: c.PaymentMethods, BankCode: c.BankCode, BankAccount: c.BankAccount, BankAccountName: c.BankAccountName,
		BankInfoAvailable: c.BankInfoAvailable,
		AllowLeave:        c.AllowLeave, LeaveDeadlineHours: c.LeaveDeadlineHrs, AllowMakeup: c.AllowMakeup,
		MakeupLimit: c.MakeupLimit, MakeupValidDays: c.MakeupValidDays, AllowCancel: c.AllowCancel,
		CancelDeadlineHours: c.CancelDeadline,
		RulesLeave:          c.RulesLeave, RulesMakeup: c.RulesMakeup, RulesCancellation: c.RulesCancel,
		RulesRefund: c.RulesRefund, RulesNoShow: c.RulesNoShow,
		EnrollmentDeadline: utcPtr(c.EnrollDeadline), Status: c.Status, Teachers: teachers, SessionCount: c.SessionCount,
		FirstSessionAt: utcPtr(c.FirstSessionAt), LastSessionAt: utcPtr(c.LastSessionAt), NextSessionAt: utcPtr(c.NextSessionAt),
		CanManage: c.CanManage, MyEnrollmentID: c.MyEnrollmentID, CreatedAt: c.CreatedAt.UTC(),
	}
	if resp.PaymentMethods == nil {
		resp.PaymentMethods = []string{}
	}
	for _, s := range c.Sessions {
		resp.Sessions = append(resp.Sessions, NewSessionResponse(s))
	}
	if c.ParticipantStats != nil {
		ps := c.ParticipantStats
		out := &ParticipantStatsResponse{Total: ps.Total, Genders: []CountResponse{}, AgeGroups: []CountResponse{}, SkillLevels: []LevelCountResponse{}}
		for _, g := range ps.Genders {
			out.Genders = append(out.Genders, CountResponse{Key: g.Key, Count: g.Count})
		}
		for _, a := range ps.AgeGroups {
			out.AgeGroups = append(out.AgeGroups, CountResponse{Key: a.Key, Count: a.Count})
		}
		for _, l := range ps.SkillLevels {
			out.SkillLevels = append(out.SkillLevels, LevelCountResponse{Level: l.Level, Label: l.Label, Count: l.Count})
		}
		resp.ParticipantStats = out
	}
	return resp
}

type CourseBriefResponse struct {
	ID            string     `json:"id"`
	Title         string     `json:"title"`
	CourseType    string     `json:"course_type"`
	Status        string     `json:"status"`
	SportName     string     `json:"sport_name"`
	OwnerName     string     `json:"owner_name"`
	NextSessionAt *time.Time `json:"next_session_at"`
}

type StudentResponse struct {
	UserID          string  `json:"user_id"`
	DisplayName     string  `json:"display_name"`
	Avatar          *string `json:"avatar"`
	AvatarThumbnail *string `json:"avatar_thumbnail"`
	Phone           *string `json:"phone"`
	LineID          *string `json:"line_id"`
	SkillLevel      *int    `json:"skill_level"`
	SkillLabel      *string `json:"skill_label"`
}

type EnrollmentResponse struct {
	ID            string                    `json:"id"`
	CourseID      string                    `json:"course_id"`
	UserID        string                    `json:"user_id"`
	Status        string                    `json:"status"`
	PaymentStatus string                    `json:"payment_status"`
	PaymentMethod *string                   `json:"payment_method"`
	Price         int                       `json:"price"`
	VenueFee      int                       `json:"venue_fee"`
	AmountDue     int                       `json:"amount_due"`
	CancelSource  *string                   `json:"cancel_source"`
	CancelledAt   *time.Time                `json:"cancelled_at"`
	CreatedAt     time.Time                 `json:"created_at"`
	Course        CourseBriefResponse       `json:"course"`
	Student       StudentResponse           `json:"student"`
	Attendance    AttendanceSummaryResponse `json:"attendance"`
}

func NewEnrollmentResponse(e *course.Enrollment) EnrollmentResponse {
	resp := EnrollmentResponse{
		ID: e.ID, CourseID: e.CourseID, UserID: e.UserID, Status: e.Status, PaymentStatus: e.PaymentStatus,
		PaymentMethod: e.PaymentMethod, Price: e.Price, VenueFee: e.VenueFee, AmountDue: e.AmountDue,
		CancelSource: e.CancelSource, CancelledAt: utcPtr(e.CancelledAt), CreatedAt: e.CreatedAt.UTC(),
		Attendance: AttendanceSummaryResponse{Present: e.Attendance.Present, Absent: e.Attendance.Absent, Leave: e.Attendance.Leave},
	}
	if e.Course != nil {
		resp.Course = CourseBriefResponse{
			ID: e.Course.ID, Title: e.Course.Title, CourseType: e.Course.Type, Status: e.Course.Status,
			SportName: e.Course.SportName, OwnerName: e.Course.OwnerName, NextSessionAt: utcPtr(e.Course.NextSessionAt),
		}
	}
	if e.Student != nil {
		avatar, thumb := file.URLs(e.Student.Avatar)
		resp.Student = StudentResponse{
			UserID: e.Student.UserID, DisplayName: e.Student.DisplayName, Avatar: avatar, AvatarThumbnail: thumb,
			Phone: e.Student.Phone, LineID: e.Student.LineID, SkillLevel: e.Student.SkillLevel, SkillLabel: e.Student.SkillLabel,
		}
	}
	return resp
}
