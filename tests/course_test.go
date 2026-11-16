package tests

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// futureSession returns an explicit session list entry starting in `hours`.
func futureSession(hours int) map[string]any {
	start := time.Now().Add(time.Duration(hours) * time.Hour).Truncate(time.Minute)
	return map[string]any{"start_at": start.Format(time.RFC3339), "end_at": start.Add(time.Hour).Format(time.RFC3339)}
}

func courseBody(sportID string, extra map[string]any) map[string]any {
	body := map[string]any{
		"title": "Badminton basics", "description": "Intro", "teaching_content": "Grip and footwork",
		"sport_id": sportID, "course_type": "single", "capacity": 4, "venue_name": "Coach's own gym",
		"price": 0, "sessions": []map[string]any{futureSession(48)},
	}
	for k, v := range extra {
		body[k] = v
	}
	return body
}

func createCourseOK(t *testing.T, token string, body map[string]any) map[string]any {
	w := executeRequest("POST", "/v1/courses", body, token)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	return jsonBody[map[string]any](t, w.Body.Bytes())
}

func paidBody(sportID string, extra map[string]any) map[string]any {
	body := courseBody(sportID, map[string]any{
		"price": 1000, "venue_fee": 200, "payment_methods": []string{"onsite", "transfer"},
		"bank_code": "8120012", "bank_account": "123456789012", "bank_account_name": "Coach Lin",
	})
	for k, v := range extra {
		body[k] = v
	}
	return body
}

func TestCourseCreationRules(t *testing.T) {
	clearTables()
	admin := newCoachUser(t, "admin", true)
	sportID := getSportID(t, "BADMINTON")
	coachA := makeApprovedCoach(t, "coacha", admin, sportID)
	plain := newCoachUser(t, "plain", false)

	t.Run("requires an approved coach", func(t *testing.T) {
		assert.Equal(t, http.StatusUnauthorized, executeRequest("POST", "/v1/courses", courseBody(sportID, nil), "").Code)
		assert.Equal(t, http.StatusForbidden, executeRequest("POST", "/v1/courses", courseBody(sportID, nil), plain.Token).Code)
	})

	t.Run("validation", func(t *testing.T) {
		cases := map[string]map[string]any{
			"bad type":            courseBody(sportID, map[string]any{"course_type": "weekly"}),
			"zero capacity":       courseBody(sportID, map[string]any{"capacity": 0}),
			"no schedule":         courseBody(sportID, map[string]any{"sessions": nil}),
			"both schedule forms": courseBody(sportID, map[string]any{"schedule": map[string]any{"start_date": "2099-01-01", "end_date": "2099-01-31", "weekdays": []int{1}, "start_time": "10:00", "end_time": "11:00"}}),
			"past session": courseBody(sportID, map[string]any{"sessions": []map[string]any{
				{"start_at": time.Now().Add(-48 * time.Hour).Format(time.RFC3339), "end_at": time.Now().Add(-47 * time.Hour).Format(time.RFC3339)}}}),
			"single with two sessions": courseBody(sportID, map[string]any{"sessions": []map[string]any{futureSession(48), futureSession(100)}}),
			"overlapping sessions": courseBody(sportID, map[string]any{"course_type": "recurring", "sessions": []map[string]any{
				futureSession(48), {"start_at": time.Now().Add(48*time.Hour + 30*time.Minute).Format(time.RFC3339), "end_at": time.Now().Add(50 * time.Hour).Format(time.RFC3339)}}}),
			"unknown sport":       courseBody(uuid.NewString(), nil),
			"bad timezone":        courseBody(sportID, map[string]any{"timezone": "Mars/Base"}),
			"paid without method": courseBody(sportID, map[string]any{"price": 500}),
			"transfer w/o bank":   courseBody(sportID, map[string]any{"price": 500, "payment_methods": []string{"transfer"}}),
			"bad bank code":       paidBody(sportID, map[string]any{"bank_code": "12"}),
			"bad account":         paidBody(sportID, map[string]any{"bank_account": "abc"}),
			"unknown method":      courseBody(sportID, map[string]any{"price": 500, "payment_methods": []string{"card"}}),
			"rule too long":       courseBody(sportID, map[string]any{"rules_refund": string(make([]rune, 501))}),
			"negative price":      courseBody(sportID, map[string]any{"price": -1}),
			"deadline in past":    courseBody(sportID, map[string]any{"enrollment_deadline": time.Now().Add(-time.Hour).Format(time.RFC3339)}),
			"deadline after last": courseBody(sportID, map[string]any{"enrollment_deadline": time.Now().Add(200 * time.Hour).Format(time.RFC3339)}),
			"coach_ids on solo":   courseBody(sportID, map[string]any{"coach_ids": []string{coachA.ID}}),
			"bad weekday":         courseBody(sportID, map[string]any{"sessions": nil, "course_type": "recurring", "schedule": map[string]any{"start_date": "2099-01-01", "end_date": "2099-01-31", "weekdays": []int{9}, "start_time": "10:00", "end_time": "11:00"}}),
		}
		for name, body := range cases {
			w := executeRequest("POST", "/v1/courses", body, coachA.Token)
			assert.Equal(t, http.StatusBadRequest, w.Code, name+": "+w.Body.String())
		}
	})

	t.Run("weekly schedule expands into sessions", func(t *testing.T) {
		out := createCourseOK(t, coachA.Token, courseBody(sportID, map[string]any{
			"course_type": "recurring", "sessions": nil, "timezone": "Asia/Taipei",
			"schedule": map[string]any{
				"start_date": "2099-01-05", "end_date": "2099-01-31", "weekdays": []int{1, 3},
				"start_time": "19:00", "end_time": "21:00", "skip_dates": []string{"2099-01-12"},
			},
		}))
		sessions := out["sessions"].([]any)
		// 2099-01-05 is a Monday: Mon 5, 19, 26 and Wed 7, 14, 21, 28 (Mon 12 skipped).
		assert.Len(t, sessions, 7, "Mondays 5,19,26 + Wednesdays 7,14,21,28")
		assert.EqualValues(t, 7, out["session_count"])
		first := sessions[0].(map[string]any)
		assert.Equal(t, "2099-01-05T11:00:00Z", first["start_at"], "19:00 Taipei is 11:00 UTC")
		assert.Equal(t, "coach", out["owner_type"])
		assert.Equal(t, true, out["is_free"])
		assert.Equal(t, true, out["can_manage"])
	})

	t.Run("private course blocked by group setting", func(t *testing.T) {
		member := makeApprovedCoach(t, "grpmember", admin, sportID)
		w := executeRequest("POST", "/v1/coach-groups", map[string]any{"name": "Closed Group", "allow_private_courses": false}, coachA.Token)
		require.Equal(t, http.StatusCreated, w.Code)
		groupID := jsonBody[map[string]any](t, w.Body.Bytes())["id"].(string)
		w = executeRequest("POST", "/v1/coach-groups/"+groupID+"/invitations", map[string]any{"user_id": member.ID, "role": "coach"}, coachA.Token)
		require.Equal(t, http.StatusCreated, w.Code)
		inv := jsonBody[map[string]any](t, w.Body.Bytes())["id"].(string)
		require.Equal(t, http.StatusOK, executeRequest("POST", "/v1/coach-invitations/"+inv+"/accept", nil, member.Token).Code)

		assert.Equal(t, http.StatusForbidden, executeRequest("POST", "/v1/courses", courseBody(sportID, nil), member.Token).Code)
		out := createCourseOK(t, member.Token, courseBody(sportID, map[string]any{"group_id": groupID, "coach_ids": []string{member.ID, coachA.ID}}))
		assert.Equal(t, "group", out["owner_type"])
		assert.Equal(t, "Closed Group", out["owner_name"])
		assert.Len(t, out["teachers"], 2)

		// Allowing private courses unlocks individual publishing.
		require.Equal(t, http.StatusOK, executeRequest("PATCH", "/v1/coach-groups/"+groupID, map[string]any{"allow_private_courses": true}, coachA.Token).Code)
		createCourseOK(t, member.Token, courseBody(sportID, nil))

		// Outsiders and assistants cannot publish for the group.
		other := makeApprovedCoach(t, "other", admin, sportID)
		assert.Equal(t, http.StatusForbidden, executeRequest("POST", "/v1/courses", courseBody(sportID, map[string]any{"group_id": groupID}), other.Token).Code)
		assert.Equal(t, http.StatusBadRequest, executeRequest("POST", "/v1/courses", courseBody(sportID, map[string]any{"group_id": groupID, "coach_ids": []string{other.ID}}), member.Token).Code)
	})
}

func TestCourseEnrollmentFlow(t *testing.T) {
	clearTables()
	admin := newCoachUser(t, "admin", true)
	sportID := getSportID(t, "BADMINTON")
	coachA := makeApprovedCoach(t, "coacha", admin, sportID)
	student := newCoachUser(t, "student", false)
	student2 := newCoachUser(t, "student2", false)

	free := createCourseOK(t, coachA.Token, courseBody(sportID, map[string]any{"capacity": 1}))
	paid := createCourseOK(t, coachA.Token, paidBody(sportID, map[string]any{"title": "Paid class"}))
	freeID, paidID := free["id"].(string), paid["id"].(string)

	t.Run("public detail hides bank info and names", func(t *testing.T) {
		w := executeRequest("GET", "/v1/courses/"+paidID, nil, "")
		require.Equal(t, http.StatusOK, w.Code)
		got := jsonBody[map[string]any](t, w.Body.Bytes())
		assert.Nil(t, got["bank_account"])
		assert.Equal(t, true, got["bank_info_available"])
		assert.EqualValues(t, 1200, got["total_fee"])
		assert.Equal(t, false, got["is_free"])
		stats := got["participant_stats"].(map[string]any)
		assert.EqualValues(t, 0, stats["total"])
		assert.NotNil(t, stats["genders"])
		assert.NotContains(t, w.Body.String(), "student")

		w = executeRequest("GET", "/v1/courses/"+paidID, nil, coachA.Token)
		got = jsonBody[map[string]any](t, w.Body.Bytes())
		assert.Equal(t, "123456789012", got["bank_account"], "managers see bank details")
		assert.Equal(t, http.StatusNotFound, executeRequest("GET", "/v1/courses/"+uuid.NewString(), nil, "").Code)
	})

	t.Run("enroll rules", func(t *testing.T) {
		assert.Equal(t, http.StatusUnauthorized, executeRequest("POST", "/v1/courses/"+paidID+"/enrollments", nil, "").Code)
		assert.Equal(t, http.StatusForbidden, executeRequest("POST", "/v1/courses/"+paidID+"/enrollments", map[string]any{"payment_method": "transfer"}, coachA.Token).Code, "teachers cannot enroll")
		assert.Equal(t, http.StatusBadRequest, executeRequest("POST", "/v1/courses/"+paidID+"/enrollments", nil, student.Token).Code, "paid courses need a method")
		assert.Equal(t, http.StatusBadRequest, executeRequest("POST", "/v1/courses/"+paidID+"/enrollments", map[string]any{"payment_method": "card"}, student.Token).Code)

		w := executeRequest("POST", "/v1/courses/"+paidID+"/enrollments", map[string]any{"payment_method": "transfer"}, student.Token)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		e := jsonBody[map[string]any](t, w.Body.Bytes())
		assert.Equal(t, "active", e["status"])
		assert.Equal(t, "unpaid", e["payment_status"], "enrollment and payment are tracked separately")
		assert.EqualValues(t, 1200, e["amount_due"])
		assert.Equal(t, 1, countNotifications(t, coachA.ID, "course_enrolled"))
		assert.Equal(t, http.StatusConflict, executeRequest("POST", "/v1/courses/"+paidID+"/enrollments", map[string]any{"payment_method": "transfer"}, student.Token).Code)

		w = executeRequest("POST", "/v1/courses/"+freeID+"/enrollments", nil, student.Token)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		assert.Equal(t, "not_required", jsonBody[map[string]any](t, w.Body.Bytes())["payment_status"])
		assert.Equal(t, http.StatusConflict, executeRequest("POST", "/v1/courses/"+freeID+"/enrollments", nil, student2.Token).Code, "capacity 1 is full")
	})

	t.Run("price change does not touch existing enrollments", func(t *testing.T) {
		require.Equal(t, http.StatusOK, executeRequest("PATCH", "/v1/courses/"+paidID, map[string]any{"price": 2000}, coachA.Token).Code)
		assert.Equal(t, 1, countNotifications(t, student.ID, "course_updated"))
		w := executeRequest("GET", "/v1/enrollments", nil, student.Token)
		require.Equal(t, http.StatusOK, w.Code)
		page := jsonBody[map[string]any](t, w.Body.Bytes())
		assert.EqualValues(t, 2, page["total"])
		for _, it := range page["items"].([]any) {
			item := it.(map[string]any)
			if item["course_id"] == paidID {
				assert.EqualValues(t, 1200, item["amount_due"])
			}
		}
		w = executeRequest("POST", "/v1/courses/"+paidID+"/enrollments", map[string]any{"payment_method": "onsite"}, student2.Token)
		require.Equal(t, http.StatusCreated, w.Code)
		assert.EqualValues(t, 2200, jsonBody[map[string]any](t, w.Body.Bytes())["amount_due"])
	})

	t.Run("roster is for managers; students see own bank info", func(t *testing.T) {
		assert.Equal(t, http.StatusForbidden, executeRequest("GET", "/v1/courses/"+paidID+"/enrollments", nil, student.Token).Code)
		assert.Equal(t, http.StatusUnauthorized, executeRequest("GET", "/v1/courses/"+paidID+"/enrollments", nil, "").Code)
		w := executeRequest("GET", "/v1/courses/"+paidID+"/enrollments", nil, coachA.Token)
		require.Equal(t, http.StatusOK, w.Code)
		page := jsonBody[map[string]any](t, w.Body.Bytes())
		assert.EqualValues(t, 2, page["total"])
		assert.NotEmpty(t, page["items"].([]any)[0].(map[string]any)["student"].(map[string]any)["display_name"])
		assert.Equal(t, http.StatusOK, executeRequest("GET", "/v1/courses/"+paidID+"/enrollments", nil, admin.Token).Code)

		w = executeRequest("GET", "/v1/courses/"+paidID, nil, student.Token)
		got := jsonBody[map[string]any](t, w.Body.Bytes())
		assert.Equal(t, "123456789012", got["bank_account"])
		assert.NotNil(t, got["my_enrollment_id"])
		assert.EqualValues(t, 2, got["participant_stats"].(map[string]any)["total"])

		enrollID := got["my_enrollment_id"].(string)
		assert.Equal(t, http.StatusOK, executeRequest("GET", "/v1/enrollments/"+enrollID, nil, student.Token).Code)
		assert.Equal(t, http.StatusOK, executeRequest("GET", "/v1/enrollments/"+enrollID, nil, coachA.Token).Code)
		assert.Equal(t, http.StatusNotFound, executeRequest("GET", "/v1/enrollments/"+enrollID, nil, student2.Token).Code)
	})

	t.Run("listing and filters", func(t *testing.T) {
		w := executeRequest("GET", "/v1/courses?sport_id="+getSportID(t, "BADMINTON")+"&free_only=true", nil, "")
		page := jsonBody[map[string]any](t, w.Body.Bytes())
		assert.EqualValues(t, 1, page["total"])
		w = executeRequest("GET", "/v1/courses?has_seats=true", nil, "")
		assert.EqualValues(t, 1, jsonBody[map[string]any](t, w.Body.Bytes())["total"], "the free course is full")
		w = executeRequest("GET", "/v1/courses?keyword=paid", nil, "")
		assert.EqualValues(t, 1, jsonBody[map[string]any](t, w.Body.Bytes())["total"])
		w = executeRequest("GET", "/v1/courses?coach_id="+coachA.ID, nil, "")
		assert.EqualValues(t, 2, jsonBody[map[string]any](t, w.Body.Bytes())["total"])
		assert.Equal(t, http.StatusUnauthorized, executeRequest("GET", "/v1/courses?managed=true", nil, "").Code)
		w = executeRequest("GET", "/v1/courses?managed=true", nil, coachA.Token)
		assert.EqualValues(t, 2, jsonBody[map[string]any](t, w.Body.Bytes())["total"])
		w = executeRequest("GET", "/v1/courses?enrolled=true", nil, student.Token)
		assert.EqualValues(t, 2, jsonBody[map[string]any](t, w.Body.Bytes())["total"])
		w = executeRequest("GET", "/v1/courses?managed=true", nil, student.Token)
		assert.EqualValues(t, 0, jsonBody[map[string]any](t, w.Body.Bytes())["total"])
		assert.Equal(t, http.StatusBadRequest, executeRequest("GET", "/v1/courses?course_type=bad", nil, "").Code)
	})

	t.Run("update permissions, capacity floor, closing, and cancellation", func(t *testing.T) {
		assert.Equal(t, http.StatusForbidden, executeRequest("PATCH", "/v1/courses/"+paidID, map[string]any{"title": "x"}, student.Token).Code)
		assert.Equal(t, http.StatusBadRequest, executeRequest("PATCH", "/v1/courses/"+paidID, map[string]any{"status": "weird"}, coachA.Token).Code)
		assert.Equal(t, http.StatusConflict, executeRequest("PATCH", "/v1/courses/"+paidID, map[string]any{"capacity": 1}, coachA.Token).Code, "2 students enrolled")
		assert.Equal(t, http.StatusBadRequest, executeRequest("PATCH", "/v1/courses/"+paidID, map[string]any{"payment_methods": []string{"transfer"}, "bank_code": ""}, coachA.Token).Code)

		w := executeRequest("PATCH", "/v1/courses/"+paidID, map[string]any{"status": "closed", "rules_refund": "No refunds within 24h", "allow_leave": true, "leave_deadline_hours": 12}, coachA.Token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		got := jsonBody[map[string]any](t, w.Body.Bytes())
		assert.Equal(t, "closed", got["status"])
		assert.Equal(t, "No refunds within 24h", got["rules_refund"])
		assert.EqualValues(t, 12, got["leave_deadline_hours"])
		late := newCoachUser(t, "late", false)
		assert.Equal(t, http.StatusConflict, executeRequest("POST", "/v1/courses/"+paidID+"/enrollments", map[string]any{"payment_method": "onsite"}, late.Token).Code)
		w = executeRequest("PATCH", "/v1/courses/"+paidID, map[string]any{"leave_deadline_hours": nil, "status": "open"}, coachA.Token)
		assert.Nil(t, jsonBody[map[string]any](t, w.Body.Bytes())["leave_deadline_hours"])

		// Cancelling the course releases every seat and notifies students.
		w = executeRequest("PATCH", "/v1/courses/"+paidID, map[string]any{"status": "cancelled"}, coachA.Token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, "cancelled", jsonBody[map[string]any](t, w.Body.Bytes())["status"])
		assert.Equal(t, 1, countNotifications(t, student.ID, "course_cancelled"))
		assert.Equal(t, 1, countNotifications(t, student2.ID, "course_cancelled"))
		var active int
		require.NoError(t, testPool.QueryRow(context.Background(),
			"SELECT count(*) FROM public.enrollments WHERE course_id = $1 AND status = 'active'", paidID).Scan(&active))
		assert.Equal(t, 0, active)
		var live int
		require.NoError(t, testPool.QueryRow(context.Background(),
			"SELECT count(*) FROM public.course_sessions WHERE course_id = $1 AND status = 'scheduled'", paidID).Scan(&live))
		assert.Equal(t, 0, live)
		assert.Equal(t, http.StatusConflict, executeRequest("PATCH", "/v1/courses/"+paidID, map[string]any{"title": "again"}, coachA.Token).Code)
		assert.Equal(t, http.StatusConflict, executeRequest("POST", "/v1/courses/"+paidID+"/enrollments", map[string]any{"payment_method": "onsite"}, late.Token).Code)
		w = executeRequest("GET", "/v1/courses", nil, "")
		assert.EqualValues(t, 1, jsonBody[map[string]any](t, w.Body.Bytes())["total"], "cancelled courses leave the public list")
	})
}

func TestCourseSessionManagement(t *testing.T) {
	clearTables()
	admin := newCoachUser(t, "admin", true)
	sportID := getSportID(t, "BADMINTON")
	coachA := makeApprovedCoach(t, "coacha", admin, sportID)
	student := newCoachUser(t, "student", false)

	out := createCourseOK(t, coachA.Token, courseBody(sportID, map[string]any{
		"course_type": "recurring", "sessions": []map[string]any{futureSession(48), futureSession(120), futureSession(200)},
	}))
	id := out["id"].(string)
	sessions := out["sessions"].([]any)
	first, second := sessions[0].(map[string]any)["id"].(string), sessions[1].(map[string]any)["id"].(string)
	require.Equal(t, http.StatusCreated, executeRequest("POST", "/v1/courses/"+id+"/enrollments", nil, student.Token).Code)

	t.Run("add session", func(t *testing.T) {
		assert.Equal(t, http.StatusForbidden, executeRequest("POST", "/v1/courses/"+id+"/sessions", futureSession(300), student.Token).Code)
		assert.Equal(t, http.StatusConflict, executeRequest("POST", "/v1/courses/"+id+"/sessions", futureSession(48), coachA.Token).Code, "overlaps the first session")
		w := executeRequest("POST", "/v1/courses/"+id+"/sessions", futureSession(300), coachA.Token)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		assert.EqualValues(t, 4, jsonBody[map[string]any](t, w.Body.Bytes())["seq"])

		single := createCourseOK(t, coachA.Token, courseBody(sportID, nil))
		assert.Equal(t, http.StatusBadRequest, executeRequest("POST", "/v1/courses/"+single["id"].(string)+"/sessions", futureSession(300), coachA.Token).Code, "single courses have one session")
	})

	t.Run("reschedule and cancel notify students", func(t *testing.T) {
		path := "/v1/courses/" + id + "/sessions/" + second
		moved := futureSession(130)
		assert.Equal(t, http.StatusBadRequest, executeRequest("PATCH", path, map[string]any{"start_at": moved["start_at"]}, coachA.Token).Code, "start and end move together")
		assert.Equal(t, http.StatusBadRequest, executeRequest("PATCH", path, map[string]any{}, coachA.Token).Code)
		assert.Equal(t, http.StatusConflict, executeRequest("PATCH", path, futureSession(48), coachA.Token).Code, "cannot overlap session 1")
		assert.Equal(t, http.StatusForbidden, executeRequest("PATCH", path, moved, student.Token).Code)

		require.Equal(t, http.StatusOK, executeRequest("PATCH", path, moved, coachA.Token).Code)
		assert.Equal(t, 1, countNotifications(t, student.ID, "course_session_changed"))

		w := executeRequest("PATCH", "/v1/courses/"+id+"/sessions/"+first, map[string]any{"cancel": true, "reason": "coach is sick"}, coachA.Token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		got := jsonBody[map[string]any](t, w.Body.Bytes())
		assert.Equal(t, "cancelled", got["status"])
		assert.Equal(t, "coach is sick", got["cancel_reason"])
		assert.Equal(t, 1, countNotifications(t, student.ID, "course_session_cancelled"))
		assert.Equal(t, http.StatusConflict, executeRequest("PATCH", "/v1/courses/"+id+"/sessions/"+first, map[string]any{"venue_note": "x"}, coachA.Token).Code)
		assert.Equal(t, http.StatusNotFound, executeRequest("PATCH", "/v1/courses/"+id+"/sessions/"+uuid.NewString(), map[string]any{"cancel": true}, coachA.Token).Code)

		// A cancelled session frees its time slot.
		assert.Equal(t, http.StatusCreated, executeRequest("POST", "/v1/courses/"+id+"/sessions", futureSession(48), coachA.Token).Code)
	})

	t.Run("past sessions are frozen", func(t *testing.T) {
		var past string
		require.NoError(t, testPool.QueryRow(context.Background(), `INSERT INTO public.course_sessions (course_id, seq, start_at, end_at)
			VALUES ($1, 99, now() - interval '3 hours', now() - interval '2 hours') RETURNING id`, id).Scan(&past))
		assert.Equal(t, http.StatusConflict, executeRequest("PATCH", "/v1/courses/"+id+"/sessions/"+past, map[string]any{"cancel": true}, coachA.Token).Code)
	})
}

// Many students racing for the last seat can never overbook a course.
func TestCourseEnrollmentConcurrency(t *testing.T) {
	clearTables()
	admin := newCoachUser(t, "admin", true)
	sportID := getSportID(t, "BADMINTON")
	coachA := makeApprovedCoach(t, "coacha", admin, sportID)
	id := createCourseOK(t, coachA.Token, courseBody(sportID, map[string]any{"capacity": 3}))["id"].(string)

	const contenders = 10
	codes := make([]int, contenders)
	tokens := make([]string, contenders)
	for i := range tokens {
		tokens[i] = newCoachUser(t, fmt.Sprintf("racer%d", i), false).Token
	}
	var wg sync.WaitGroup
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i] = executeRequest("POST", "/v1/courses/"+id+"/enrollments", nil, tokens[i]).Code
		}()
	}
	wg.Wait()

	created, full := 0, 0
	for _, c := range codes {
		switch c {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			full++
		}
	}
	assert.Equal(t, 3, created)
	assert.Equal(t, contenders-3, full)
}

// A suspended coach's courses disappear from search and stop taking enrollments.
func TestCourseHiddenWhenCoachSuspended(t *testing.T) {
	clearTables()
	admin := newCoachUser(t, "admin", true)
	sportID := getSportID(t, "BADMINTON")
	coachA := makeApprovedCoach(t, "coacha", admin, sportID)
	student := newCoachUser(t, "student", false)
	id := createCourseOK(t, coachA.Token, courseBody(sportID, nil))["id"].(string)

	w := executeRequest("GET", "/v1/courses", nil, "")
	assert.EqualValues(t, 1, jsonBody[map[string]any](t, w.Body.Bytes())["total"])

	_, err := testPool.Exec(context.Background(), "UPDATE public.coaches SET status = 'suspended' WHERE user_id = $1", coachA.ID)
	require.NoError(t, err)
	w = executeRequest("GET", "/v1/courses", nil, "")
	assert.EqualValues(t, 0, jsonBody[map[string]any](t, w.Body.Bytes())["total"])
	assert.Equal(t, http.StatusConflict, executeRequest("POST", "/v1/courses/"+id+"/enrollments", nil, student.Token).Code)
	assert.Equal(t, http.StatusForbidden, executeRequest("POST", "/v1/courses", courseBody(sportID, nil), coachA.Token).Code)
}
