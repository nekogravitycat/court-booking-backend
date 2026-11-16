package tests

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// markPresent records an attendance directly: sessions in these tests are in the future.
func markPresent(t *testing.T, enrollmentID string) {
	var sessionID string
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT s.id FROM public.course_sessions s JOIN public.enrollments e ON e.course_id = s.course_id WHERE e.id = $1 ORDER BY s.start_at LIMIT 1`,
		enrollmentID).Scan(&sessionID))
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO public.session_attendances (session_id, enrollment_id, kind, status) VALUES ($1, $2, 'regular', 'present')`, sessionID, enrollmentID)
	require.NoError(t, err)
}

func TestCoachReviews(t *testing.T) {
	clearTables()
	admin := newCoachUser(t, "admin", true)
	sportID := getSportID(t, "BADMINTON")
	coachA := makeApprovedCoach(t, "coacha", admin, sportID)
	coachB := makeApprovedCoach(t, "coachb", admin, sportID)
	student := newCoachUser(t, "student", false)
	absent := newCoachUser(t, "absent", false)
	stranger := newCoachUser(t, "stranger", false)

	courseID := createCourseOK(t, coachA.Token, courseBody(sportID, map[string]any{"capacity": 5}))["id"].(string)
	enrollID := enrollOK(t, courseID, student, "")
	absentEnroll := enrollOK(t, courseID, absent, "")
	path := "/v1/enrollments/" + enrollID + "/review"

	t.Run("only students who attended can review", func(t *testing.T) {
		assert.Equal(t, http.StatusUnauthorized, executeRequest("POST", path, map[string]any{"rating": 5}, "").Code)
		assert.Equal(t, http.StatusForbidden, executeRequest("POST", path, map[string]any{"rating": 5}, student.Token).Code, "has not attended yet")
		markPresent(t, enrollID)
		assert.Equal(t, http.StatusNotFound, executeRequest("POST", path, map[string]any{"rating": 5}, stranger.Token).Code)
		assert.Equal(t, http.StatusForbidden, executeRequest("POST", "/v1/enrollments/"+absentEnroll+"/review", map[string]any{"rating": 5}, absent.Token).Code)
	})

	t.Run("rating and comment validation", func(t *testing.T) {
		for name, body := range map[string]map[string]any{
			"missing rating": {"comment": "ok"},
			"negative":       {"rating": -0.5},
			"above five":     {"rating": 5.5},
			"quarter step":   {"rating": 4.25},
			"third step":     {"rating": 3.3},
			"long comment":   {"rating": 4, "comment": strings.Repeat("好", 51)},
			"wrong coach":    {"rating": 4, "coach_id": coachB.ID},
			"bad coach id":   {"rating": 4, "coach_id": "x"},
		} {
			assert.Equal(t, http.StatusBadRequest, executeRequest("POST", path, body, student.Token).Code, name)
		}
	})

	t.Run("a review with half points and a 50-character comment", func(t *testing.T) {
		comment := strings.Repeat("好", 50)
		w := executeRequest("POST", path, map[string]any{"rating": 4.5, "comment": comment}, student.Token)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		got := jsonBody[map[string]any](t, w.Body.Bytes())
		assert.EqualValues(t, 4.5, got["rating"])
		assert.Equal(t, comment, got["comment"])
		assert.Equal(t, coachA.ID, got["coach_id"], "single-teacher courses need no coach_id")
		assert.Equal(t, 1, countNotifications(t, coachA.ID, "coach_review_received"))
		assert.Equal(t, http.StatusConflict, executeRequest("POST", path, map[string]any{"rating": 1}, student.Token).Code, "one review per enrollment and coach")

		w = executeRequest("GET", path, nil, student.Token)
		require.Equal(t, http.StatusOK, w.Code)
		assert.EqualValues(t, 1, jsonBody[map[string]any](t, w.Body.Bytes())["total"])
		assert.Equal(t, http.StatusNotFound, executeRequest("GET", path, nil, stranger.Token).Code)
	})

	t.Run("zero is a valid rating", func(t *testing.T) {
		markPresent(t, absentEnroll)
		w := executeRequest("POST", "/v1/enrollments/"+absentEnroll+"/review", map[string]any{"rating": 0}, absent.Token)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		assert.EqualValues(t, 0, jsonBody[map[string]any](t, w.Body.Bytes())["rating"])
	})

	t.Run("public listing, summary, and coach profile rating", func(t *testing.T) {
		w := executeRequest("GET", "/v1/coaches/"+coachA.ID+"/reviews", nil, "")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		got := jsonBody[map[string]any](t, w.Body.Bytes())
		assert.EqualValues(t, 2, got["total"])
		sum := got["summary"].(map[string]any)
		assert.EqualValues(t, 2, sum["count"])
		assert.EqualValues(t, 2.25, sum["average"])
		item := got["items"].([]any)[0].(map[string]any)
		assert.NotContains(t, item, "hidden_reason", "moderation fields stay private")
		assert.NotContains(t, item, "is_visible")

		w = executeRequest("GET", "/v1/coaches/"+coachA.ID, nil, "")
		rating := jsonBody[map[string]any](t, w.Body.Bytes())["rating"].(map[string]any)
		assert.EqualValues(t, 2.25, rating["average"])
		assert.EqualValues(t, 2, rating["count"])
		assert.Equal(t, http.StatusNotFound, executeRequest("GET", "/v1/coaches/"+uuid.NewString()+"/reviews", nil, "").Code)
	})

	t.Run("moderation hides a review without touching the internal score", func(t *testing.T) {
		w := executeRequest("GET", "/v1/admin/reviews?coach_id="+coachA.ID, nil, admin.Token)
		require.Equal(t, http.StatusOK, w.Code)
		items := jsonBody[map[string]any](t, w.Body.Bytes())["items"].([]any)
		require.Len(t, items, 2)
		var low string
		for _, it := range items {
			if it.(map[string]any)["rating"].(float64) == 0 {
				low = it.(map[string]any)["id"].(string)
			}
		}
		assert.Equal(t, http.StatusForbidden, executeRequest("GET", "/v1/admin/reviews", nil, student.Token).Code)
		assert.Equal(t, http.StatusForbidden, executeRequest("PATCH", "/v1/admin/reviews/"+low, map[string]any{"is_visible": false, "reason": "x"}, coachA.Token).Code)
		assert.Equal(t, http.StatusBadRequest, executeRequest("PATCH", "/v1/admin/reviews/"+low, map[string]any{"is_visible": false}, admin.Token).Code, "hiding needs a reason")
		require.Equal(t, http.StatusOK, executeRequest("PATCH", "/v1/admin/reviews/"+low, map[string]any{"is_visible": false, "reason": "abusive"}, admin.Token).Code)

		w = executeRequest("GET", "/v1/coaches/"+coachA.ID+"/reviews", nil, "")
		got := jsonBody[map[string]any](t, w.Body.Bytes())
		assert.EqualValues(t, 1, got["total"])
		assert.EqualValues(t, 4.5, got["summary"].(map[string]any)["average"])

		assert.Equal(t, http.StatusOK, executeRequest("PATCH", "/v1/admin/reviews/"+low, map[string]any{"is_visible": true}, admin.Token).Code)
		w = executeRequest("GET", "/v1/coaches/"+coachA.ID+"/reviews", nil, "")
		assert.EqualValues(t, 2, jsonBody[map[string]any](t, w.Body.Bytes())["total"])
	})

	t.Run("courses with several teachers need coach_id", func(t *testing.T) {
		w := executeRequest("POST", "/v1/coach-groups", map[string]any{"name": "Duo"}, coachB.Token)
		require.Equal(t, http.StatusCreated, w.Code)
		groupID := jsonBody[map[string]any](t, w.Body.Bytes())["id"].(string)
		w = executeRequest("POST", "/v1/coach-groups/"+groupID+"/invitations", map[string]any{"user_id": coachA.ID, "role": "coach"}, coachB.Token)
		require.Equal(t, http.StatusCreated, w.Code)
		// coachA already belongs to no group; accept.
		inv := jsonBody[map[string]any](t, w.Body.Bytes())["id"].(string)
		require.Equal(t, http.StatusOK, executeRequest("POST", "/v1/coach-invitations/"+inv+"/accept", nil, coachA.Token).Code)

		duo := createCourseOK(t, coachB.Token, courseBody(sportID, map[string]any{"group_id": groupID, "coach_ids": []string{coachA.ID, coachB.ID}}))["id"].(string)
		duoStudent := newCoachUser(t, "duostudent", false)
		duoEnroll := enrollOK(t, duo, duoStudent, "")
		markPresent(t, duoEnroll)
		p := "/v1/enrollments/" + duoEnroll + "/review"
		assert.Equal(t, http.StatusBadRequest, executeRequest("POST", p, map[string]any{"rating": 5}, duoStudent.Token).Code)
		assert.Equal(t, http.StatusCreated, executeRequest("POST", p, map[string]any{"rating": 5, "coach_id": coachA.ID}, duoStudent.Token).Code)
		assert.Equal(t, http.StatusCreated, executeRequest("POST", p, map[string]any{"rating": 3, "coach_id": coachB.ID}, duoStudent.Token).Code, "each coach can be reviewed once")
	})
}

func TestComplaints(t *testing.T) {
	clearTables()
	admin := newCoachUser(t, "admin", true)
	sportID := getSportID(t, "BADMINTON")
	coachA := makeApprovedCoach(t, "coacha", admin, sportID)
	student := newCoachUser(t, "student", false)
	stranger := newCoachUser(t, "stranger", false)
	courseID := createCourseOK(t, coachA.Token, courseBody(sportID, nil))["id"].(string)
	enrollID := enrollOK(t, courseID, student, "")

	var complaintID string
	t.Run("a student files a complaint about the coach", func(t *testing.T) {
		assert.Equal(t, http.StatusUnauthorized, executeRequest("POST", "/v1/complaints", map[string]any{}, "").Code)
		base := map[string]any{"enrollment_id": enrollID, "category": "no_show", "description": "the coach never came"}
		for name, mod := range map[string]map[string]any{
			"bad category":   {"category": "gossip"},
			"no description": {"description": ""},
			"long":           {"description": strings.Repeat("x", 1001)},
			"bad enrollment": {"enrollment_id": "x"},
			"wrong target":   {"target_user_id": student.ID},
		} {
			body := map[string]any{}
			for k, v := range base {
				body[k] = v
			}
			for k, v := range mod {
				body[k] = v
			}
			assert.Equal(t, http.StatusBadRequest, executeRequest("POST", "/v1/complaints", body, student.Token).Code, name)
		}
		assert.Equal(t, http.StatusNotFound, executeRequest("POST", "/v1/complaints", base, stranger.Token).Code, "no relation to the enrollment")

		w := executeRequest("POST", "/v1/complaints", base, student.Token)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		got := jsonBody[map[string]any](t, w.Body.Bytes())
		complaintID = got["id"].(string)
		assert.Equal(t, "open", got["status"])
		assert.Equal(t, "student", got["reporter_role"])
		assert.Equal(t, coachA.ID, got["target"].(map[string]any)["id"])
		assert.Equal(t, 1, countNotifications(t, admin.ID, "complaint_filed"))
		assert.Equal(t, http.StatusConflict, executeRequest("POST", "/v1/complaints", base, student.Token).Code, "one open complaint at a time")
	})

	t.Run("a coach can complain about a student", func(t *testing.T) {
		w := executeRequest("POST", "/v1/complaints", map[string]any{"enrollment_id": enrollID, "category": "misconduct", "description": "rude to others"}, coachA.Token)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		got := jsonBody[map[string]any](t, w.Body.Bytes())
		assert.Equal(t, "coach", got["reporter_role"])
		assert.Equal(t, student.ID, got["target"].(map[string]any)["id"])
	})

	t.Run("reporters see their own complaints, never internal notes", func(t *testing.T) {
		w := executeRequest("GET", "/v1/complaints", nil, student.Token)
		require.Equal(t, http.StatusOK, w.Code)
		assert.EqualValues(t, 1, jsonBody[map[string]any](t, w.Body.Bytes())["total"])
		assert.Equal(t, http.StatusNotFound, executeRequest("GET", "/v1/complaints/"+complaintID, nil, stranger.Token).Code)
		assert.Equal(t, http.StatusNotFound, executeRequest("GET", "/v1/complaints/"+complaintID, nil, coachA.Token).Code, "the accused cannot read the report")

		assert.Equal(t, http.StatusForbidden, executeRequest("POST", "/v1/admin/complaints/"+complaintID+"/notes", map[string]any{"note": "x"}, student.Token).Code)
		assert.Equal(t, http.StatusBadRequest, executeRequest("POST", "/v1/admin/complaints/"+complaintID+"/notes", map[string]any{"note": ""}, admin.Token).Code)
		require.Equal(t, http.StatusCreated, executeRequest("POST", "/v1/admin/complaints/"+complaintID+"/notes", map[string]any{"note": "coach has two prior reports"}, admin.Token).Code)

		w = executeRequest("GET", "/v1/complaints/"+complaintID, nil, student.Token)
		require.Equal(t, http.StatusOK, w.Code)
		assert.NotContains(t, w.Body.String(), "two prior reports")
		w = executeRequest("GET", "/v1/admin/complaints/"+complaintID, nil, admin.Token)
		assert.Contains(t, w.Body.String(), "two prior reports")
	})

	t.Run("staff triage with simple status changes", func(t *testing.T) {
		w := executeRequest("GET", "/v1/admin/complaints?status=open", nil, admin.Token)
		require.Equal(t, http.StatusOK, w.Code)
		assert.EqualValues(t, 2, jsonBody[map[string]any](t, w.Body.Bytes())["total"])
		assert.EqualValues(t, 1, jsonBody[map[string]any](t, executeRequest("GET", "/v1/admin/complaints?target_user_id="+coachA.ID, nil, admin.Token).Body.Bytes())["total"])
		assert.Equal(t, http.StatusForbidden, executeRequest("GET", "/v1/admin/complaints", nil, student.Token).Code)

		path := "/v1/admin/complaints/" + complaintID
		assert.Equal(t, http.StatusForbidden, executeRequest("PATCH", path, map[string]any{"status": "resolved", "note": "x"}, student.Token).Code)
		assert.Equal(t, http.StatusBadRequest, executeRequest("PATCH", path, map[string]any{"status": "weird"}, admin.Token).Code)
		assert.Equal(t, http.StatusBadRequest, executeRequest("PATCH", path, map[string]any{"status": "resolved"}, admin.Token).Code, "closing needs a note for the reporter")
		assert.Equal(t, http.StatusConflict, executeRequest("PATCH", path, map[string]any{"status": "open"}, admin.Token).Code)

		require.Equal(t, http.StatusOK, executeRequest("PATCH", path, map[string]any{"status": "in_review", "note": "looking into it"}, admin.Token).Code)
		w = executeRequest("PATCH", path, map[string]any{"status": "resolved", "note": "coach warned"}, admin.Token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		got := jsonBody[map[string]any](t, w.Body.Bytes())
		assert.Equal(t, "resolved", got["status"])
		assert.Equal(t, "coach warned", got["resolution_note"])
		assert.Equal(t, 2, countNotifications(t, student.ID, "complaint_updated"))

		// Staff can reopen, and the reporter can file again once closed.
		require.Equal(t, http.StatusOK, executeRequest("PATCH", path, map[string]any{"status": "in_review"}, admin.Token).Code)
		w = executeRequest("GET", path, nil, admin.Token)
		assert.Nil(t, jsonBody[map[string]any](t, w.Body.Bytes())["resolution_note"], "reopening clears the resolution")
		require.Equal(t, http.StatusOK, executeRequest("PATCH", path, map[string]any{"status": "dismissed", "note": "not enough evidence"}, admin.Token).Code)
		assert.Equal(t, http.StatusCreated, executeRequest("POST", "/v1/complaints",
			map[string]any{"enrollment_id": enrollID, "category": "quality", "description": "again"}, student.Token).Code)
	})
}

func TestReputationAndSuspension(t *testing.T) {
	clearTables()
	admin := newCoachUser(t, "admin", true)
	sportID := getSportID(t, "BADMINTON")
	coachA := makeApprovedCoach(t, "coacha", admin, sportID)
	pending := newCoachUser(t, "pending", false)
	student := newCoachUser(t, "student", false)
	require.Equal(t, http.StatusCreated, executeRequest("POST", "/v1/coach-applications", coachApplyBody(sportID, nil), pending.Token).Code)

	courseID := createCourseOK(t, coachA.Token, paidBody(sportID, nil))["id"].(string)
	enrollID := enrollOK(t, courseID, student, "onsite")
	w := executeRequest("POST", "/v1/complaints", map[string]any{"enrollment_id": enrollID, "category": "quality", "description": "poor class"}, student.Token)
	require.Equal(t, http.StatusCreated, w.Code)
	complaintID := jsonBody[map[string]any](t, w.Body.Bytes())["id"].(string)

	t.Run("internal reputation is a ledger with no automatic rules", func(t *testing.T) {
		rep := "/v1/admin/coaches/" + coachA.ID + "/reputation"
		assert.Equal(t, http.StatusForbidden, executeRequest("GET", rep, nil, coachA.Token).Code, "coaches cannot see their own internal score")
		assert.Equal(t, http.StatusNotFound, executeRequest("GET", "/v1/admin/coaches/"+uuid.NewString()+"/reputation", nil, admin.Token).Code)

		w := executeRequest("GET", rep, nil, admin.Token)
		require.Equal(t, http.StatusOK, w.Code)
		got := jsonBody[map[string]any](t, w.Body.Bytes())
		assert.EqualValues(t, 100, got["base_score"])
		assert.EqualValues(t, 100, got["score"])
		assert.EqualValues(t, 1, got["open_complaints"])

		events := "/v1/admin/coaches/" + coachA.ID + "/reputation-events"
		for name, body := range map[string]map[string]any{
			"zero delta": {"delta": 0, "reason": "x"},
			"too big":    {"delta": 101, "reason": "x"},
			"no reason":  {"delta": -5},
		} {
			assert.Equal(t, http.StatusBadRequest, executeRequest("POST", events, body, admin.Token).Code, name)
		}
		assert.Equal(t, http.StatusNotFound, executeRequest("POST", events, map[string]any{"delta": -5, "reason": "x", "complaint_id": uuid.NewString()}, admin.Token).Code, "the complaint must be about this coach")
		assert.Equal(t, http.StatusForbidden, executeRequest("POST", events, map[string]any{"delta": -5, "reason": "x"}, coachA.Token).Code)
		require.Equal(t, http.StatusCreated, executeRequest("POST", events, map[string]any{"delta": -15, "reason": "no-show", "complaint_id": complaintID}, admin.Token).Code)
		require.Equal(t, http.StatusCreated, executeRequest("POST", events, map[string]any{"delta": 5, "reason": "goodwill"}, admin.Token).Code)

		got = jsonBody[map[string]any](t, executeRequest("GET", rep, nil, admin.Token).Body.Bytes())
		assert.EqualValues(t, -10, got["total_adjustment"])
		assert.EqualValues(t, 90, got["score"])
		assert.Len(t, got["events"], 2)
		assert.Equal(t, "approved", got["coach_status"], "a low score never suspends by itself")
	})

	t.Run("suspension lists what it affects and cancels nothing", func(t *testing.T) {
		path := "/v1/admin/coaches/" + coachA.ID + "/suspensions"
		assert.Equal(t, http.StatusForbidden, executeRequest("POST", path, map[string]any{"reason": "x"}, coachA.Token).Code)
		assert.Equal(t, http.StatusBadRequest, executeRequest("POST", path, map[string]any{}, admin.Token).Code)
		assert.Equal(t, http.StatusConflict, executeRequest("POST", "/v1/admin/coaches/"+pending.ID+"/suspensions", map[string]any{"reason": "x"}, admin.Token).Code, "only approved coaches")
		assert.Equal(t, http.StatusNotFound, executeRequest("POST", "/v1/admin/coaches/"+uuid.NewString()+"/suspensions", map[string]any{"reason": "x"}, admin.Token).Code)

		w := executeRequest("POST", path, map[string]any{"reason": "repeated no-shows"}, admin.Token)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		got := jsonBody[map[string]any](t, w.Body.Bytes())
		assert.Equal(t, "active", got["status"])
		impact := got["impact"].(map[string]any)
		require.Len(t, impact["courses"], 1)
		assert.Len(t, impact["sessions"], 1)
		enrollments := impact["enrollments"].([]any)
		require.Len(t, enrollments, 1)
		assert.Equal(t, enrollID, enrollments[0].(map[string]any)["id"])
		assert.Equal(t, 1, countNotifications(t, coachA.ID, "coach_suspended"))
		suspensionID := got["id"].(string)

		assert.Equal(t, http.StatusConflict, executeRequest("POST", path, map[string]any{"reason": "again"}, admin.Token).Code)

		// Nothing was cancelled: enrollments stay active, the coach can still manage the course.
		var status string
		require.NoError(t, testPool.QueryRow(context.Background(), "SELECT status FROM public.enrollments WHERE id = $1", enrollID).Scan(&status))
		assert.Equal(t, "active", status)
		assert.Equal(t, http.StatusNotFound, executeRequest("GET", "/v1/coaches/"+coachA.ID, nil, "").Code, "suspended coaches leave the public directory")
		assert.EqualValues(t, 0, jsonBody[map[string]any](t, executeRequest("GET", "/v1/courses", nil, "").Body.Bytes())["total"])
		assert.Equal(t, http.StatusForbidden, executeRequest("POST", "/v1/courses", courseBody(sportID, nil), coachA.Token).Code)
		assert.Equal(t, http.StatusOK, executeRequest("PATCH", "/v1/courses/"+courseID, map[string]any{"status": "cancelled"}, coachA.Token).Code,
			"the coach can still wind down existing courses")

		w = executeRequest("GET", path, nil, admin.Token)
		require.Equal(t, http.StatusOK, w.Code)
		assert.EqualValues(t, 1, jsonBody[map[string]any](t, w.Body.Bytes())["total"])
		rep := jsonBody[map[string]any](t, executeRequest("GET", "/v1/admin/coaches/"+coachA.ID+"/reputation", nil, admin.Token).Body.Bytes())
		assert.NotNil(t, rep["active_suspension"])
		assert.Equal(t, "suspended", rep["coach_status"])

		// Lifting restores the coach.
		lift := "/v1/admin/suspensions/" + suspensionID + "/lift"
		assert.Equal(t, http.StatusForbidden, executeRequest("POST", lift, map[string]any{}, coachA.Token).Code)
		require.Equal(t, http.StatusOK, executeRequest("POST", lift, map[string]any{"note": "appeal accepted"}, admin.Token).Code)
		assert.Equal(t, http.StatusConflict, executeRequest("POST", lift, nil, admin.Token).Code)
		assert.Equal(t, 1, countNotifications(t, coachA.ID, "coach_suspension_lifted"))
		assert.Equal(t, http.StatusOK, executeRequest("GET", "/v1/coaches/"+coachA.ID, nil, "").Code)
		assert.Equal(t, http.StatusCreated, executeRequest("POST", "/v1/courses", courseBody(sportID, nil), coachA.Token).Code)
		// And can be suspended again later.
		assert.Equal(t, http.StatusCreated, executeRequest("POST", path, map[string]any{"reason": "second offence"}, admin.Token).Code)
	})
}
