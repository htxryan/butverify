// Package api is the bv CLI's HTTP client for the butverify.dev control plane.
//
// Design notes:
//
//   - Stdlib net/http only — no third-party retry libs. The cases that
//     require retry (token refresh after 401, idempotent push) are bespoke
//     enough that a generic retry framework would obscure intent.
//   - User-Agent is `bv/<version> <os>-<arch>`. The control plane reads
//     this for HTTP 426 Upgrade Required (E7) and for analytics.
//   - Accept: application/json by default; the CLI's --json mode adds
//     no behavior here (the server always returns JSON for non-streaming
//     endpoints).
//   - On 426 the client surfaces the structured details (min_version,
//     download_url) so the caller can route to self-update.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"time"
)

// Client is the bv CLI's API client. Construct with New().
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
	Version    string
	UserAgent  string
}

// New constructs a Client with sensible defaults.
func New(baseURL, token, version string) *Client {
	ua := fmt.Sprintf("bv/%s %s-%s", version, runtime.GOOS, runtime.GOARCH)
	return &Client{
		BaseURL:    baseURL,
		Token:      token,
		Version:    version,
		UserAgent:  ua,
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// APIError mirrors the server's error envelope. The Status carries the
// HTTP status code (so a caller can branch on 401/402/426/etc.) and Details
// surfaces the structured payload (e.g. UPGRADE_REQUIRED's min_version).
type APIError struct {
	Status    int            `json:"-"`
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	RequestID string         `json:"request_id,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
	// Raw is the bytes the server returned, for debugging non-JSON 5xx.
	Raw []byte `json:"-"`
}

func (e *APIError) Error() string {
	if e.RequestID != "" {
		return fmt.Sprintf("api %d %s: %s (request_id=%s)", e.Status, e.Code, e.Message, e.RequestID)
	}
	return fmt.Sprintf("api %d %s: %s", e.Status, e.Code, e.Message)
}

// IsUpgradeRequired reports whether err is a 426 from the server.
func IsUpgradeRequired(err error) bool {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae.Status == http.StatusUpgradeRequired || ae.Code == "UPGRADE_REQUIRED"
	}
	return false
}

// IsUnauthenticated reports whether err is a 401 from the server (token
// expired / invalid). The caller routes to a refresh-and-retry path.
func IsUnauthenticated(err error) bool {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae.Status == http.StatusUnauthorized
	}
	return false
}

// IsPaymentRequired reports whether err is a 402 (quota or billing gate).
func IsPaymentRequired(err error) bool {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae.Status == http.StatusPaymentRequired
	}
	return false
}

// IsNotFound reports whether err is a 404.
func IsNotFound(err error) bool {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae.Status == http.StatusNotFound
	}
	return false
}

// IsConflict reports whether err is a 409.
func IsConflict(err error) bool {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae.Status == http.StatusConflict
	}
	return false
}

// Do issues a request and decodes a JSON body into out (if non-nil). On a
// non-2xx response the body is parsed into an APIError. Network errors are
// returned as-is; the caller decides retry policy.
func (c *Client) Do(ctx context.Context, method, path string, body any, out any) error {
	req, err := c.newRequest(ctx, method, path, body)
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("api: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("api: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		ae := parseError(resp.StatusCode, respBody)
		return ae
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("api: decode response: %w", err)
		}
	}
	return nil
}

// DoRaw issues a request and returns the raw response body + status. Used
// for streaming endpoints (manifest, files/<path>) where the caller wants
// the bytes verbatim.
func (c *Client) DoRaw(ctx context.Context, method, path string, body any) (*http.Response, error) {
	req, err := c.newRequest(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("api: %s %s: %w", method, path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, parseError(resp.StatusCode, respBody)
	}
	return resp, nil
}

func (c *Client) newRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("api: parse base URL %q: %w", c.BaseURL, err)
	}
	// `path` may include a query string (e.g. "/v1/reviews?unacknowledged=true").
	// Parse it as a reference URL so Path and RawQuery split correctly —
	// otherwise the resolver treats the literal `?` as part of Path and
	// percent-encodes it, breaking server-side route matching.
	ref, err := url.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("api: parse path %q: %w", path, err)
	}
	u = u.ResolveReference(ref)
	var reader io.Reader
	if body != nil {
		switch b := body.(type) {
		case []byte:
			reader = bytes.NewReader(b)
		case io.Reader:
			reader = b
		default:
			data, err := json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("api: marshal body: %w", err)
			}
			reader = bytes.NewReader(data)
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), reader)
	if err != nil {
		return nil, fmt.Errorf("api: new request: %w", err)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if body != nil {
		// JSON by default; callers writing []byte/io.Reader can override
		// via SetHeader before invoking. (DoRaw doesn't expose that yet;
		// add when the first non-JSON body endpoint shows up.)
		if _, ok := body.([]byte); !ok {
			if _, ok := body.(io.Reader); !ok {
				req.Header.Set("Content-Type", "application/json")
			}
		}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.UserAgent)
	return req, nil
}

func parseError(status int, body []byte) *APIError {
	ae := &APIError{Status: status, Raw: body}
	var env struct {
		Error APIError `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err == nil && env.Error.Code != "" {
		ae.Code = env.Error.Code
		ae.Message = env.Error.Message
		ae.RequestID = env.Error.RequestID
		ae.Details = env.Error.Details
		return ae
	}
	// Non-JSON body (e.g. an upstream gateway error). Surface the status
	// code with a generic message so the CLI doesn't lose context.
	ae.Code = "UNKNOWN"
	ae.Message = fmt.Sprintf("non-JSON response from server (status %d)", status)
	if len(body) > 0 && len(body) < 256 {
		ae.Message = string(body)
	}
	return ae
}
