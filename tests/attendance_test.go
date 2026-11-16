package tests

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// attendanceFixture builds a coach with two classes of the same sport so
// students of class A can make up in class B.
type attendanceFixture struct {
	admin, coach, otherCoach coachUser
	sportID                  string
	classA, classB, classX   map[string]any // course JSON
	sessionsA, sessionsB     []string
}

func sessionIDs(course map[string]any) []string {
	var ids []string
	for _, s := range course["sessions"].([]any) {
		ids = append(ids, s.(map[string]any)["id"].(string))
	}
	return ids
}

func newAttendanceFixture(t *testing.T) *attendanceFixture {
	clearTables()
	f := &attendanceFixture{}
	f.admin = newCoachUser(t, "admin", true)
	f.sportID = getSportID(t, "BADMINTON")
	f.coach = makeApprovedCoach(t, "coach", f.admin, f.sportID)
	f.otherCoach = makeApprovedCoach(t, "othercoach", f.admin, f.sportID)

	rules := map[string]any{
		"course_type": "recurring", "allow_leave": true, "leave_deadline_hours": 2,
		"allow_makeup": true, "makeup_limit": 1, "makeup_valid_days": 30,
	}
	merge := func(extra map[string]any) map[string]any {
		out := map[string]any{}
		for k, v := range rules {
			out[k] = v
		}
		for k, v := range extra {
			out[k] = v
		}
		return out
	}
	f.classA = createCourseOK(t, f.coach.Token, courseBody(f.sportID, merge(map[string]any{
		"title": "Monday class", "capacity": 3,
		"sessions": []map[string]any{futureSession(48), futureSession(120), futureSession(200)}})))
	f.classB = createCourseOK(t, f.coach.Token, courseBody(f.sportID, merge(map[string]any{
		"title": "Wednesday class", "capacity": 1,
		"sessions": []map[string]any{futureSession(72), futureSession(150)}})))
	f.classX = createCourseOK(t, f.otherCoach.Token, courseBody(f.sportID, merge(map[string]any{
		"title": "Someone else's class", "capacity": 5, "sessions": []map[string]any{futureSession(80)}})))
	f.sessionsA, f.sessionsB = sessionIDs(f.classA), sessionIDs(f.classB)
	return f
}

func TestLeaveRules(t *testing.T) {
	f := newAttendanceFixture(t)
	student := newCoachUser(t, "student", false)
	stranger := newCoachUser(t, "stranger", false)
	enrollOK(t, f.classA["id"].(string), student, "")
	path := "/v1/sessions/" + f.sessionsA[0] + "/leave-requests"

	assert.Equal(t, http.StatusUnauthorized, executeRequest("POST", path, nil, "").Code)
	assert.Equal(t, http.StatusNotFound, executeRequest("POST", path, nil, stranger.Token).Code, "only enrolled students can take leave")
	assert.Equal(t, http.StatusNotFound, executeRequest("POST", "/v1/sessions/"+uuid.NewString()+"/leave-requests", nil, student.Token).Code)
	assert.Equal(t, http.StatusBadRequest, executeRequest("POST", path, map[string]any{"reason": string(make([]rune, 201))}, student.Token).Code)

	// A course that does not allow leave.
	noLeave := createCourseOK(t, f.coach.Token, courseBody(f.sportID, map[string]any{"allow_leave": false}))
	enrollOK(t, noLeave["id"].(string), student, "")
	assert.Equal(t, http.StatusConflict, executeRequest("POST", "/v1/sessions/"+sessionIDs(noLeave)[0]+"/leave-requests", nil, student.Token).Code)

	// Inside the 2-hour deadline.
	tight := createCourseOK(t, f.coach.Token, courseBody(f.sportID, map[string]any{
		"allow_leave": true, "leave_deadline_hours": 5, "sessions": []map[string]any{futureSession(3)}}))
	enrollOK(t, tight["id"].(string), student, "")
	assert.Equal(t, http.StatusConflict, executeRequest("POST", "/v1/sessions/"+sessionIDs(tight)[0]+"/leave-requests", nil, student.Token).Code)

	w := executeRequest("POST", path, map[string]any{"reason": "business trip"}, student.Token)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	leave := jsonBody[map[string]any](t, w.Body.Bytes())
	assert.Equal(t, "open", leave["status"])
	assert.Equal(t, 1, countNotifications(t, f.coach.ID, "leave_requested"))
	assert.Equal(t, http.StatusConflict, executeRequest("POST", path, nil, student.Token).Code, "one leave per session")

	// Leave can be withdrawn, then filed again.
	leaveID := leave["id"].(string)
	assert.Equal(t, http.StatusNotFound, executeRequest("DELETE", "/v1/leave-requests/"+leaveID, nil, stranger.Token).Code)
	assert.Equal(t, http.StatusNoContent, executeRequest("DELETE", "/v1/leave-requests/"+leaveID, nil, student.Token).Code)
	assert.Equal(t, http.StatusCreated, executeRequest("POST", path, nil, student.Token).Code)
}

func TestMakeupFlow(t *testing.T) {
	f := newAttendanceFixture(t)
	student := newCoachUser(t, "student", false)
	rival := newCoachUser(t, "rival", false)
	stranger := newCoachUser(t, "stranger", false)
	enrollOK(t, f.classA["id"].(string), student, "")
	enrollOK(t, f.classA["id"].(string), rival, "")

	leave := func(token, sessionID string) string {
		w := executeRequest("POST", "/v1/sessions/"+sessionID+"/leave-requests", nil, token)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		return jsonBody[map[string]any](t, w.Body.Bytes())["id"].(string)
	}
	leaveID := leave(student.Token, f.sessionsA[0])

	t.Run("options only list free seats in the same owner's other classes", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound, executeRequest("GET", "/v1/leave-requests/"+leaveID+"/makeup-options", nil, stranger.Token).Code)
		w := executeRequest("GET", "/v1/leave-requests/"+leaveID+"/makeup-options", nil, student.Token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		got := jsonBody[map[string]any](t, w.Body.Bytes())
		assert.Equal(t, true, got["eligible"])
		assert.EqualValues(t, 1, got["remaining_makeups"])
		opts := got["options"].([]any)
		require.Len(t, opts, 2, "both Wednesday sessions; not Monday's own, not the other coach's class")
		for _, o := range opts {
			assert.Equal(t, "Wednesday class", o.(map[string]any)["course_title"])
			assert.EqualValues(t, 1, o.(map[string]any)["remaining_seats"])
		}
		// The coach may look at it too.
		assert.Equal(t, http.StatusOK, executeRequest("GET", "/v1/leave-requests/"+leaveID+"/makeup-options", nil, f.coach.Token).Code)
	})

	t.Run("selection re-validates eligibility", func(t *testing.T) {
		path := "/v1/leave-requests/" + leaveID + "/makeup-selection"
		assert.Equal(t, http.StatusBadRequest, executeRequest("POST", path, map[string]any{"session_id": "nope"}, student.Token).Code)
		assert.Equal(t, http.StatusConflict, executeRequest("POST", path, map[string]any{"session_id": f.sessionsA[1]}, student.Token).Code, "own class is not a make-up")
		assert.Equal(t, http.StatusConflict, executeRequest("POST", path, map[string]any{"session_id": sessionIDs(f.classX)[0]}, student.Token).Code, "another coach's class")
		assert.Equal(t, http.StatusConflict, executeRequest("POST", path, map[string]any{"session_id": uuid.NewString()}, student.Token).Code)
		assert.Equal(t, http.StatusNotFound, executeRequest("POST", path, map[string]any{"session_id": f.sessionsB[0]}, stranger.Token).Code)

		w := executeRequest("POST", path, map[string]any{"session_id": f.sessionsB[0]}, student.Token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		got := jsonBody[map[string]any](t, w.Body.Bytes())
		assert.Equal(t, "used", got["status"])
		assert.Equal(t, f.sessionsB[0], got["makeup"].(map[string]any)["session_id"])
		assert.Equal(t, 1, countNotifications(t, f.coach.ID, "makeup_booked"))
		assert.Equal(t, http.StatusConflict, executeRequest("POST", path, map[string]any{"session_id": f.sessionsB[1]}, student.Token).Code, "already used")
		assert.Equal(t, http.StatusConflict, executeRequest("DELETE", "/v1/leave-requests/"+leaveID, nil, student.Token).Code, "a leave with a booked make-up cannot be withdrawn")
	})

	t.Run("seat limit and make-up limit", func(t *testing.T) {
		// The Wednesday session 1 (capacity 1) is taken; the rival's leave sees only session 2.
		rivalLeave := leave(rival.Token, f.sessionsA[1])
		w := executeRequest("GET", "/v1/leave-requests/"+rivalLeave+"/makeup-options", nil, rival.Token)
		opts := jsonBody[map[string]any](t, w.Body.Bytes())["options"].([]any)
		require.Len(t, opts, 1)
		assert.Equal(t, f.sessionsB[1], opts[0].(map[string]any)["session_id"])
		assert.Equal(t, http.StatusConflict, executeRequest("POST", "/v1/leave-requests/"+rivalLeave+"/makeup-selection", map[string]any{"session_id": f.sessionsB[0]}, rival.Token).Code, "session is full")

		// The student used the single allowed make-up.
		second := leave(student.Token, f.sessionsA[2])
		w = executeRequest("GET", "/v1/leave-requests/"+second+"/makeup-options", nil, student.Token)
		got := jsonBody[map[string]any](t, w.Body.Bytes())
		assert.Equal(t, false, got["eligible"])
		assert.NotEmpty(t, got["reason"])
		assert.Empty(t, got["options"])
		assert.Equal(t, http.StatusConflict, executeRequest("POST", "/v1/leave-requests/"+second+"/makeup-selection", map[string]any{"session_id": f.sessionsB[1]}, student.Token).Code)
	})

	t.Run("cancelling a make-up reopens the leave and frees the seat", func(t *testing.T) {
		path := "/v1/leave-requests/" + leaveID + "/makeup-selection"
		assert.Equal(t, http.StatusNotFound, executeRequest("DELETE", path, nil, stranger.Token).Code)
		w := executeRequest("DELETE", path, nil, student.Token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, "open", jsonBody[map[string]any](t, w.Body.Bytes())["status"])
		assert.Equal(t, http.StatusConflict, executeRequest("DELETE", path, nil, student.Token).Code)
	})

	t.Run("a cancelled make-up session releases its guests", func(t *testing.T) {
		w := executeRequest("POST", "/v1/leave-requests/"+leaveID+"/makeup-selection", map[string]any{"session_id": f.sessionsB[0]}, student.Token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		w = executeRequest("PATCH", "/v1/courses/"+f.classB["id"].(string)+"/sessions/"+f.sessionsB[0], map[string]any{"cancel": true, "reason": "gym closed"}, f.coach.Token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, 1, countNotifications(t, student.ID, "makeup_released"))
		var status string
		require.NoError(t, testPool.QueryRow(context.Background(), "SELECT status FROM public.leave_requests WHERE id = $1", leaveID).Scan(&status))
		assert.Equal(t, "open", status)
	})
}

// Two students racing for the last make-up seat: exactly one gets it.
func TestMakeupSeatRace(t *testing.T) {
	f := newAttendanceFixture(t)
	s1, s2 := newCoachUser(t, "s1", false), newCoachUser(t, "s2", false)
	enrollOK(t, f.classA["id"].(string), s1, "")
	enrollOK(t, f.classA["id"].(string), s2, "")
	ids := make([]string, 2)
	for i, u := range []coachUser{s1, s2} {
		w := executeRequest("POST", "/v1/sessions/"+f.sessionsA[0]+"/leave-requests", nil, u.Token)
		require.Equal(t, http.StatusCreated, w.Code)
		ids[i] = jsonBody[map[string]any](t, w.Body.Bytes())["id"].(string)
	}
	users := []coachUser{s1, s2}
	codes := make([]int, 2)
	var wg sync.WaitGroup
	for i := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i] = executeRequest("POST", "/v1/leave-requests/"+ids[i]+"/makeup-selection", map[string]any{"session_id": f.sessionsB[0]}, users[i].Token).Code
		}()
	}
	wg.Wait()
	assert.ElementsMatch(t, []int{http.StatusOK, http.StatusConflict}, codes)
}

func TestAttendanceRecording(t *testing.T) {
	f := newAttendanceFixture(t)
	s1, s2, guest := newCoachUser(t, "s1", false), newCoachUser(t, "s2", false), newCoachUser(t, "guest", false)
	e1 := enrollOK(t, f.classA["id"].(string), s1, "")
	e2 := enrollOK(t, f.classA["id"].(string), s2, "")
	eg := enrollOK(t, f.classA["id"].(string), guest, "")
	classBID := f.classB["id"].(string)

	// Backdate the enrollments and add a session that has already started.
	_, err := testPool.Exec(context.Background(), "UPDATE public.enrollments SET created_at = now() - interval '10 days'")
	require.NoError(t, err)
	var pastA, pastB string
	require.NoError(t, testPool.QueryRow(context.Background(), `INSERT INTO public.course_sessions (course_id, seq, start_at, end_at)
		VALUES ($1, 90, now() - interval '3 hours', now() - interval '2 hours') RETURNING id`, f.classA["id"]).Scan(&pastA))
	require.NoError(t, testPool.QueryRow(context.Background(), `INSERT INTO public.course_sessions (course_id, seq, start_at, end_at)
		VALUES ($1, 90, now() - interval '3 hours', now() - interval '2 hours') RETURNING id`, classBID).Scan(&pastB))

	// A guest on leave elsewhere books a make-up in the (already running) class B session directly.
	_, err = testPool.Exec(context.Background(), `INSERT INTO public.session_attendances (session_id, enrollment_id, kind, status) VALUES ($1, $2, 'makeup', 'pending')`, pastB, eg)
	require.NoError(t, err)
	// s2 took leave for the running session.
	_, err = testPool.Exec(context.Background(), `INSERT INTO public.leave_requests (session_id, enrollment_id) VALUES ($1, $2)`, pastA, e2)
	require.NoError(t, err)
	_, err = testPool.Exec(context.Background(), `INSERT INTO public.session_attendances (session_id, enrollment_id, kind, status) VALUES ($1, $2, 'regular', 'leave')`, pastA, e2)
	require.NoError(t, err)

	t.Run("roster is for managers and shows skill level", func(t *testing.T) {
		path := "/v1/sessions/" + pastA + "/attendance"
		assert.Equal(t, http.StatusUnauthorized, executeRequest("GET", path, nil, "").Code)
		assert.Equal(t, http.StatusForbidden, executeRequest("GET", path, nil, s1.Token).Code)
		assert.Equal(t, http.StatusForbidden, executeRequest("GET", path, nil, f.otherCoach.Token).Code)
		w := executeRequest("GET", path, nil, f.coach.Token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		got := jsonBody[map[string]any](t, w.Body.Bytes())
		assert.EqualValues(t, 3, got["total"])
		for _, it := range got["items"].([]any) {
			item := it.(map[string]any)
			assert.Equal(t, "regular", item["kind"])
			assert.NotNil(t, item["skill_level"], "coaches see their students' skill level")
			if item["enrollment_id"] == e2 {
				assert.Equal(t, "leave", item["status"])
			} else {
				assert.Equal(t, "pending", item["status"])
			}
		}
		// The make-up guest shows up in the host class's roster.
		w = executeRequest("GET", "/v1/sessions/"+pastB+"/attendance", nil, f.coach.Token)
		items := jsonBody[map[string]any](t, w.Body.Bytes())["items"].([]any)
		require.Len(t, items, 1)
		assert.Equal(t, "makeup", items[0].(map[string]any)["kind"])
		assert.Equal(t, "Monday class", items[0].(map[string]any)["from_course"])
	})

	t.Run("marking rules", func(t *testing.T) {
		path := "/v1/sessions/" + pastA + "/attendance"
		rec := func(id, status string) map[string]any {
			return map[string]any{"records": []map[string]any{{"enrollment_id": id, "status": status}}}
		}
		assert.Equal(t, http.StatusForbidden, executeRequest("PATCH", path, rec(e1, "present"), s1.Token).Code, "students cannot mark themselves")
		assert.Equal(t, http.StatusBadRequest, executeRequest("PATCH", path, map[string]any{"records": []map[string]any{}}, f.coach.Token).Code)
		assert.Equal(t, http.StatusBadRequest, executeRequest("PATCH", path, rec(e1, "late"), f.coach.Token).Code)
		assert.Equal(t, http.StatusBadRequest, executeRequest("PATCH", path, rec(uuid.NewString(), "present"), f.coach.Token).Code, "not in the roster")
		outsiderEnroll := enrollOK(t, f.classX["id"].(string), newCoachUser(t, "xstudent", false), "")
		assert.Equal(t, http.StatusBadRequest, executeRequest("PATCH", path, rec(outsiderEnroll, "present"), f.coach.Token).Code, "another class's student is not on this roster")
		assert.Equal(t, http.StatusConflict, executeRequest("PATCH", path, rec(e2, "present"), f.coach.Token).Code, "a student on leave cannot be marked")
		assert.Equal(t, http.StatusConflict, executeRequest("PATCH", "/v1/sessions/"+f.sessionsA[0]+"/attendance", rec(e1, "present"), f.coach.Token).Code, "the session has not started")

		w := executeRequest("PATCH", path, map[string]any{"records": []map[string]any{
			{"enrollment_id": e1, "status": "present"}, {"enrollment_id": outsiderEnroll, "status": "absent"}}}, f.coach.Token)
		assert.Equal(t, http.StatusBadRequest, w.Code, "the whole batch fails if one record is invalid")
		var n int
		require.NoError(t, testPool.QueryRow(context.Background(), "SELECT count(*) FROM public.session_attendances WHERE session_id = $1 AND enrollment_id = $2", pastA, e1).Scan(&n))
		assert.Equal(t, 0, n, "a failed batch records nothing")

		w = executeRequest("PATCH", path, rec(e1, "present"), f.coach.Token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		w = executeRequest("PATCH", "/v1/sessions/"+pastB+"/attendance", rec(eg, "present"), f.coach.Token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		w = executeRequest("PATCH", path, rec(e1, "absent"), f.coach.Token)
		require.Equal(t, http.StatusOK, w.Code, "marks can be corrected")
	})

	t.Run("attendance history and summary", func(t *testing.T) {
		w := executeRequest("GET", "/v1/enrollments/"+eg+"/attendance", nil, guest.Token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		got := jsonBody[map[string]any](t, w.Body.Bytes())
		assert.EqualValues(t, 1, got["summary"].(map[string]any)["present"])
		kinds := map[string]bool{}
		for _, it := range got["items"].([]any) {
			kinds[it.(map[string]any)["kind"].(string)] = true
		}
		assert.True(t, kinds["makeup"], "the make-up appears in the student's own history")

		w = executeRequest("GET", "/v1/enrollments/"+e2+"/attendance", nil, f.coach.Token)
		require.Equal(t, http.StatusOK, w.Code)
		assert.EqualValues(t, 1, jsonBody[map[string]any](t, w.Body.Bytes())["summary"].(map[string]any)["leave"])
		assert.Equal(t, http.StatusNotFound, executeRequest("GET", "/v1/enrollments/"+e2+"/attendance", nil, s1.Token).Code)

		// The course roster carries the same counters.
		w = executeRequest("GET", "/v1/courses/"+f.classA["id"].(string)+"/enrollments", nil, f.coach.Token)
		require.Equal(t, http.StatusOK, w.Code)
		for _, it := range jsonBody[map[string]any](t, w.Body.Bytes())["items"].([]any) {
			item := it.(map[string]any)
			att := item["attendance"].(map[string]any)
			switch item["id"] {
			case e1:
				assert.EqualValues(t, 1, att["absent"])
			case e2:
				assert.EqualValues(t, 1, att["leave"])
			}
		}
	})
}
