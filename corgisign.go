// Package corgisign is the official Go client for the CorgiSign public API
// (the /api/v1 surface). It is intended for server-to-server integrations that
// authenticate with an organisation API key (a cs_live_… secret), never a user
// session.
//
// Mint a key from the CorgiSign app (Organisation -> Settings -> API keys) or
// via POST /api/orgs/{orgId}/api-keys, then:
//
//	c := corgisign.New(corgisign.Options{
//		APIKey:  os.Getenv("CORGISIGN_API_KEY"),
//		BaseURL: "https://api.corgisign.example",
//	})
//
//	tmpls, err := c.Templates.List(ctx)
//	env, err := c.Envelopes.Create(ctx, corgisign.CreateEnvelope{
//		TemplateID: tmpls[0].ID,
//		Title:      "Policy ACK",
//		Recipients: []corgisign.Recipient{{Role: "signer", Name: "Jane", Email: "jane@example.com"}},
//		Send:       true,
//	})
//
// Every call is scoped by the server to the key's organisation, its team (when
// the key is team-pinned) and the key's capability scopes. Non-2xx responses
// are returned as *corgisign.Error.
package corgisign

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Version is the SDK version, reported in the default User-Agent.
const Version = "0.1.0"

// Default request header names.
const (
	headerAPIKey         = "X-API-Key"
	headerIdempotencyKey = "Idempotency-Key"
)

// Options configures a Client. APIKey and BaseURL are required.
type Options struct {
	// APIKey is the organisation API key (a cs_live_… secret). Required.
	APIKey string
	// BaseURL is the root of the CorgiSign API, e.g. "https://api.corgisign.example"
	// or "http://localhost:8080". The "/api/v1" path is appended by the client.
	// Required.
	BaseURL string
	// HTTPClient is used for all requests. Optional; defaults to a client with a
	// 30s timeout.
	HTTPClient *http.Client
	// UserAgent overrides the default "corgisign-go/<version>" User-Agent.
	UserAgent string
}

// Client is a CorgiSign API client. Create one with New. It is safe for
// concurrent use by multiple goroutines.
type Client struct {
	apiKey     string
	baseURL    string
	userAgent  string
	httpClient *http.Client

	// Templates groups the template endpoints.
	Templates *TemplatesService
	// Envelopes groups the envelope endpoints.
	Envelopes *EnvelopesService
	// Webhooks groups the webhook-registration endpoints. To *verify* an inbound
	// webhook signature, use the sibling package
	// github.com/Corgi-Star/corgisign-go-sdk/webhooks.
	Webhooks *WebhooksService
}

// New returns a Client configured by opts. It never returns nil; if APIKey or
// BaseURL is missing the omission surfaces as an error on the first request.
func New(opts Options) *Client {
	hc := opts.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	ua := opts.UserAgent
	if ua == "" {
		ua = "corgisign-go/" + Version
	}
	c := &Client{
		apiKey:     opts.APIKey,
		baseURL:    strings.TrimRight(opts.BaseURL, "/"),
		userAgent:  ua,
		httpClient: hc,
	}
	c.Templates = &TemplatesService{c: c}
	c.Envelopes = &EnvelopesService{c: c}
	c.Webhooks = &WebhooksService{c: c}
	return c
}

// WhoAmI returns the identity and capability scoping of the API key (or OAuth
// token) configured on the client. It is the canonical "is my key valid, and
// what can it do?" probe — any valid key may call it regardless of scopes.
func (c *Client) WhoAmI(ctx context.Context, opts ...RequestOption) (*Identity, error) {
	var id Identity
	if err := c.do(ctx, http.MethodGet, "/me", nil, &id, opts); err != nil {
		return nil, err
	}
	return &id, nil
}

// Error is returned for any non-2xx API response. It carries the HTTP status,
// the server's error message and, for 429s, the Retry-After delay.
type Error struct {
	// StatusCode is the HTTP status of the response.
	StatusCode int
	// Message is the server-provided error string (the "error" JSON field), or a
	// fallback derived from the status.
	Message string
	// RetryAfter is the parsed Retry-After header (only set on 429s).
	RetryAfter time.Duration
	// Body is the raw response body, for diagnostics.
	Body []byte
}

func (e *Error) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("corgisign: %d %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("corgisign: HTTP %d", e.StatusCode)
}

// IsRateLimited reports whether the request was rejected for exceeding the
// per-key rate limit (HTTP 429). Inspect RetryAfter to back off.
func (e *Error) IsRateLimited() bool { return e.StatusCode == http.StatusTooManyRequests }

// IsNotFound reports an HTTP 404 (unknown envelope/template, or a resource in
// another organisation that reads as not found).
func (e *Error) IsNotFound() bool { return e.StatusCode == http.StatusNotFound }

// IsConflict reports an HTTP 409 (e.g. sending a non-draft envelope, or an
// idempotency key whose original request is still in flight).
func (e *Error) IsConflict() bool { return e.StatusCode == http.StatusConflict }

// IsUnprocessable reports an HTTP 422 (e.g. an invalid recipient mapping, a
// malformed payload, or an idempotency-key body mismatch).
func (e *Error) IsUnprocessable() bool { return e.StatusCode == http.StatusUnprocessableEntity }

// RequestOption customises a single request (e.g. an idempotency key or an
// extra query filter). Pass any number of them as the trailing arguments to a
// service method.
type RequestOption func(*requestConfig)

type requestConfig struct {
	idempotencyKey string
	query          url.Values
	header         http.Header
}

// WithIdempotencyKey attaches an Idempotency-Key header so a retried POST
// replays the original response instead of acting twice. Keys must be <= 255
// chars and are scoped to the API key.
func WithIdempotencyKey(key string) RequestOption {
	return func(rc *requestConfig) { rc.idempotencyKey = key }
}

// WithTeamID adds a teamId query filter. It is ignored by the server for
// team-pinned keys.
func WithTeamID(teamID string) RequestOption {
	return WithQuery("teamId", teamID)
}

// WithQuery adds an arbitrary query parameter to the request URL.
func WithQuery(key, value string) RequestOption {
	return func(rc *requestConfig) {
		if rc.query == nil {
			rc.query = url.Values{}
		}
		rc.query.Set(key, value)
	}
}

// WithHeader sets an arbitrary request header.
func WithHeader(key, value string) RequestOption {
	return func(rc *requestConfig) {
		if rc.header == nil {
			rc.header = http.Header{}
		}
		rc.header.Set(key, value)
	}
}

// do performs an API request against /api/v1 + path, encoding body as JSON (when
// non-nil) and decoding the response into out (when non-nil).
func (c *Client) do(ctx context.Context, method, path string, body any, out any, opts []RequestOption) error {
	if c.apiKey == "" {
		return &Error{Message: "APIKey is required"}
	}
	if c.baseURL == "" {
		return &Error{Message: "BaseURL is required"}
	}

	rc := &requestConfig{}
	for _, o := range opts {
		o(rc)
	}

	u := c.baseURL + "/api/v1" + path
	if len(rc.query) > 0 {
		u += "?" + rc.query.Encode()
	}

	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("corgisign: encoding request body: %w", err)
		}
		reader = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return fmt.Errorf("corgisign: building request: %w", err)
	}
	for k, vs := range rc.header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	req.Header.Set(headerAPIKey, c.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if rc.idempotencyKey != "" {
		req.Header.Set(headerIdempotencyKey, rc.idempotencyKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("corgisign: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("corgisign: reading response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return newError(resp, respBody)
	}

	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("corgisign: decoding response: %w", err)
		}
	}
	return nil
}

// doDownload performs a GET against /api/v1 + path and returns the raw response
// body (e.g. a PAdES-sealed PDF). Non-2xx responses are returned as *Error, with
// any JSON error message parsed out.
func (c *Client) doDownload(ctx context.Context, path string, opts []RequestOption) ([]byte, error) {
	if c.apiKey == "" {
		return nil, &Error{Message: "APIKey is required"}
	}
	if c.baseURL == "" {
		return nil, &Error{Message: "BaseURL is required"}
	}

	rc := &requestConfig{}
	for _, o := range opts {
		o(rc)
	}

	u := c.baseURL + "/api/v1" + path
	if len(rc.query) > 0 {
		u += "?" + rc.query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("corgisign: building request: %w", err)
	}
	for k, vs := range rc.header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	req.Header.Set(headerAPIKey, c.apiKey)
	req.Header.Set("Accept", "application/pdf")
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("corgisign: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("corgisign: reading response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, newError(resp, body)
	}
	return body, nil
}

func newError(resp *http.Response, body []byte) *Error {
	e := &Error{StatusCode: resp.StatusCode, Body: body}
	var env struct {
		Error string `json:"error"`
	}
	if len(body) > 0 && json.Unmarshal(body, &env) == nil {
		e.Message = env.Error
	}
	if e.Message == "" {
		e.Message = http.StatusText(resp.StatusCode)
	}
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		if secs, err := strconv.Atoi(strings.TrimSpace(ra)); err == nil {
			e.RetryAfter = time.Duration(secs) * time.Second
		}
	}
	return e
}
