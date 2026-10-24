// Package lineauth verifies LINE Login (OpenID Connect) authorization codes.
package lineauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
)

var (
	ErrRedirectURINotAllowed = apperror.New(http.StatusBadRequest, "redirect_uri is not allowed")
	ErrAuthorizationFailed   = apperror.New(http.StatusUnauthorized, "LINE authorization failed")
	ErrUpstream              = apperror.New(http.StatusBadGateway, "LINE service is unavailable")
)

const (
	defaultAPIBaseURL = "https://api.line.me"
	// issuer is the iss claim LINE puts in every ID token.
	issuer = "https://access.line.me"
	// pictureHost is the only host profile pictures are fetched from.
	pictureHost = "profile.line-scdn.net"

	requestTimeout = 10 * time.Second
	maxPictureSize = 5 * 1024 * 1024
)

// Config holds the LINE Login channel settings.
type Config struct {
	ChannelID     string
	ChannelSecret string
	// RedirectURIs lists the callback URLs the frontends registered in the LINE
	// console; a request using any other redirect_uri is rejected.
	RedirectURIs []string
	// APIBaseURL overrides https://api.line.me (used by tests).
	APIBaseURL string
}

// Profile is the signed-in person as described by LINE.
type Profile struct {
	Subject    string // LINE user ID, stable per channel provider
	Name       string
	PictureURL string // Empty when the person has no profile picture
	Email      string // Empty unless the email scope was granted and LINE has one
}

// ExchangeInput carries what the frontend got from the LINE authorization redirect.
type ExchangeInput struct {
	Code         string
	RedirectURI  string
	CodeVerifier string
	Nonce        string
}

// Client talks to LINE on behalf of the backend.
type Client interface {
	// Exchange trades an authorization code for a verified profile.
	Exchange(ctx context.Context, in ExchangeInput) (*Profile, error)
	// FetchPicture downloads a profile picture returned in Profile.PictureURL.
	FetchPicture(ctx context.Context, pictureURL string) ([]byte, error)
}

type httpClient struct {
	cfg  Config
	base string
	http *http.Client
}

// NewClient returns a Client that calls the LINE API.
func NewClient(cfg Config) Client {
	base := strings.TrimRight(cfg.APIBaseURL, "/")
	if base == "" {
		base = defaultAPIBaseURL
	}
	return &httpClient{
		cfg:  cfg,
		base: base,
		http: &http.Client{
			Timeout: requestTimeout,
			// A redirect would leave the host check; treat it as a failed response.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

type tokenResponse struct {
	IDToken string `json:"id_token"`
}

type idTokenClaims struct {
	Issuer   string `json:"iss"`
	Subject  string `json:"sub"`
	Audience string `json:"aud"`
	Expires  int64  `json:"exp"`
	Nonce    string `json:"nonce"`
	Name     string `json:"name"`
	Picture  string `json:"picture"`
	Email    string `json:"email"`
}

func (c *httpClient) Exchange(ctx context.Context, in ExchangeInput) (*Profile, error) {
	if !slices.Contains(c.cfg.RedirectURIs, in.RedirectURI) {
		return nil, ErrRedirectURINotAllowed
	}

	var token tokenResponse
	err := c.postForm(ctx, "/oauth2/v2.1/token", url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {in.Code},
		"redirect_uri":  {in.RedirectURI},
		"client_id":     {c.cfg.ChannelID},
		"client_secret": {c.cfg.ChannelSecret},
		"code_verifier": {in.CodeVerifier},
	}, &token)
	if err != nil {
		return nil, err
	}
	if token.IDToken == "" {
		return nil, ErrAuthorizationFailed
	}

	// LINE's verify endpoint checks the signature, expiry, audience and nonce.
	var claims idTokenClaims
	err = c.postForm(ctx, "/oauth2/v2.1/verify", url.Values{
		"id_token":  {token.IDToken},
		"client_id": {c.cfg.ChannelID},
		"nonce":     {in.Nonce},
	}, &claims)
	if err != nil {
		return nil, err
	}

	// Re-check the claims we depend on instead of trusting the response shape.
	if claims.Subject == "" || claims.Issuer != issuer || claims.Audience != c.cfg.ChannelID ||
		claims.Nonce != in.Nonce || claims.Expires <= time.Now().Unix() {
		return nil, ErrAuthorizationFailed
	}

	return &Profile{
		Subject:    claims.Subject,
		Name:       claims.Name,
		PictureURL: claims.Picture,
		Email:      claims.Email,
	}, nil
}

// postForm posts form values and decodes a 200 JSON response into out. LINE
// answers a bad code or token with a 4xx, which means the login failed; any
// other failure is reported as an upstream problem.
func (c *httpClient) postForm(ctx context.Context, path string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build LINE request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusOK:
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
			return fmt.Errorf("%w: invalid response: %v", ErrUpstream, err)
		}
		return nil
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		return ErrAuthorizationFailed
	default:
		return fmt.Errorf("%w: status %d", ErrUpstream, resp.StatusCode)
	}
}

// pictureSizeSuffix asks LINE's CDN for the 200x200 rendition.
const pictureSizeSuffix = "/large"

func (c *httpClient) FetchPicture(ctx context.Context, pictureURL string) ([]byte, error) {
	u, err := url.Parse(pictureURL)
	if err != nil || u.Scheme != "https" || u.Hostname() != pictureHost {
		return nil, errors.New("profile picture URL is not on the LINE CDN")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(pictureURL, "/")+pictureSizeSuffix, nil)
	if err != nil {
		return nil, fmt.Errorf("build picture request failed: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch picture failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch picture failed: status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxPictureSize+1))
	if err != nil {
		return nil, fmt.Errorf("read picture failed: %w", err)
	}
	if len(data) > maxPictureSize {
		return nil, errors.New("profile picture is too large")
	}
	return data, nil
}
