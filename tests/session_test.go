package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	userHttp "github.com/nekogravitycat/court-booking-backend/internal/user/http"
)

// sessionCookieName is the dev/test cookie name (production uses a __Host- prefix).
const sessionCookieName = "session"

// cookieRequest sends a request carrying the session cookie and optional Origin header.
func cookieRequest(method, path, cookie, origin string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest(method, path, bytes.NewBuffer(nil))
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie})
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	testRouter.ServeHTTP(w, req)
	return w
}

func sessionCookieFrom(w *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookieName {
			return c
		}
	}
	return nil
}

func TestSharedSession(t *testing.T) {
	clearTables()
	createTestUser(t, "shared@example.com", "password123", false)

	wLogin := executeRequest("POST", "/v1/auth/login", userHttp.LoginRequest{Email: "shared@example.com", Password: "password123"}, "")
	require.Equal(t, http.StatusOK, wLogin.Code)

	cookie := sessionCookieFrom(wLogin)
	require.NotNil(t, cookie, "login must set the session cookie")
	assert.True(t, cookie.HttpOnly)
	assert.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
	assert.Empty(t, cookie.Domain, "cookie must be host-only")
	assert.Equal(t, 24*3600, cookie.MaxAge)

	var loginResp userHttp.LoginResponse
	require.NoError(t, json.Unmarshal(wLogin.Body.Bytes(), &loginResp))

	t.Run("Another frontend refreshes using the cookie", func(t *testing.T) {
		w := cookieRequest("POST", "/v1/auth/refresh", cookie.Value, "https://admin.example.com")
		require.Equal(t, http.StatusOK, w.Code)

		var resp userHttp.LoginResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, loginResp.User.ID, resp.User.ID)
		assert.Equal(t, http.StatusOK, executeRequest("GET", "/v1/me", nil, resp.AccessToken).Code)
	})

	t.Run("Refresh without cookie is unauthorized", func(t *testing.T) {
		assert.Equal(t, http.StatusUnauthorized, cookieRequest("POST", "/v1/auth/refresh", "", "").Code)
	})

	t.Run("Refresh with unknown cookie is unauthorized and clears it", func(t *testing.T) {
		w := cookieRequest("POST", "/v1/auth/refresh", "bogus", "")
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		cleared := sessionCookieFrom(w)
		require.NotNil(t, cleared)
		assert.Negative(t, cleared.MaxAge)
	})

	t.Run("Logout revokes the session and its access tokens everywhere", func(t *testing.T) {
		wOther := cookieRequest("POST", "/v1/auth/refresh", cookie.Value, "")
		require.Equal(t, http.StatusOK, wOther.Code)
		var other userHttp.LoginResponse
		require.NoError(t, json.Unmarshal(wOther.Body.Bytes(), &other))

		w := cookieRequest("POST", "/v1/auth/logout", cookie.Value, "")
		assert.Equal(t, http.StatusNoContent, w.Code)
		cleared := sessionCookieFrom(w)
		require.NotNil(t, cleared)
		assert.Negative(t, cleared.MaxAge)

		assert.Equal(t, http.StatusUnauthorized, executeRequest("GET", "/v1/me", nil, loginResp.AccessToken).Code, "login token must be revoked")
		assert.Equal(t, http.StatusUnauthorized, executeRequest("GET", "/v1/me", nil, other.AccessToken).Code, "refreshed token must be revoked")
		assert.Equal(t, http.StatusUnauthorized, cookieRequest("POST", "/v1/auth/refresh", cookie.Value, "").Code)

		assert.Equal(t, http.StatusNoContent, cookieRequest("POST", "/v1/auth/logout", cookie.Value, "").Code, "logout is idempotent")
	})
}

func TestLogoutWithBearerOnly(t *testing.T) {
	clearTables()
	u := createTestUser(t, "bearer@example.com", "password123", false)
	token := generateToken(u.ID)

	require.Equal(t, http.StatusOK, executeRequest("GET", "/v1/me", nil, token).Code)
	assert.Equal(t, http.StatusNoContent, executeRequest("POST", "/v1/auth/logout", nil, token).Code)
	assert.Equal(t, http.StatusUnauthorized, executeRequest("GET", "/v1/me", nil, token).Code)
}

func TestSessionTokenRequiresLiveSession(t *testing.T) {
	clearTables()
	u := createTestUser(t, "gone@example.com", "password123", false)

	// A token for a session that does not exist must be rejected.
	token, err := jwtManager.GenerateAccessToken(u.ID, "00000000-0000-0000-0000-000000000000")
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, executeRequest("GET", "/v1/me", nil, token).Code)
}
