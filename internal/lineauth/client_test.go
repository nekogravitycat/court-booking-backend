package lineauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testRedirect = "https://app.example.com/callback"
	testChannel  = "1234567890"
)

// fakeLINE serves the token and verify endpoints. verifyClaims is returned by
// /verify as-is so tests can simulate a bad token.
func fakeLINE(t *testing.T, tokenStatus int, verifyClaims map[string]any) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/v2.1/token", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "authorization_code", r.PostForm.Get("grant_type"))
		assert.Equal(t, testRedirect, r.PostForm.Get("redirect_uri"))
		assert.Equal(t, "verifier", r.PostForm.Get("code_verifier"))
		assert.Equal(t, "secret", r.PostForm.Get("client_secret"))
		w.WriteHeader(tokenStatus)
		_ = json.NewEncoder(w).Encode(map[string]string{"id_token": "tok"})
	})
	mux.HandleFunc("/oauth2/v2.1/verify", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "tok", r.PostForm.Get("id_token"))
		assert.Equal(t, testChannel, r.PostForm.Get("client_id"))
		_ = json.NewEncoder(w).Encode(verifyClaims)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newTestClient(srv *httptest.Server) Client {
	return NewClient(Config{
		ChannelID:     testChannel,
		ChannelSecret: "secret",
		RedirectURIs:  []string{testRedirect},
		APIBaseURL:    srv.URL,
	})
}

func goodClaims() map[string]any {
	return map[string]any{
		"iss": issuer, "sub": "U123", "aud": testChannel,
		"exp": time.Now().Add(time.Hour).Unix(), "nonce": "n1",
		"name": "Alice", "picture": "https://profile.line-scdn.net/abc", "email": "a@line.test",
	}
}

var goodInput = ExchangeInput{Code: "c", RedirectURI: testRedirect, CodeVerifier: "verifier", Nonce: "n1"}

func TestExchange(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		c := newTestClient(fakeLINE(t, http.StatusOK, goodClaims()))
		p, err := c.Exchange(context.Background(), goodInput)
		require.NoError(t, err)
		assert.Equal(t, &Profile{Subject: "U123", Name: "Alice", PictureURL: "https://profile.line-scdn.net/abc", Email: "a@line.test"}, p)
	})

	t.Run("Redirect URI not on the allowlist", func(t *testing.T) {
		c := newTestClient(fakeLINE(t, http.StatusOK, goodClaims()))
		in := goodInput
		in.RedirectURI = "https://evil.example.com/callback"
		_, err := c.Exchange(context.Background(), in)
		assert.ErrorIs(t, err, ErrRedirectURINotAllowed)
	})

	t.Run("LINE rejects the code", func(t *testing.T) {
		c := newTestClient(fakeLINE(t, http.StatusBadRequest, goodClaims()))
		_, err := c.Exchange(context.Background(), goodInput)
		assert.ErrorIs(t, err, ErrAuthorizationFailed)
	})

	t.Run("LINE is down", func(t *testing.T) {
		c := newTestClient(fakeLINE(t, http.StatusBadGateway, goodClaims()))
		_, err := c.Exchange(context.Background(), goodInput)
		assert.ErrorIs(t, err, ErrUpstream)
	})

	bad := map[string]func(map[string]any){
		"wrong issuer":   func(m map[string]any) { m["iss"] = "https://evil.example.com" },
		"wrong audience": func(m map[string]any) { m["aud"] = "other-channel" },
		"wrong nonce":    func(m map[string]any) { m["nonce"] = "other" },
		"expired":        func(m map[string]any) { m["exp"] = time.Now().Add(-time.Minute).Unix() },
		"missing sub":    func(m map[string]any) { delete(m, "sub") },
	}
	for name, mutate := range bad {
		t.Run("Rejects "+name, func(t *testing.T) {
			claims := goodClaims()
			mutate(claims)
			c := newTestClient(fakeLINE(t, http.StatusOK, claims))
			_, err := c.Exchange(context.Background(), goodInput)
			assert.ErrorIs(t, err, ErrAuthorizationFailed)
		})
	}
}

func TestFetchPictureRejectsOtherHosts(t *testing.T) {
	c := NewClient(Config{})
	for _, u := range []string{
		"http://profile.line-scdn.net/abc",
		"https://evil.example.com/abc",
		"https://profile.line-scdn.net.evil.com/abc",
		"https://127.0.0.1/abc",
		"not a url",
	} {
		_, err := c.FetchPicture(context.Background(), u)
		assert.Error(t, err, u)
	}
}
