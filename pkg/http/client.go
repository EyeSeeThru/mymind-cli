package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"mymind/pkg/auth"
	"mymind/pkg/errors"
)

const (
	BaseURL        = "https://api.mymind.com"
	UserAgentPrefix = "mymind-cli/"
	MaxWaitSeconds  = 30
)

// QueryValue is a query parameter value.
type QueryValue interface{}

// Client is the MyMind HTTP client.
type Client struct {
	Creds     *auth.Credentials
	UserAgent string
	Verbose   bool
}

// New creates a new HTTP client.
func New(creds *auth.Credentials) *Client {
	return &Client{Creds: creds, UserAgent: UserAgentPrefix + "0.0.0"}
}

// RequestOptions configures an HTTP request.
type RequestOptions struct {
	Body        interface{}
	ContentType string
	Accept      string
	Query       map[string]QueryValue
	DryRun      bool
}

// DryRunResult is returned when --dry-run is active.
type DryRunResult struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    string
}

// IsDryRun marks this as a dry-run result.
func (d DryRunResult) IsDryRun() {}

// RequestRaw returns the raw *http.Response for streaming/binary operations.
func (c *Client) RequestRaw(method, path string, opts RequestOptions) (*DryRunResult, *http.Response, error) {
	// If dry-run with no creds, return preview
	if opts.DryRun && c.Creds == nil {
		query := BuildQuery(opts.Query)
		url := BaseURL + path + query
		headers := map[string]string{
			"Authorization": "<signed-jwt redacted (no credentials)>",
			"User-Agent":    c.UserAgent,
		}
		if opts.Accept != "" {
			headers["Accept"] = opts.Accept
		} else {
			headers["Accept"] = "application/json"
		}
		return &DryRunResult{Method: method, URL: url, Headers: headers, Body: ""}, nil, nil
	}
	query := BuildQuery(opts.Query)
	url := BaseURL + path + query

	jwt, err := auth.SignRequest(method, path, c.Creds)
	if err != nil {
		return nil, nil, fmt.Errorf("signing request: %w", err)
	}

	headers := map[string]string{
		"Authorization": "Bearer " + jwt,
		"User-Agent":    c.UserAgent,
	}
	if opts.Accept != "" {
		headers["Accept"] = opts.Accept
	} else {
		headers["Accept"] = "application/json"
	}

	if opts.DryRun {
		redacted := make(map[string]string)
		for k, v := range headers {
			if k == "Authorization" {
				v = "<signed-jwt redacted>"
			}
			redacted[k] = v
		}
		return &DryRunResult{Method: method, URL: url, Headers: redacted, Body: ""}, nil, nil
	}

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, nil, &errors.ErrNetwork{Err: err}
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, &errors.ErrNetwork{Err: err}
	}

	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, nil, &errors.ApiError{
			Type:   fmt.Sprintf("HTTP%d", resp.StatusCode),
			Status: resp.StatusCode,
			Detail: fmt.Sprintf("HTTP error %d", resp.StatusCode),
		}
	}

	return nil, resp, nil
}

// BuildQuery encodes query params to a URL string.
func BuildQuery(query map[string]QueryValue) string {
	if len(query) == 0 {
		return ""
	}
	var params []string
	for k, v := range query {
		if v == nil || v == "" {
			continue
		}
		switch vv := v.(type) {
		case []string:
			for _, item := range vv {
				params = append(params, fmt.Sprintf("%s=%s", url.QueryEscape(k), url.QueryEscape(item)))
			}
		case int:
			if vv != 0 {
				params = append(params, fmt.Sprintf("%s=%d", url.QueryEscape(k), vv))
			}
		case bool:
			params = append(params, fmt.Sprintf("%s=%t", url.QueryEscape(k), vv))
		default:
			params = append(params, fmt.Sprintf("%s=%s", url.QueryEscape(k), url.QueryEscape(fmt.Sprintf("%v", vv))))
		}
	}
	if len(params) == 0 {
		return ""
	}
	return "?" + strings.Join(params, "&")
}

func reqID(headers http.Header) string {
	for _, h := range []string{"x-request-id", "request-id", "x-trace-id"} {
		if v := headers.Get(h); v != "" {
			return v
		}
	}
	return ""
}

// parseRateLimit parses the RateLimit header: "<name>";r=<remaining>;t=<seconds>
func parseRateLimit(hdr string) []struct{ Name string; R, T int } {
	if hdr == "" {
		return nil
	}
	var policies []struct{ Name string; R, T int }
	re := regexp.MustCompile(`"([^"]+)"|(\S+)|(r=\d+)|(t=\d+)`)
	for _, part := range strings.Split(hdr, ",") {
		part = strings.TrimSpace(part)
		matches := re.FindAllStringSubmatch(part, -1)
		if len(matches) == 0 {
			continue
		}
		name := ""
		r, t := 0, 0
		for _, m := range matches {
			if m[1] != "" {
				name = m[1]
			} else if m[3] != "" {
				fmt.Sscanf(m[3][2:], "%d", &r)
			} else if m[4] != "" {
				fmt.Sscanf(m[4][2:], "%d", &t)
			}
		}
		if name != "" {
			policies = append(policies, struct{ Name string; R, T int }{Name: name, R: r, T: t})
		}
	}
	return policies
}

type rateLimitPolicy struct {
	Name string
	T    int
}

func maxWait(policies []struct{ Name string; R, T int }) (wait int, exhausted []rateLimitPolicy) {
	for _, p := range policies {
		if p.R <= 0 {
			exhausted = append(exhausted, rateLimitPolicy{Name: p.Name, T: p.T})
			if p.T > wait {
				wait = p.T
			}
		}
	}
	return
}

// Request performs an authenticated request with automatic rate-limit retry.
func (c *Client) Request(method, path string, opts RequestOptions) (interface{}, error) {
	// If dry-run with no creds, build preview without signing
	if opts.DryRun && c.Creds == nil {
		return c.dryRunPreview(method, path, opts)
	}
	return c.doRequest(method, path, opts, true)
}

// dryRunPreview generates a dry-run result without credentials.
func (c *Client) dryRunPreview(method, path string, opts RequestOptions) (interface{}, error) {
	query := BuildQuery(opts.Query)
	url := BaseURL + path + query

	headers := map[string]string{
		"Authorization": "<signed-jwt redacted (no credentials)>",
		"User-Agent":    c.UserAgent,
	}
	if opts.ContentType != "" {
		headers["Content-Type"] = opts.ContentType
	}
	if opts.Accept != "" {
		headers["Accept"] = opts.Accept
	} else {
		headers["Accept"] = "application/json"
	}

	var bodyPreview string
	if opts.Body != nil {
		switch b := opts.Body.(type) {
		case string:
			bodyPreview = b
		case []byte:
			bodyPreview = string(b)
		default:
			bodyPreview = fmt.Sprintf("%v", b)
		}
	}
	return DryRunResult{Method: method, URL: url, Headers: headers, Body: bodyPreview}, nil
}

func (c *Client) doRequest(method, path string, opts RequestOptions, retrying bool) (interface{}, error) {
	query := BuildQuery(opts.Query)
	url := BaseURL + path + query

	var bodyReader io.Reader
	if opts.Body != nil {
		switch b := opts.Body.(type) {
		case string:
			bodyReader = strings.NewReader(b)
		case []byte:
			bodyReader = bytes.NewReader(b)
		default:
			bodyReader = strings.NewReader(fmt.Sprintf("%v", b))
		}
	}

	jwt, err := auth.SignRequest(method, path, c.Creds)
	if err != nil {
		return nil, fmt.Errorf("signing request: %w", err)
	}

	headers := map[string]string{
		"Authorization": "Bearer " + jwt,
		"User-Agent":    c.UserAgent,
	}
	if opts.ContentType != "" {
		headers["Content-Type"] = opts.ContentType
	}
	if opts.Accept != "" {
		headers["Accept"] = opts.Accept
	} else {
		headers["Accept"] = "application/json"
	}

	if opts.DryRun {
		redacted := make(map[string]string)
		for k, v := range headers {
			if k == "Authorization" {
				v = "<signed-jwt redacted>"
			}
			redacted[k] = v
		}
		var bodyPreview string
		if bodyReader != nil {
			data, _ := io.ReadAll(bodyReader)
			bodyPreview = string(data)
			if s, ok := bodyReader.(io.Seeker); ok {
				s.Seek(0, 0)
			}
		}
		return DryRunResult{Method: method, URL: url, Headers: redacted, Body: bodyPreview}, nil
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, &errors.ErrNetwork{Err: err}
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	if c.Verbose {
		fmt.Fprintf(os.Stderr, "--> %s %s\n", method, url)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, &errors.ErrNetwork{Err: err}
	}
	defer resp.Body.Close()

	rid := reqID(resp.Header)

	if c.Verbose {
		fmt.Fprintf(os.Stderr, "<-- %d %s (requestId=%s)\n", resp.StatusCode, resp.Status, rid)
	}

	// Handle rate limiting
	if resp.StatusCode == 429 {
		policies := parseRateLimit(resp.Header.Get("RateLimit"))
		wait, exhausted := maxWait(policies)
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if n, _ := strconv.Atoi(ra); n > wait {
				wait = n
			}
		}

		exhaustedPols := make([]errors.ExhaustedPolicy, len(exhausted))
		for i, e := range exhausted {
			exhaustedPols[i] = errors.ExhaustedPolicy{Name: e.Name, T: e.T}
		}

		if wait > 0 && wait <= MaxWaitSeconds && !retrying {
			io.Copy(io.Discard, resp.Body)
			time.Sleep(time.Duration(wait) * time.Second)
			return c.doRequest(method, path, opts, true)
		}

		apiErr := &errors.ApiError{
			Type:              "RateLimited",
			Status:            429,
			Detail:            fmt.Sprintf("Rate limit exceeded. Wait %ds.", wait),
			RequestID:         rid,
			RetryAfter:        &wait,
			ExhaustedPolicies: exhaustedPols,
		}
		return nil, apiErr
	}

	if resp.StatusCode >= 400 {
		var detail string
		ct := resp.Header.Get("Content-Type")
		if strings.Contains(ct, "problem+json") || strings.Contains(ct, "json") {
			var body map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&body); err == nil {
				if d, ok := body["detail"].(string); ok {
					detail = d
				} else if t, ok := body["type"].(string); ok {
					detail = t
				}
			}
		}
		if detail == "" {
			data, _ := io.ReadAll(resp.Body)
			detail = strings.TrimSpace(string(data))
		}
		return nil, &errors.ApiError{
			Type:      fmt.Sprintf("HTTP%d", resp.StatusCode),
			Status:    resp.StatusCode,
			Detail:    detail,
			RequestID: rid,
		}
	}

	if resp.StatusCode == 204 {
		return nil, nil
	}

	var out interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		data, _ := io.ReadAll(resp.Body)
		return string(data), nil
	}
	return out, nil
}
