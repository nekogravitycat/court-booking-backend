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

// enrollOK enrolls the user and returns the enrollment id.
func enrollOK(t *testing.T, courseID string, u coachUser, method string) string {
	var body any
	if method != "" {
		body = map[string]any{"payment_method": method}
	}
	w := executeRequest("POST", "/v1/courses/"+courseID+"/enrollments", body, u.Token)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	return jsonBody[map[string]any](t, w.Body.Bytes())["id"].(string)
}

func paymentStatus(t *testing.T, enrollmentID string) string {
	var s string
	require.NoError(t, testPool.QueryRow(context.Background(), "SELECT payment_status FROM public.enrollments WHERE id = $1", enrollmentID).Scan(&s))
	return s
}

func TestTransferReportReview(t *testing.T) {
	clearTables()
	admin := newCoachUser(t, "admin", true)
	sportID := getSportID(t, "BADMINTON")
	coachA := makeApprovedCoach(t, "coacha", admin, sportID)
	coachB := makeApprovedCoach(t, "coachb", admin, sportID)
	student := newCoachUser(t, "student", false)
	other := newCoachUser(t, "other", false)

	courseID := createCourseOK(t, coachA.Token, paidBody(sportID, nil))["id"].(string)
	enrollID := enrollOK(t, courseID, student, "transfer")
	onsiteID := enrollOK(t, courseID, other, "onsite")
	path := "/v1/enrollments/" + enrollID + "/transfer-reports"

	t.Run("report rules", func(t *testing.T) {
		assert.Equal(t, http.StatusUnauthorized, executeRequest("POST", path, map[string]any{"last5": "12345"}, "").Code)
		for name, body := range map[string]map[string]any{
			"short":     {"last5": "1234"},
			"letters":   {"last5": "12a45"},
			"too long":  {"last5": "123456"},
			"zero":      {"last5": "12345", "amount": 0},
			"note long": {"last5": "12345", "note": string(make([]rune, 201))},
		} {
			assert.Equal(t, http.StatusBadRequest, executeRequest("POST", path, body, student.Token).Code, name)
		}
		assert.Equal(t, http.StatusNotFound, executeRequest("POST", path, map[string]any{"last5": "12345"}, other.Token).Code, "not your enrollment")
		assert.Equal(t, http.StatusNotFound, executeRequest("POST", path, map[string]any{"last5": "12345"}, coachA.Token).Code, "coaches cannot report for students")
		assert.Equal(t, http.StatusConflict, executeRequest("POST", "/v1/enrollments/"+onsiteID+"/transfer-reports", map[string]any{"last5": "12345"}, other.Token).Code, "onsite enrollments do not transfer")
	})

	var reportID string
	t.Run("an unreviewed report never marks the enrollment paid", func(t *testing.T) {
		w := executeRequest("POST", path, map[string]any{"last5": "12345", "note": "from my bank app"}, student.Token)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		got := jsonBody[map[string]any](t, w.Body.Bytes())
		reportID = got["id"].(string)
		assert.EqualValues(t, 1200, got["amount"], "defaults to course fee plus venue fee")
		assert.Equal(t, true, got["amount_matches"])
		assert.Equal(t, "pending", got["status"])
		assert.Equal(t, "reported", paymentStatus(t, enrollID))
		assert.Equal(t, 1, countNotifications(t, coachA.ID, "transfer_reported"))
		assert.Equal(t, http.StatusConflict, executeRequest("POST", path, map[string]any{"last5": "99999"}, student.Token).Code, "one pending report at a time")
	})

	t.Run("only the course's managers (or admins) can review", func(t *testing.T) {
		review := "/v1/transfer-reports/" + reportID + "/review"
		assert.Equal(t, http.StatusNotFound, executeRequest("POST", review, map[string]any{"decision": "approve"}, student.Token).Code)
		assert.Equal(t, http.StatusNotFound, executeRequest("POST", review, map[string]any{"decision": "approve"}, coachB.Token).Code)
		assert.Equal(t, http.StatusBadRequest, executeRequest("POST", review, map[string]any{"decision": "maybe"}, coachA.Token).Code)
		assert.Equal(t, http.StatusBadRequest, executeRequest("POST", review, map[string]any{"decision": "reject"}, coachA.Token).Code, "rejecting needs a note")

		// Pending list: coach sees their own, other coach sees none, admin sees all.
		list := func(token string) float64 {
			w := executeRequest("GET", "/v1/transfer-reports?status=pending", nil, token)
			require.Equal(t, http.StatusOK, w.Code)
			return jsonBody[map[string]any](t, w.Body.Bytes())["total"].(float64)
		}
		assert.EqualValues(t, 1, list(coachA.Token))
		assert.EqualValues(t, 0, list(coachB.Token))
		assert.EqualValues(t, 1, list(admin.Token))
		assert.EqualValues(t, 0, list(student.Token))
	})

	t.Run("reject returns to unpaid and allows a new report", func(t *testing.T) {
		review := "/v1/transfer-reports/" + reportID + "/review"
		w := executeRequest("POST", review, map[string]any{"decision": "reject", "note": "no matching deposit"}, coachA.Token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, "rejected", jsonBody[map[string]any](t, w.Body.Bytes())["status"])
		assert.Equal(t, "unpaid", paymentStatus(t, enrollID))
		assert.Equal(t, 1, countNotifications(t, student.ID, "transfer_rejected"))
		assert.Equal(t, http.StatusConflict, executeRequest("POST", review, map[string]any{"decision": "approve"}, coachA.Token).Code, "already reviewed")

		w = executeRequest("POST", path, map[string]any{"last5": "54321", "amount": 1200}, student.Token)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		reportID = jsonBody[map[string]any](t, w.Body.Bytes())["id"].(string)
	})

	t.Run("approve marks the enrollment paid", func(t *testing.T) {
		// Admins can review as an oversight path.
		w := executeRequest("POST", "/v1/transfer-reports/"+reportID+"/review", map[string]any{"decision": "approve"}, admin.Token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, "paid", paymentStatus(t, enrollID))
		assert.Equal(t, 1, countNotifications(t, student.ID, "transfer_approved"))
		assert.Equal(t, http.StatusConflict, executeRequest("POST", path, map[string]any{"last5": "11111"}, student.Token).Code, "already paid")

		w = executeRequest("GET", path, nil, student.Token)
		assert.EqualValues(t, 2, jsonBody[map[string]any](t, w.Body.Bytes())["total"])
		assert.Equal(t, http.StatusOK, executeRequest("GET", path, nil, coachA.Token).Code)
		assert.Equal(t, http.StatusNotFound, executeRequest("GET", path, nil, other.Token).Code)
	})
}

func TestManualPaymentMarking(t *testing.T) {
	clearTables()
	admin := newCoachUser(t, "admin", true)
	sportID := getSportID(t, "BADMINTON")
	coachA := makeApprovedCoach(t, "coacha", admin, sportID)
	student := newCoachUser(t, "student", false)
	freeStudent := newCoachUser(t, "freestudent", false)

	paidID := createCourseOK(t, coachA.Token, paidBody(sportID, nil))["id"].(string)
	freeID := createCourseOK(t, coachA.Token, courseBody(sportID, nil))["id"].(string)
	enrollID := enrollOK(t, paidID, student, "onsite")
	freeEnroll := enrollOK(t, freeID, freeStudent, "")
	path := "/v1/enrollments/" + enrollID + "/payment"

	assert.Equal(t, http.StatusNotFound, executeRequest("PATCH", "/v1/enrollments/"+enrollID+"/payment", map[string]any{"payment_status": "paid"}, newCoachUser(t, "stranger", false).Token).Code)
	assert.Equal(t, http.StatusForbidden, executeRequest("PATCH", path, map[string]any{"payment_status": "paid"}, student.Token).Code, "students cannot mark themselves paid")
	assert.Equal(t, http.StatusBadRequest, executeRequest("PATCH", path, map[string]any{"payment_status": "refunded"}, coachA.Token).Code)
	assert.Equal(t, http.StatusConflict, executeRequest("PATCH", "/v1/enrollments/"+freeEnroll+"/payment", map[string]any{"payment_status": "paid"}, coachA.Token).Code, "free enrollments need no payment")
	assert.Equal(t, http.StatusConflict, executeRequest("PATCH", path, map[string]any{"payment_status": "unpaid"}, coachA.Token).Code, "already unpaid")

	w := executeRequest("PATCH", path, map[string]any{"payment_status": "paid"}, coachA.Token)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "paid", jsonBody[map[string]any](t, w.Body.Bytes())["payment_status"])
	assert.Equal(t, 1, countNotifications(t, student.ID, "payment_updated"))
	assert.Equal(t, http.StatusOK, executeRequest("PATCH", path, map[string]any{"payment_status": "unpaid"}, coachA.Token).Code, "mistakes can be undone")

	// Marking paid settles a transfer report that is still waiting for review.
	reporter := newCoachUser(t, "transferer", false)
	reportEnroll := enrollOK(t, paidID, reporter, "transfer")
	w = executeRequest("POST", "/v1/enrollments/"+reportEnroll+"/transfer-reports", map[string]any{"last5": "12345"}, reporter.Token)
	require.Equal(t, http.StatusCreated, w.Code)
	reportID := jsonBody[map[string]any](t, w.Body.Bytes())["id"].(string)
	assert.Equal(t, http.StatusConflict, executeRequest("PATCH", "/v1/enrollments/"+reportEnroll+"/payment", map[string]any{"payment_status": "unpaid"}, coachA.Token).Code, "a reported enrollment is reviewed, not reset")
	require.Equal(t, http.StatusOK, executeRequest("PATCH", "/v1/enrollments/"+reportEnroll+"/payment", map[string]any{"payment_status": "paid"}, coachA.Token).Code)
	var status string
	require.NoError(t, testPool.QueryRow(context.Background(), "SELECT status FROM public.transfer_reports WHERE id = $1", reportID).Scan(&status))
	assert.Equal(t, "approved", status)
}

// Two reviewers approving the same report at once must settle it exactly once.
func TestTransferReportConcurrentReview(t *testing.T) {
	clearTables()
	admin := newCoachUser(t, "admin", true)
	sportID := getSportID(t, "BADMINTON")
	coachA := makeApprovedCoach(t, "coacha", admin, sportID)
	student := newCoachUser(t, "student", false)
	courseID := createCourseOK(t, coachA.Token, paidBody(sportID, nil))["id"].(string)
	enrollID := enrollOK(t, courseID, student, "transfer")
	w := executeRequest("POST", "/v1/enrollments/"+enrollID+"/transfer-reports", map[string]any{"last5": "12345"}, student.Token)
	require.Equal(t, http.StatusCreated, w.Code)
	reportID := jsonBody[map[string]any](t, w.Body.Bytes())["id"].(string)

	tokens := []string{coachA.Token, admin.Token, coachA.Token, admin.Token}
	codes := make([]int, len(tokens))
	var wg sync.WaitGroup
	for i := range tokens {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i] = executeRequest("POST", "/v1/transfer-reports/"+reportID+"/review", map[string]any{"decision": "approve"}, tokens[i]).Code
		}()
	}
	wg.Wait()
	ok, conflict := 0, 0
	for _, c := range codes {
		switch c {
		case http.StatusOK:
			ok++
		case http.StatusConflict:
			conflict++
		}
	}
	assert.Equal(t, 1, ok)
	assert.Equal(t, 3, conflict)
	assert.Equal(t, 1, countNotifications(t, student.ID, "transfer_approved"))
}

func TestCancellationRequests(t *testing.T) {
	clearTables()
	admin := newCoachUser(t, "admin", true)
	sportID := getSportID(t, "BADMINTON")
	coachA := makeApprovedCoach(t, "coacha", admin, sportID)
	student := newCoachUser(t, "student", false)
	paidStudent := newCoachUser(t, "paidstudent", false)
	stranger := newCoachUser(t, "stranger", false)

	t.Run("self-service cancel only when the rules allow it and no money moved", func(t *testing.T) {
		locked := createCourseOK(t, coachA.Token, courseBody(sportID, nil))["id"].(string)
		enrollID := enrollOK(t, locked, student, "")
		path := "/v1/enrollments/" + enrollID + "/cancellation-requests"
		assert.Equal(t, http.StatusBadRequest, executeRequest("POST", path, map[string]any{"reason": ""}, student.Token).Code)
		assert.Equal(t, http.StatusNotFound, executeRequest("POST", path, map[string]any{"reason": "x"}, stranger.Token).Code)

		// allow_cancel is false: the request waits for the coach, nothing is automatic.
		w := executeRequest("POST", path, map[string]any{"reason": "schedule clash"}, student.Token)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		got := jsonBody[map[string]any](t, w.Body.Bytes())
		assert.Equal(t, "pending", got["status"])
		assert.Equal(t, "student", got["requester_role"])
		assert.Equal(t, "active", got["enrollment_status"])
		assert.Equal(t, 1, countNotifications(t, coachA.ID, "cancellation_requested"))
		assert.Equal(t, http.StatusConflict, executeRequest("POST", path, map[string]any{"reason": "again"}, student.Token).Code)

		// Rules that allow cancelling cancel instantly for unpaid enrollments.
		open := createCourseOK(t, coachA.Token, courseBody(sportID, map[string]any{"allow_cancel": true, "cancel_deadline_hours": 24}))["id"].(string)
		openEnroll := enrollOK(t, open, student, "")
		w = executeRequest("POST", "/v1/enrollments/"+openEnroll+"/cancellation-requests", map[string]any{"reason": "changed my mind"}, student.Token)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		got = jsonBody[map[string]any](t, w.Body.Bytes())
		assert.Equal(t, "resolved", got["status"])
		assert.Equal(t, "cancelled", got["resolution"])
		assert.Equal(t, "cancelled", got["enrollment_status"])
		assert.Equal(t, 1, countNotifications(t, coachA.ID, "enrollment_cancelled"))
		// The seat is free again and the student can re-enroll.
		enrollOK(t, open, student, "")

		// Inside the deadline window (session in 2h, deadline 24h) it needs the coach.
		soon := createCourseOK(t, coachA.Token, courseBody(sportID, map[string]any{
			"allow_cancel": true, "cancel_deadline_hours": 24, "sessions": []map[string]any{futureSession(2)}}))["id"].(string)
		soonEnroll := enrollOK(t, soon, student, "")
		w = executeRequest("POST", "/v1/enrollments/"+soonEnroll+"/cancellation-requests", map[string]any{"reason": "late"}, student.Token)
		require.Equal(t, http.StatusCreated, w.Code)
		assert.Equal(t, "pending", jsonBody[map[string]any](t, w.Body.Bytes())["status"])
	})

	t.Run("paid enrollments go through the coach; refunds are recorded", func(t *testing.T) {
		courseID := createCourseOK(t, coachA.Token, paidBody(sportID, map[string]any{"allow_cancel": true}))["id"].(string)
		enrollID := enrollOK(t, courseID, paidStudent, "onsite")
		require.Equal(t, http.StatusOK, executeRequest("PATCH", "/v1/enrollments/"+enrollID+"/payment", map[string]any{"payment_status": "paid"}, coachA.Token).Code)

		w := executeRequest("POST", "/v1/enrollments/"+enrollID+"/cancellation-requests", map[string]any{"reason": "moving abroad"}, paidStudent.Token)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		got := jsonBody[map[string]any](t, w.Body.Bytes())
		assert.Equal(t, "pending", got["status"], "paid enrollments are never cancelled automatically")
		reqID := got["id"].(string)
		resolve := "/v1/cancellation-requests/" + reqID + "/resolve"

		assert.Equal(t, http.StatusForbidden, executeRequest("POST", resolve, map[string]any{"resolution": "refunded", "refund_amount": 500}, paidStudent.Token).Code, "students cannot resolve their own request")
		assert.Equal(t, http.StatusNotFound, executeRequest("POST", resolve, map[string]any{"resolution": "refunded", "refund_amount": 500}, stranger.Token).Code)
		assert.Equal(t, http.StatusBadRequest, executeRequest("POST", resolve, map[string]any{"resolution": "gone"}, coachA.Token).Code)
		assert.Equal(t, http.StatusBadRequest, executeRequest("POST", resolve, map[string]any{"resolution": "refunded"}, coachA.Token).Code, "a refund needs an amount")
		assert.Equal(t, http.StatusBadRequest, executeRequest("POST", resolve, map[string]any{"resolution": "refunded", "refund_amount": 9999}, coachA.Token).Code, "cannot refund more than was due")
		assert.Equal(t, http.StatusBadRequest, executeRequest("POST", resolve, map[string]any{"resolution": "cancelled", "refund_amount": 100}, coachA.Token).Code)

		// Lists: the student sees their own, the coach sees the course's, outsiders none.
		count := func(token string) float64 {
			w := executeRequest("GET", "/v1/cancellation-requests?status=pending&course_id="+courseID, nil, token)
			require.Equal(t, http.StatusOK, w.Code)
			return jsonBody[map[string]any](t, w.Body.Bytes())["total"].(float64)
		}
		assert.EqualValues(t, 1, count(paidStudent.Token))
		assert.EqualValues(t, 1, count(coachA.Token))
		assert.EqualValues(t, 1, count(admin.Token))
		assert.EqualValues(t, 0, count(stranger.Token))

		w = executeRequest("POST", resolve, map[string]any{"resolution": "refunded", "refund_amount": 1000, "note": "refunded by bank transfer"}, coachA.Token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		got = jsonBody[map[string]any](t, w.Body.Bytes())
		assert.Equal(t, "resolved", got["status"])
		assert.EqualValues(t, 1000, got["refund_amount"])
		assert.Equal(t, "cancelled", got["enrollment_status"])
		assert.Equal(t, "refunded", got["payment_status"])
		assert.Equal(t, 1, countNotifications(t, paidStudent.ID, "cancellation_resolved"))
		assert.Equal(t, http.StatusConflict, executeRequest("POST", resolve, map[string]any{"resolution": "rejected"}, coachA.Token).Code)
		assert.Equal(t, http.StatusConflict, executeRequest("POST", "/v1/enrollments/"+enrollID+"/cancellation-requests", map[string]any{"reason": "x"}, paidStudent.Token).Code, "enrollment already cancelled")
	})

	t.Run("coach no-show: session-scoped request records the outcome without cancelling", func(t *testing.T) {
		out := createCourseOK(t, coachA.Token, paidBody(sportID, map[string]any{
			"course_type": "recurring", "sessions": []map[string]any{futureSession(48), futureSession(120)}}))
		courseID := out["id"].(string)
		sessionID := out["sessions"].([]any)[0].(map[string]any)["id"].(string)
		enrollID := enrollOK(t, courseID, student, "onsite")
		require.Equal(t, http.StatusOK, executeRequest("PATCH", "/v1/enrollments/"+enrollID+"/payment", map[string]any{"payment_status": "paid"}, coachA.Token).Code)

		path := "/v1/enrollments/" + enrollID + "/cancellation-requests"
		assert.Equal(t, http.StatusBadRequest, executeRequest("POST", path, map[string]any{"reason": "no-show", "session_id": uuid.NewString()}, student.Token).Code)
		w := executeRequest("POST", path, map[string]any{"reason": "coach did not show up", "session_id": sessionID}, student.Token)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		reqID := jsonBody[map[string]any](t, w.Body.Bytes())["id"].(string)

		w = executeRequest("POST", "/v1/cancellation-requests/"+reqID+"/resolve", map[string]any{"resolution": "makeup", "note": "extra class next week"}, coachA.Token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		got := jsonBody[map[string]any](t, w.Body.Bytes())
		assert.Equal(t, "makeup", got["resolution"])
		assert.Equal(t, "active", got["enrollment_status"])
		assert.Equal(t, "paid", got["payment_status"])
	})

	t.Run("coach-initiated cancellation", func(t *testing.T) {
		courseID := createCourseOK(t, coachA.Token, courseBody(sportID, nil))["id"].(string)
		enrollID := enrollOK(t, courseID, stranger, "")
		w := executeRequest("POST", "/v1/enrollments/"+enrollID+"/cancellation-requests", map[string]any{"reason": "class merged"}, coachA.Token)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		got := jsonBody[map[string]any](t, w.Body.Bytes())
		assert.Equal(t, "coach", got["requester_role"])
		assert.Equal(t, "pending", got["status"])
		assert.Equal(t, 1, countNotifications(t, stranger.ID, "cancellation_requested"))
		w = executeRequest("POST", "/v1/cancellation-requests/"+got["id"].(string)+"/resolve", map[string]any{"resolution": "cancelled"}, coachA.Token)
		require.Equal(t, http.StatusOK, w.Code)
		var source string
		require.NoError(t, testPool.QueryRow(context.Background(), "SELECT cancel_source FROM public.enrollments WHERE id = $1", enrollID).Scan(&source))
		assert.Equal(t, "coach", source)
	})
}

// Cancelling a whole course leaves the coach a refund to-do list and discards
// transfer reports that can no longer be reviewed.
func TestCourseCancellationCreatesRefundTodos(t *testing.T) {
	clearTables()
	admin := newCoachUser(t, "admin", true)
	sportID := getSportID(t, "BADMINTON")
	coachA := makeApprovedCoach(t, "coacha", admin, sportID)
	paid := newCoachUser(t, "paid", false)
	reporter := newCoachUser(t, "reporter", false)
	unpaid := newCoachUser(t, "unpaid", false)

	courseID := createCourseOK(t, coachA.Token, paidBody(sportID, nil))["id"].(string)
	paidEnroll := enrollOK(t, courseID, paid, "onsite")
	reportEnroll := enrollOK(t, courseID, reporter, "transfer")
	enrollOK(t, courseID, unpaid, "onsite")
	require.Equal(t, http.StatusOK, executeRequest("PATCH", "/v1/enrollments/"+paidEnroll+"/payment", map[string]any{"payment_status": "paid"}, coachA.Token).Code)
	require.Equal(t, http.StatusCreated, executeRequest("POST", "/v1/enrollments/"+reportEnroll+"/transfer-reports", map[string]any{"last5": "12345"}, reporter.Token).Code)

	require.Equal(t, http.StatusOK, executeRequest("PATCH", "/v1/courses/"+courseID, map[string]any{"status": "cancelled"}, coachA.Token).Code)

	w := executeRequest("GET", "/v1/cancellation-requests?status=pending&course_id="+courseID, nil, coachA.Token)
	require.Equal(t, http.StatusOK, w.Code)
	page := jsonBody[map[string]any](t, w.Body.Bytes())
	assert.EqualValues(t, 2, page["total"], "paid and reported enrollments need a refund decision; the unpaid one does not")

	var rejected int
	require.NoError(t, testPool.QueryRow(context.Background(),
		"SELECT count(*) FROM public.transfer_reports WHERE status = 'rejected' AND review_note = 'course cancelled'").Scan(&rejected))
	assert.Equal(t, 1, rejected)

	// The coach records the refund on the already-cancelled enrollment.
	var reqID string
	for _, it := range page["items"].([]any) {
		if it.(map[string]any)["enrollment_id"] == paidEnroll {
			reqID = it.(map[string]any)["id"].(string)
		}
	}
	require.NotEmpty(t, reqID)
	w = executeRequest("POST", "/v1/cancellation-requests/"+reqID+"/resolve", map[string]any{"resolution": "refunded", "refund_amount": 1200}, coachA.Token)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "refunded", paymentStatus(t, paidEnroll))
}
