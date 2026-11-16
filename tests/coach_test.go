package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// coachUser is a test account together with its auth token.
type coachUser struct {
	ID    string
	Token string
}

func newCoachUser(t *testing.T, label string, isAdmin bool) coachUser {
	u := createTestUser(t, fmt.Sprintf("%s-%d@coach.com", label, time.Now().UnixNano()), "pass", isAdmin)
	return coachUser{ID: u.ID, Token: generateToken(u.ID)}
}

// insertTestFile inserts a file row owned by the user and returns its id.
func insertTestFile(t *testing.T, ownerID, contentType string) string {
	var id string
	require.NoError(t, testPool.QueryRow(context.Background(),
		`INSERT INTO public.files (user_id, filename, storage_path, content_type, size)
		 VALUES ($1, 'f', $2, $3, 1) RETURNING id`, ownerID, "coach-test-"+uuid.NewString(), contentType).Scan(&id))
	return id
}

func jsonBody[T any](t *testing.T, body []byte) T {
	var v T
	require.NoError(t, json.Unmarshal(body, &v), string(body))
	return v
}

func coachApplyBody(sportID string, extra map[string]any) map[string]any {
	body := map[string]any{
		"bio": "Ten years of badminton coaching", "teaching_content": "Footwork and smashes",
		"target_audience": "Beginners and intermediate", "fee_info": "NT$800 per hour",
		"sport_ids": []string{sportID},
	}
	for k, v := range extra {
		body[k] = v
	}
	return body
}

// makeApprovedCoach registers a user, applies, and has the admin approve.
func makeApprovedCoach(t *testing.T, label string, admin coachUser, sportID string) coachUser {
	u := newCoachUser(t, label, false)
	w := executeRequest("POST", "/v1/coach-applications", coachApplyBody(sportID, nil), u.Token)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	w = executeRequest("POST", "/v1/admin/coach-applications/"+u.ID+"/review", map[string]any{"decision": "approve"}, admin.Token)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	return u
}

func countNotifications(t *testing.T, userID, typ string) int {
	var n int
	require.NoError(t, testPool.QueryRow(context.Background(),
		"SELECT count(*) FROM public.notifications WHERE user_id = $1 AND type = $2", userID, typ).Scan(&n))
	return n
}

func TestCoachApplicationWorkflow(t *testing.T) {
	clearTables()
	admin := newCoachUser(t, "admin", true)
	applicant := newCoachUser(t, "applicant", false)
	other := newCoachUser(t, "other", false)
	sportID, _ := getSportSkill(t, "BADMINTON", "A")

	t.Run("validation", func(t *testing.T) {
		assert.Equal(t, http.StatusUnauthorized, executeRequest("POST", "/v1/coach-applications", coachApplyBody(sportID, nil), "").Code)
		for name, body := range map[string]map[string]any{
			"missing bio":   coachApplyBody(sportID, map[string]any{"bio": ""}),
			"blank fee":     coachApplyBody(sportID, map[string]any{"fee_info": "   "}),
			"no sports":     coachApplyBody(sportID, map[string]any{"sport_ids": []string{}}),
			"unknown sport": coachApplyBody(sportID, map[string]any{"sport_ids": []string{uuid.NewString()}}),
			"bad sport id":  coachApplyBody(sportID, map[string]any{"sport_ids": []string{"x"}}),
			"bio too long":  coachApplyBody(sportID, map[string]any{"bio": string(make([]rune, 1001))}),
			"unknown file":  coachApplyBody(sportID, map[string]any{"photo_file_id": uuid.NewString()}),
			"cert no title": coachApplyBody(sportID, map[string]any{"certificates": []map[string]any{{"file_id": uuid.NewString(), "title": ""}}}),
		} {
			w := executeRequest("POST", "/v1/coach-applications", body, applicant.Token)
			assert.Equal(t, http.StatusBadRequest, w.Code, name+": "+w.Body.String())
		}
		// Someone else's file cannot be attached.
		foreign := insertTestFile(t, other.ID, "image/jpeg")
		w := executeRequest("POST", "/v1/coach-applications", coachApplyBody(sportID, map[string]any{"photo_file_id": foreign}), applicant.Token)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		// A PDF is not a valid profile photo, but is a valid certificate.
		pdf := insertTestFile(t, applicant.ID, "application/pdf")
		w = executeRequest("POST", "/v1/coach-applications", coachApplyBody(sportID, map[string]any{"photo_file_id": pdf}), applicant.Token)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		text := insertTestFile(t, applicant.ID, "text/plain")
		w = executeRequest("POST", "/v1/coach-applications", coachApplyBody(sportID, map[string]any{
			"certificates": []map[string]any{{"file_id": text, "title": "x"}}}), applicant.Token)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	photo := insertTestFile(t, applicant.ID, "image/jpeg")
	cert := insertTestFile(t, applicant.ID, "application/pdf")
	t.Run("apply, duplicate, and admin notification", func(t *testing.T) {
		w := executeRequest("POST", "/v1/coach-applications", coachApplyBody(sportID, map[string]any{
			"photo_file_id": photo, "certificates": []map[string]any{{"file_id": cert, "title": "Level 2"}},
		}), applicant.Token)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		got := jsonBody[map[string]any](t, w.Body.Bytes())
		assert.Equal(t, "pending", got["status"])
		assert.Equal(t, true, got["certificate_provided"])
		assert.Equal(t, applicant.ID, got["id"])

		assert.Equal(t, http.StatusConflict, executeRequest("POST", "/v1/coach-applications", coachApplyBody(sportID, nil), applicant.Token).Code)
		assert.Equal(t, 1, countNotifications(t, admin.ID, "coach_application_submitted"))
	})

	t.Run("pending profile is private to owner and admin", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound, executeRequest("GET", "/v1/coaches/"+applicant.ID, nil, "").Code)
		assert.Equal(t, http.StatusNotFound, executeRequest("GET", "/v1/coaches/"+applicant.ID, nil, other.Token).Code)
		assert.Equal(t, http.StatusOK, executeRequest("GET", "/v1/coaches/"+applicant.ID, nil, applicant.Token).Code)
		assert.Equal(t, http.StatusOK, executeRequest("GET", "/v1/coaches/"+applicant.ID, nil, admin.Token).Code)
		assert.Equal(t, http.StatusOK, executeRequest("GET", "/v1/coaches/me", nil, applicant.Token).Code)
		assert.Equal(t, http.StatusNotFound, executeRequest("GET", "/v1/coaches/me", nil, other.Token).Code)

		w := executeRequest("GET", "/v1/coaches", nil, "")
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, float64(0), jsonBody[map[string]any](t, w.Body.Bytes())["total"])
	})

	t.Run("review permissions and rules", func(t *testing.T) {
		path := "/v1/admin/coach-applications/" + applicant.ID + "/review"
		assert.Equal(t, http.StatusForbidden, executeRequest("POST", path, map[string]any{"decision": "approve"}, applicant.Token).Code)
		assert.Equal(t, http.StatusBadRequest, executeRequest("POST", path, map[string]any{"decision": "maybe"}, admin.Token).Code)
		assert.Equal(t, http.StatusBadRequest, executeRequest("POST", path, map[string]any{"decision": "reject"}, admin.Token).Code)
		assert.Equal(t, http.StatusNotFound, executeRequest("POST", "/v1/admin/coach-applications/"+uuid.NewString()+"/review", map[string]any{"decision": "approve"}, admin.Token).Code)

		w := executeRequest("GET", "/v1/admin/coach-applications", nil, admin.Token)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, float64(1), jsonBody[map[string]any](t, w.Body.Bytes())["total"])
		assert.Equal(t, http.StatusForbidden, executeRequest("GET", "/v1/admin/coach-applications", nil, applicant.Token).Code)

		// Reject, then re-apply, then approve.
		w = executeRequest("POST", path, map[string]any{"decision": "reject", "reason": "need more detail"}, admin.Token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, "need more detail", jsonBody[map[string]any](t, w.Body.Bytes())["reject_reason"])
		assert.Equal(t, 1, countNotifications(t, applicant.ID, "coach_application_rejected"))
		assert.Equal(t, http.StatusConflict, executeRequest("POST", path, map[string]any{"decision": "approve"}, admin.Token).Code)
		assert.Equal(t, http.StatusForbidden, executeRequest("PATCH", "/v1/coaches/me", map[string]any{"bio": "x"}, applicant.Token).Code)

		w = executeRequest("POST", "/v1/coach-applications", coachApplyBody(sportID, map[string]any{"bio": "Updated bio"}), applicant.Token)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		got := jsonBody[map[string]any](t, w.Body.Bytes())
		assert.Equal(t, "pending", got["status"])
		assert.Nil(t, got["reject_reason"])
		assert.Equal(t, false, got["certificate_provided"], "re-application replaces the certificate list")
		// Files dropped by the re-application are released.
		var n int
		require.NoError(t, testPool.QueryRow(context.Background(), "SELECT count(*) FROM public.files WHERE id IN ($1, $2)", photo, cert).Scan(&n))
		assert.Equal(t, 0, n)

		w = executeRequest("POST", path, map[string]any{"decision": "approve"}, admin.Token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, 1, countNotifications(t, applicant.ID, "coach_application_approved"))
	})

	t.Run("approved profile is public and editable", func(t *testing.T) {
		w := executeRequest("GET", "/v1/coaches/"+applicant.ID, nil, "")
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "Updated bio", jsonBody[map[string]any](t, w.Body.Bytes())["bio"])
		w = executeRequest("GET", "/v1/coaches?sport_id="+sportID, nil, "")
		assert.Equal(t, float64(1), jsonBody[map[string]any](t, w.Body.Bytes())["total"])
		other2, _ := getSportSkill(t, "BASKETBALL", "A")
		w = executeRequest("GET", "/v1/coaches?sport_id="+other2, nil, "")
		assert.Equal(t, float64(0), jsonBody[map[string]any](t, w.Body.Bytes())["total"])
		assert.Equal(t, http.StatusBadRequest, executeRequest("GET", "/v1/coaches?sport_id=nope", nil, "").Code)

		assert.Equal(t, http.StatusBadRequest, executeRequest("PATCH", "/v1/coaches/me", map[string]any{}, applicant.Token).Code)
		assert.Equal(t, http.StatusBadRequest, executeRequest("PATCH", "/v1/coaches/me", map[string]any{"bio": ""}, applicant.Token).Code)
		newPhoto := insertTestFile(t, applicant.ID, "image/png")
		w = executeRequest("PATCH", "/v1/coaches/me", map[string]any{
			"fee_info": "NT$900", "photo_file_id": newPhoto, "sport_ids": []string{sportID, getSportID(t, "BASKETBALL")},
		}, applicant.Token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		got := jsonBody[map[string]any](t, w.Body.Bytes())
		assert.Equal(t, "NT$900", got["fee_info"])
		assert.Len(t, got["sports"], 2)
		assert.NotNil(t, got["photo"])

		w = executeRequest("PATCH", "/v1/coaches/me", map[string]any{"photo_file_id": nil}, applicant.Token)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Nil(t, jsonBody[map[string]any](t, w.Body.Bytes())["photo"])

		_, err := testPool.Exec(context.Background(), "UPDATE public.coaches SET status = 'suspended' WHERE user_id = $1", applicant.ID)
		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, executeRequest("PATCH", "/v1/coaches/me", map[string]any{"bio": "x"}, applicant.Token).Code)
		assert.Equal(t, http.StatusNotFound, executeRequest("GET", "/v1/coaches/"+applicant.ID, nil, "").Code)
	})
}

func getSportID(t *testing.T, code string) string {
	var id string
	require.NoError(t, testPool.QueryRow(context.Background(), "SELECT id FROM public.sports WHERE code = $1", code).Scan(&id))
	return id
}

func TestCoachGroups(t *testing.T) {
	clearTables()
	admin := newCoachUser(t, "admin", true)
	sportID := getSportID(t, "BADMINTON")
	leader := makeApprovedCoach(t, "leader", admin, sportID)
	member := makeApprovedCoach(t, "member", admin, sportID)
	third := makeApprovedCoach(t, "third", admin, sportID)
	outsider := newCoachUser(t, "outsider", false)

	var groupID string
	t.Run("create requires an approved coach", func(t *testing.T) {
		body := map[string]any{"name": "Dream Team", "allow_private_courses": true}
		assert.Equal(t, http.StatusForbidden, executeRequest("POST", "/v1/coach-groups", body, outsider.Token).Code)
		assert.Equal(t, http.StatusBadRequest, executeRequest("POST", "/v1/coach-groups", map[string]any{"name": " "}, leader.Token).Code)
		w := executeRequest("POST", "/v1/coach-groups", body, leader.Token)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		got := jsonBody[map[string]any](t, w.Body.Bytes())
		groupID = got["id"].(string)
		assert.Equal(t, true, got["allow_private_courses"])
		members := got["members"].([]any)
		require.Len(t, members, 1)
		assert.Equal(t, "leader", members[0].(map[string]any)["role"])
		// One group per coach.
		assert.Equal(t, http.StatusConflict, executeRequest("POST", "/v1/coach-groups", body, leader.Token).Code)

		w = executeRequest("GET", "/v1/coaches/me/group", nil, leader.Token)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, groupID, jsonBody[map[string]any](t, w.Body.Bytes())["id"])
		assert.Equal(t, http.StatusNotFound, executeRequest("GET", "/v1/coaches/me/group", nil, member.Token).Code)
		w = executeRequest("GET", "/v1/coach-groups/"+groupID, nil, outsider.Token)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, http.StatusUnauthorized, executeRequest("GET", "/v1/coach-groups/"+groupID, nil, "").Code)
	})

	var invitationID string
	t.Run("invite, decline, and accept", func(t *testing.T) {
		path := "/v1/coach-groups/" + groupID + "/invitations"
		assert.Equal(t, http.StatusForbidden, executeRequest("POST", path, map[string]any{"user_id": third.ID, "role": "coach"}, member.Token).Code)
		assert.Equal(t, http.StatusBadRequest, executeRequest("POST", path, map[string]any{"user_id": third.ID, "role": "leader"}, leader.Token).Code)
		assert.Equal(t, http.StatusBadRequest, executeRequest("POST", path, map[string]any{"user_id": leader.ID, "role": "coach"}, leader.Token).Code)
		assert.Equal(t, http.StatusBadRequest, executeRequest("POST", path, map[string]any{"user_id": outsider.ID, "role": "coach"}, leader.Token).Code, "invitee must be an approved coach")

		w := executeRequest("POST", path, map[string]any{"user_id": member.ID, "role": "assistant"}, leader.Token)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		invitationID = jsonBody[map[string]any](t, w.Body.Bytes())["id"].(string)
		assert.Equal(t, http.StatusConflict, executeRequest("POST", path, map[string]any{"user_id": member.ID, "role": "coach"}, leader.Token).Code)
		assert.Equal(t, 1, countNotifications(t, member.ID, "coach_group_invited"))

		// The invitee sees it; nobody else can answer it.
		w = executeRequest("GET", "/v1/coaches/me/invitations?status=pending", nil, member.Token)
		assert.Equal(t, float64(1), jsonBody[map[string]any](t, w.Body.Bytes())["total"])
		assert.Equal(t, http.StatusNotFound, executeRequest("POST", "/v1/coach-invitations/"+invitationID+"/accept", nil, third.Token).Code)

		// Decline, re-invite, then accept.
		assert.Equal(t, http.StatusNoContent, executeRequest("POST", "/v1/coach-invitations/"+invitationID+"/decline", nil, member.Token).Code)
		assert.Equal(t, 1, countNotifications(t, leader.ID, "coach_group_invite_declined"))
		assert.Equal(t, http.StatusConflict, executeRequest("POST", "/v1/coach-invitations/"+invitationID+"/accept", nil, member.Token).Code)

		w = executeRequest("POST", path, map[string]any{"user_id": member.ID, "role": "assistant"}, leader.Token)
		require.Equal(t, http.StatusCreated, w.Code)
		invitationID = jsonBody[map[string]any](t, w.Body.Bytes())["id"].(string)
		w = executeRequest("POST", "/v1/coach-invitations/"+invitationID+"/accept", nil, member.Token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Len(t, jsonBody[map[string]any](t, w.Body.Bytes())["members"], 2)
		assert.Equal(t, 1, countNotifications(t, leader.ID, "coach_group_invite_accepted"))

		// Already-in-group coaches cannot be invited again.
		assert.Equal(t, http.StatusConflict, executeRequest("POST", path, map[string]any{"user_id": member.ID, "role": "coach"}, leader.Token).Code)

		w = executeRequest("GET", path, nil, leader.Token)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, http.StatusForbidden, executeRequest("GET", path, nil, member.Token).Code)
	})

	t.Run("member updates and last-leader protection", func(t *testing.T) {
		mpath := "/v1/coach-groups/" + groupID + "/members/" + member.ID
		lpath := "/v1/coach-groups/" + groupID + "/members/" + leader.ID
		assert.Equal(t, http.StatusBadRequest, executeRequest("PATCH", mpath, map[string]any{}, leader.Token).Code)
		assert.Equal(t, http.StatusBadRequest, executeRequest("PATCH", mpath, map[string]any{"status": "gone"}, leader.Token).Code)
		// A member may rest themselves but not promote themselves.
		assert.Equal(t, http.StatusOK, executeRequest("PATCH", mpath, map[string]any{"status": "inactive"}, member.Token).Code)
		assert.Equal(t, http.StatusForbidden, executeRequest("PATCH", mpath, map[string]any{"role": "leader"}, member.Token).Code)
		assert.Equal(t, http.StatusForbidden, executeRequest("PATCH", lpath, map[string]any{"status": "inactive"}, member.Token).Code)

		// The sole leader cannot demote or remove themselves.
		assert.Equal(t, http.StatusConflict, executeRequest("PATCH", lpath, map[string]any{"role": "coach"}, leader.Token).Code)
		assert.Equal(t, http.StatusConflict, executeRequest("DELETE", lpath, nil, leader.Token).Code)

		w := executeRequest("PATCH", mpath, map[string]any{"role": "leader", "status": "active"}, leader.Token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, 1, countNotifications(t, member.ID, "coach_group_member_changed"))
		// Now the original leader may step down.
		assert.Equal(t, http.StatusOK, executeRequest("PATCH", lpath, map[string]any{"role": "coach"}, leader.Token).Code)
		// And a former leader is now a plain member.
		assert.Equal(t, http.StatusForbidden, executeRequest("PATCH", mpath, map[string]any{"role": "coach"}, leader.Token).Code)
	})

	t.Run("update group, remove, leave, and disband", func(t *testing.T) {
		gpath := "/v1/coach-groups/" + groupID
		assert.Equal(t, http.StatusForbidden, executeRequest("PATCH", gpath, map[string]any{"name": "Nope"}, leader.Token).Code)
		w := executeRequest("PATCH", gpath, map[string]any{"name": "Renamed", "allow_private_courses": false}, member.Token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, "Renamed", jsonBody[map[string]any](t, w.Body.Bytes())["name"])
		assert.Equal(t, http.StatusBadRequest, executeRequest("PATCH", gpath, map[string]any{}, member.Token).Code)

		// The leader removes the former leader, who is notified; they can join elsewhere.
		assert.Equal(t, http.StatusNoContent, executeRequest("DELETE", gpath+"/members/"+leader.ID, nil, member.Token).Code)
		assert.Equal(t, 1, countNotifications(t, leader.ID, "coach_group_member_removed"))
		assert.Equal(t, http.StatusNotFound, executeRequest("DELETE", gpath+"/members/"+leader.ID, nil, member.Token).Code)
		assert.Equal(t, http.StatusForbidden, executeRequest("DELETE", gpath+"/members/"+member.ID, nil, third.Token).Code)

		// The last member leaving dissolves the group.
		assert.Equal(t, http.StatusNoContent, executeRequest("DELETE", gpath+"/members/"+member.ID, nil, member.Token).Code)
		assert.Equal(t, http.StatusNotFound, executeRequest("GET", gpath, nil, member.Token).Code)
		assert.Equal(t, http.StatusNotFound, executeRequest("POST", gpath+"/invitations", map[string]any{"user_id": third.ID, "role": "coach"}, member.Token).Code)

		w = executeRequest("POST", "/v1/coach-groups", map[string]any{"name": "Second"}, leader.Token)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		second := jsonBody[map[string]any](t, w.Body.Bytes())["id"].(string)

		// Disband by admin notifies the members and cancels open invitations.
		w = executeRequest("POST", "/v1/coach-groups/"+second+"/invitations", map[string]any{"user_id": third.ID, "role": "coach"}, leader.Token)
		require.Equal(t, http.StatusCreated, w.Code)
		inv := jsonBody[map[string]any](t, w.Body.Bytes())["id"].(string)
		assert.Equal(t, http.StatusNoContent, executeRequest("DELETE", "/v1/coach-groups/"+second, nil, admin.Token).Code)
		assert.Equal(t, 1, countNotifications(t, leader.ID, "coach_group_disbanded"))
		assert.Equal(t, http.StatusConflict, executeRequest("POST", "/v1/coach-invitations/"+inv+"/accept", nil, third.Token).Code)
	})

	t.Run("invitation cancel", func(t *testing.T) {
		w := executeRequest("POST", "/v1/coach-groups", map[string]any{"name": "Third"}, leader.Token)
		require.Equal(t, http.StatusCreated, w.Code)
		g := jsonBody[map[string]any](t, w.Body.Bytes())["id"].(string)
		w = executeRequest("POST", "/v1/coach-groups/"+g+"/invitations", map[string]any{"user_id": third.ID, "role": "coach"}, leader.Token)
		require.Equal(t, http.StatusCreated, w.Code)
		inv := jsonBody[map[string]any](t, w.Body.Bytes())["id"].(string)
		assert.Equal(t, http.StatusForbidden, executeRequest("DELETE", "/v1/coach-groups/"+g+"/invitations/"+inv, nil, third.Token).Code)
		assert.Equal(t, http.StatusNoContent, executeRequest("DELETE", "/v1/coach-groups/"+g+"/invitations/"+inv, nil, leader.Token).Code)
		assert.Equal(t, http.StatusConflict, executeRequest("DELETE", "/v1/coach-groups/"+g+"/invitations/"+inv, nil, leader.Token).Code)
		assert.Equal(t, http.StatusNotFound, executeRequest("DELETE", "/v1/coach-groups/"+g+"/invitations/"+uuid.NewString(), nil, leader.Token).Code)
		assert.Equal(t, http.StatusConflict, executeRequest("POST", "/v1/coach-invitations/"+inv+"/accept", nil, third.Token).Code)
	})
}

// A coach holding invitations from two groups can join only one of them, even
// when both are accepted at the same instant.
func TestCoachGroupConcurrentAccept(t *testing.T) {
	clearTables()
	admin := newCoachUser(t, "admin", true)
	sportID := getSportID(t, "BADMINTON")
	leaderA := makeApprovedCoach(t, "leadera", admin, sportID)
	leaderB := makeApprovedCoach(t, "leaderb", admin, sportID)
	target := makeApprovedCoach(t, "target", admin, sportID)

	var invs []string
	for _, l := range []coachUser{leaderA, leaderB} {
		w := executeRequest("POST", "/v1/coach-groups", map[string]any{"name": "G"}, l.Token)
		require.Equal(t, http.StatusCreated, w.Code)
		g := jsonBody[map[string]any](t, w.Body.Bytes())["id"].(string)
		w = executeRequest("POST", "/v1/coach-groups/"+g+"/invitations", map[string]any{"user_id": target.ID, "role": "coach"}, l.Token)
		require.Equal(t, http.StatusCreated, w.Code)
		invs = append(invs, jsonBody[map[string]any](t, w.Body.Bytes())["id"].(string))
	}

	codes := make([]int, len(invs))
	var wg sync.WaitGroup
	for i, inv := range invs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i] = executeRequest("POST", "/v1/coach-invitations/"+inv+"/accept", nil, target.Token).Code
		}()
	}
	wg.Wait()
	assert.ElementsMatch(t, []int{http.StatusOK, http.StatusConflict}, codes)
	var n int
	require.NoError(t, testPool.QueryRow(context.Background(), "SELECT count(*) FROM public.coach_group_members WHERE user_id = $1", target.ID).Scan(&n))
	assert.Equal(t, 1, n)
}
