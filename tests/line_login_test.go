package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nekogravitycat/court-booking-backend/internal/app"
	"github.com/nekogravitycat/court-booking-backend/internal/lineauth"
	orgHttp "github.com/nekogravitycat/court-booking-backend/internal/organization/http"
	userHttp "github.com/nekogravitycat/court-booking-backend/internal/user/http"
)

// fakeLine stands in for the LINE API: each authorization code maps to a profile.
type fakeLine struct {
	profiles   map[string]*lineauth.Profile
	picture    []byte
	pictureErr error
}

func (f *fakeLine) Exchange(_ context.Context, in lineauth.ExchangeInput) (*lineauth.Profile, error) {
	p, ok := f.profiles[in.Code]
	if !ok {
		return nil, lineauth.ErrAuthorizationFailed
	}
	return p, nil
}

func (f *fakeLine) FetchPicture(context.Context, string) ([]byte, error) {
	return f.picture, f.pictureErr
}

func testJPEG(t *testing.T) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for x := 0; x < 8; x++ {
		for y := 0; y < 8; y++ {
			img.Set(x, y, color.RGBA{R: 200, G: 30, B: 30, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, nil))
	return buf.Bytes()
}

// lineEnv is a router with LINE login enabled backed by a fake LINE client.
type lineEnv struct {
	router http.Handler
	fake   *fakeLine
}

func newLineEnv(t *testing.T) *lineEnv {
	fake := &fakeLine{profiles: map[string]*lineauth.Profile{}, picture: testJPEG(t)}
	c := app.NewContainer(app.Config{
		DBPool:     testPool,
		JWTSecret:  os.Getenv("TEST_JWT_SECRET"),
		JWTTTL:     30 * time.Minute,
		SessionTTL: 24 * time.Hour,
		BcryptCost: 4,
		LineClient: fake,
	})
	return &lineEnv{router: c.Router, fake: fake}
}

func (e *lineEnv) do(method, path string, body any, token string) *httptest.ResponseRecorder {
	var raw []byte
	if body != nil {
		raw, _ = json.Marshal(body)
	}
	req, _ := http.NewRequest(method, path, bytes.NewBuffer(raw))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	return w
}

// lineBody builds a valid LINE auth request for the given fake authorization code.
func lineBody(code string) userHttp.LineAuthRequest {
	return userHttp.LineAuthRequest{
		Code:         code,
		RedirectURI:  "https://app.example.com/callback",
		CodeVerifier: "v-0123456789012345678901234567890123456789012",
		Nonce:        "nonce",
	}
}

func (e *lineEnv) lineLogin(t *testing.T, code string) (userHttp.LoginResponse, *httptest.ResponseRecorder) {
	w := e.do("POST", "/v1/auth/line", lineBody(code), "")
	var resp userHttp.LoginResponse
	if w.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	}
	return resp, w
}

var generatedUsernamePattern = regexp.MustCompile(`^l[a-z0-9]{14}$`)

func TestLineLoginCreatesAccount(t *testing.T) {
	clearTables()
	e := newLineEnv(t)
	e.fake.profiles["c1"] = &lineauth.Profile{Subject: "U-alice", Name: "Alice", Email: "Alice@Line.test", PictureURL: "https://profile.line-scdn.net/x"}

	resp, w := e.lineLogin(t, "c1")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	require.NotNil(t, resp.IsNewUser)
	assert.True(t, *resp.IsNewUser)
	assert.NotEmpty(t, resp.AccessToken)
	assert.NotNil(t, sessionCookieFrom(w), "login must set the session cookie")

	u := resp.User
	require.NotNil(t, u.DisplayName)
	assert.Equal(t, "Alice", *u.DisplayName)
	require.NotNil(t, u.Email)
	assert.Equal(t, "alice@line.test", *u.Email)
	assert.Regexp(t, generatedUsernamePattern, u.Username)
	assert.False(t, u.HasPassword)
	assert.Equal(t, []string{"line"}, u.LinkedProviders)
	assert.NotNil(t, u.Avatar, "profile picture should be imported as the avatar")

	t.Run("Access token works", func(t *testing.T) {
		wMe := e.do("GET", "/v1/me", nil, resp.AccessToken)
		assert.Equal(t, http.StatusOK, wMe.Code)
	})

	t.Run("Returning login reuses the account", func(t *testing.T) {
		again, w := e.lineLogin(t, "c1")
		require.Equal(t, http.StatusOK, w.Code)
		require.NotNil(t, again.IsNewUser)
		assert.False(t, *again.IsNewUser)
		assert.Equal(t, u.ID, again.User.ID)
	})

	t.Run("Password login is refused for a password-less account", func(t *testing.T) {
		wLogin := executeRequest("POST", "/v1/auth/login", userHttp.LoginRequest{Email: "alice@line.test", Password: "anything123"}, "")
		assert.Equal(t, http.StatusUnauthorized, wLogin.Code)
	})
}

func TestLineLoginWithoutEmailOrPicture(t *testing.T) {
	clearTables()
	e := newLineEnv(t)
	e.fake.profiles["a"] = &lineauth.Profile{Subject: "U-a", Name: "  "}
	e.fake.profiles["b"] = &lineauth.Profile{Subject: "U-b", Name: "Bob", PictureURL: "https://profile.line-scdn.net/b"}
	e.fake.pictureErr = assert.AnError

	a, w := e.lineLogin(t, "a")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Nil(t, a.User.Email, "no email shared means a null email")
	require.NotNil(t, a.User.DisplayName)
	assert.Equal(t, "LINE user", *a.User.DisplayName, "blank names fall back to a default")
	assert.Nil(t, a.User.Avatar)

	// Several email-less accounts must coexist (NULL emails do not collide).
	b, w := e.lineLogin(t, "b")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotEqual(t, a.User.ID, b.User.ID)
	assert.Nil(t, b.User.Avatar, "a failed picture download must not fail the sign-up")
}

func TestLineLoginEmailOwnedByAnotherAccount(t *testing.T) {
	clearTables()
	e := newLineEnv(t)
	existing := createTestUser(t, "taken@example.com", "password123", false)
	e.fake.profiles["c"] = &lineauth.Profile{Subject: "U-c", Name: "Carol", Email: "taken@example.com"}

	resp, w := e.lineLogin(t, "c")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotEqual(t, existing.ID, resp.User.ID, "must not merge into the account that owns the email")
	assert.Nil(t, resp.User.Email)
}

func TestLineLoginErrors(t *testing.T) {
	clearTables()
	e := newLineEnv(t)
	e.fake.profiles["ok"] = &lineauth.Profile{Subject: "U-ok", Name: "Ok"}

	t.Run("Invalid code", func(t *testing.T) {
		_, w := e.lineLogin(t, "bad")
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Missing fields", func(t *testing.T) {
		w := e.do("POST", "/v1/auth/line", map[string]string{"code": "ok"}, "")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Inactive account", func(t *testing.T) {
		resp, w := e.lineLogin(t, "ok")
		require.Equal(t, http.StatusOK, w.Code)
		_, err := testPool.Exec(context.Background(), "UPDATE public.users SET is_active = false WHERE id = $1", resp.User.ID)
		require.NoError(t, err)

		_, w = e.lineLogin(t, "ok")
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Disabled when not configured", func(t *testing.T) {
		w := executeRequest("POST", "/v1/auth/line", lineBody("ok"), "")
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestLineLoginConcurrentFirstLogin(t *testing.T) {
	clearTables()
	e := newLineEnv(t)
	e.fake.profiles["race"] = &lineauth.Profile{Subject: "U-race", Name: "Racer", Email: "racer@line.test"}

	const n = 8
	ids := make([]string, n)
	codes := make([]int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			resp, w := e.lineLogin(t, "race")
			codes[i] = w.Code
			ids[i] = resp.User.ID
		}(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		assert.Equal(t, http.StatusOK, codes[i])
		assert.Equal(t, ids[0], ids[i], "every concurrent first login must land on the same account")
	}
	var count int
	require.NoError(t, testPool.QueryRow(context.Background(), "SELECT count(*) FROM public.users").Scan(&count))
	assert.Equal(t, 1, count)
}

func TestLineLinkAndUnlink(t *testing.T) {
	clearTables()
	e := newLineEnv(t)
	pwUser := createTestUser(t, "linker@example.com", "password123", false)
	token := generateToken(pwUser.ID)
	e.fake.profiles["mine"] = &lineauth.Profile{Subject: "U-mine", Name: "Mine"}
	e.fake.profiles["other"] = &lineauth.Profile{Subject: "U-other", Name: "Other"}

	t.Run("Link requires login", func(t *testing.T) {
		w := e.do("POST", "/v1/me/identities/line", lineBody("mine"), "")
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Link", func(t *testing.T) {
		w := e.do("POST", "/v1/me/identities/line", lineBody("mine"), token)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp userHttp.MeResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, []string{"line"}, resp.User.LinkedProviders)
		assert.True(t, resp.User.HasPassword)
	})

	t.Run("LINE login now reaches the linked account", func(t *testing.T) {
		resp, w := e.lineLogin(t, "mine")
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, pwUser.ID, resp.User.ID)
		require.NotNil(t, resp.IsNewUser)
		assert.False(t, *resp.IsNewUser)
	})

	t.Run("Linking twice is a conflict", func(t *testing.T) {
		w := e.do("POST", "/v1/me/identities/line", lineBody("other"), token)
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("Another account cannot take a linked LINE identity", func(t *testing.T) {
		second := createTestUser(t, "second@example.com", "password123", false)
		w := e.do("POST", "/v1/me/identities/line", lineBody("mine"), generateToken(second.ID))
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("Unlink", func(t *testing.T) {
		w := e.do("DELETE", "/v1/me/identities/line", nil, token)
		assert.Equal(t, http.StatusNoContent, w.Code)

		w = e.do("DELETE", "/v1/me/identities/line", nil, token)
		assert.Equal(t, http.StatusNotFound, w.Code, "unlinking a provider that is not linked")
	})
}

func TestLineOnlyAccountPasswordFlow(t *testing.T) {
	clearTables()
	e := newLineEnv(t)
	e.fake.profiles["solo"] = &lineauth.Profile{Subject: "U-solo", Name: "Solo"}

	resp, w := e.lineLogin(t, "solo")
	require.Equal(t, http.StatusOK, w.Code)
	token := resp.AccessToken

	t.Run("Cannot unlink the only login method", func(t *testing.T) {
		w := e.do("DELETE", "/v1/me/identities/line", nil, token)
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("Setting a password needs an email when the account has none", func(t *testing.T) {
		w := e.do("PUT", "/v1/me/password", userHttp.SetPasswordRequest{Password: "password123"}, token)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Email already used by another account", func(t *testing.T) {
		createTestUser(t, "used@example.com", "password123", false)
		w := e.do("PUT", "/v1/me/password", userHttp.SetPasswordRequest{Email: "used@example.com", Password: "password123"}, token)
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("Set password and email", func(t *testing.T) {
		w := e.do("PUT", "/v1/me/password", userHttp.SetPasswordRequest{Email: "Solo@Example.com", Password: "password123"}, token)
		require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())

		wLogin := executeRequest("POST", "/v1/auth/login", userHttp.LoginRequest{Email: "solo@example.com", Password: "password123"}, "")
		assert.Equal(t, http.StatusOK, wLogin.Code, "password login should work after setting a password")
	})

	t.Run("Password can be set only once", func(t *testing.T) {
		w := e.do("PUT", "/v1/me/password", userHttp.SetPasswordRequest{Password: "another-pass1"}, token)
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("Unlink now allowed", func(t *testing.T) {
		w := e.do("DELETE", "/v1/me/identities/line", nil, token)
		assert.Equal(t, http.StatusNoContent, w.Code)
	})
}

func TestOrganizationAddMemberByUsername(t *testing.T) {
	clearTables()
	e := newLineEnv(t)
	sysAdmin := createTestUser(t, "sys@org-username.com", "pass1234", true)
	owner := createTestUser(t, "owner@org-username.com", "pass1234", false)
	e.fake.profiles["m"] = &lineauth.Profile{Subject: "U-m", Name: "Member"}
	member, w := e.lineLogin(t, "m")
	require.Equal(t, http.StatusOK, w.Code)

	wOrg := e.do("POST", "/v1/organizations", orgHttp.CreateOrganizationRequest{Name: "Username Org", OwnerID: owner.ID}, generateToken(sysAdmin.ID))
	require.Equal(t, http.StatusCreated, wOrg.Code)
	var org orgHttp.OrganizationResponse
	require.NoError(t, json.Unmarshal(wOrg.Body.Bytes(), &org))
	ownerToken := generateToken(owner.ID)

	t.Run("Needs exactly one identifier", func(t *testing.T) {
		w := e.do("POST", "/v1/organizations/"+org.ID+"/members", map[string]string{}, ownerToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		w = e.do("POST", "/v1/organizations/"+org.ID+"/members",
			map[string]string{"email": "owner@org-username.com", "username": member.User.Username}, ownerToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Unknown username", func(t *testing.T) {
		w := e.do("POST", "/v1/organizations/"+org.ID+"/members", map[string]string{"username": "nobody_here"}, ownerToken)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("Add by username", func(t *testing.T) {
		w := e.do("POST", "/v1/organizations/"+org.ID+"/members", map[string]string{"username": member.User.Username}, ownerToken)
		assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	})

	t.Run("Member without an email lists with a null email", func(t *testing.T) {
		w := e.do("GET", "/v1/organizations/"+org.ID+"/members", nil, ownerToken)
		require.Equal(t, http.StatusOK, w.Code)
		var resp struct {
			Items []struct {
				ID    string  `json:"id"`
				Email *string `json:"email"`
			} `json:"items"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Len(t, resp.Items, 1)
		assert.Equal(t, member.User.ID, resp.Items[0].ID)
		assert.Nil(t, resp.Items[0].Email)
	})
}
