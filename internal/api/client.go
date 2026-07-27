// Package api implements a Meshy REST client for async generation tasks.
package api

import (
	"bufio"
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

const DefaultBaseURL = "https://api.meshy.ai"

// Client talks to the Meshy OpenAPI.
type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

// NewClient returns a client with defaults applied.
func NewClient(baseURL, apiKey string, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 120 * time.Second}
	}
	return &Client{
		BaseURL:    baseURL,
		APIKey:     apiKey,
		HTTPClient: httpClient,
	}
}

// CreateResult is the POST create response body.
type CreateResult struct {
	Result string `json:"result"`
}

// TaskError is nested under task_error on failed tasks.
type TaskError struct {
	Message string `json:"message"`
}

// Task is a generic Meshy task payload. Unknown fields are preserved via Raw.
type Task map[string]any

func (t Task) ID() string {
	if v, ok := t["id"].(string); ok {
		return v
	}
	return ""
}

func (t Task) Status() string {
	if v, ok := t["status"].(string); ok {
		return v
	}
	return ""
}

func (t Task) Progress() float64 {
	switch v := t["progress"].(type) {
	case float64:
		return v
	case json.Number:
		f, _ := v.Float64()
		return f
	}
	return 0
}

func (t Task) TaskErrorMessage() string {
	te, ok := t["task_error"].(map[string]any)
	if !ok {
		return ""
	}
	if m, ok := te["message"].(string); ok {
		return m
	}
	return ""
}

// Terminal returns true when status is SUCCEEDED, FAILED, or CANCELED.
func (t Task) Terminal() bool {
	switch t.Status() {
	case "SUCCEEDED", "FAILED", "CANCELED":
		return true
	}
	return false
}

// ModelURLs extracts model_urls if present.
func (t Task) ModelURLs() map[string]string {
	out := map[string]string{}
	mu, ok := t["model_urls"].(map[string]any)
	if !ok {
		return out
	}
	for k, v := range mu {
		if s, ok := v.(string); ok && s != "" {
			out[k] = s
		}
	}
	return out
}

// APIError is a non-2xx HTTP response from Meshy.
type APIError struct {
	StatusCode int
	Body       string
	Message    string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("meshy api error %d: %s", e.StatusCode, e.Message)
	}
	if e.Body != "" {
		return fmt.Sprintf("meshy api error %d: %s", e.StatusCode, truncate(e.Body, 400))
	}
	return fmt.Sprintf("meshy api error %d", e.StatusCode)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// doJSON performs an authenticated JSON request.
func (c *Client) doJSON(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	if c.APIKey == "" {
		return fmt.Errorf("missing API key: set --api-key, MESHY_API_KEY, or `meshy config set api_key <key>`")
	}
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		rdr = bytes.NewReader(b)
	}
	u := c.BaseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := ""
		var errBody map[string]any
		if json.Unmarshal(data, &errBody) == nil {
			if m, ok := errBody["message"].(string); ok {
				msg = m
			}
		}
		return &APIError{StatusCode: resp.StatusCode, Body: string(data), Message: msg}
	}
	if out == nil || len(data) == 0 || string(data) == "null" {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// Create posts a create body to path and returns the task id.
func (c *Client) Create(ctx context.Context, path string, body any) (string, error) {
	var res CreateResult
	if err := c.doJSON(ctx, http.MethodPost, path, nil, body, &res); err != nil {
		return "", err
	}
	if res.Result == "" {
		return "", fmt.Errorf("create response missing result id")
	}
	return res.Result, nil
}

// Get retrieves a task by path (including id).
func (c *Client) Get(ctx context.Context, path string) (Task, error) {
	var t Task
	if err := c.doJSON(ctx, http.MethodGet, path, nil, nil, &t); err != nil {
		return nil, err
	}
	return t, nil
}

// List retrieves a page of tasks.
func (c *Client) List(ctx context.Context, path string, pageNum, pageSize int) ([]Task, error) {
	q := url.Values{}
	if pageNum > 0 {
		q.Set("page_num", strconv.Itoa(pageNum))
	}
	if pageSize > 0 {
		q.Set("page_size", strconv.Itoa(pageSize))
	}
	var tasks []Task
	if err := c.doJSON(ctx, http.MethodGet, path, q, nil, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

// Delete deletes a task.
func (c *Client) Delete(ctx context.Context, path string) error {
	return c.doJSON(ctx, http.MethodDelete, path, nil, nil, nil)
}

// GetBalance returns account balance.
func (c *Client) GetBalance(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	if err := c.doJSON(ctx, http.MethodGet, "/openapi/v1/balance", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Download fetches a URL's body (e.g. model asset). Auth is not required for CDN URLs.
func (c *Client) Download(ctx context.Context, assetURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(data)}
	}
	return data, nil
}

// WaitOptions controls polling.
type WaitOptions struct {
	Interval time.Duration
	Timeout  time.Duration
	OnUpdate func(Task)
}

// Wait polls Get until terminal status or timeout.
func (c *Client) Wait(ctx context.Context, path string, opts WaitOptions) (Task, error) {
	if opts.Interval <= 0 {
		opts.Interval = 5 * time.Second
	}
	deadline := time.Time{}
	if opts.Timeout > 0 {
		deadline = time.Now().Add(opts.Timeout)
	}
	for {
		t, err := c.Get(ctx, path)
		if err != nil {
			return nil, err
		}
		if opts.OnUpdate != nil {
			opts.OnUpdate(t)
		}
		if t.Terminal() {
			return t, nil
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			return t, fmt.Errorf("wait timed out after %s (status=%s progress=%v)", opts.Timeout, t.Status(), t.Progress())
		}
		select {
		case <-ctx.Done():
			return t, ctx.Err()
		case <-time.After(opts.Interval):
		}
	}
}

// WaitStream connects to the SSE stream at path (usually …/:id/stream) until terminal status.
// Falls back to poll Wait if the stream cannot be opened.
func (c *Client) WaitStream(ctx context.Context, streamPath string, opts WaitOptions) (Task, error) {
	if c.APIKey == "" {
		return nil, fmt.Errorf("missing API key: set --api-key, MESHY_API_KEY, or `meshy config set api_key <key>`")
	}
	u := c.BaseURL + streamPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Accept", "text/event-stream")

	// Streaming needs a client without a short overall Timeout (or with long enough).
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	// If the shared client has a short Timeout, clone without it for SSE.
	if httpClient.Timeout > 0 && httpClient.Timeout < 30*time.Minute {
		clone := *httpClient
		clone.Timeout = 0
		httpClient = &clone
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		// fall back to polling the non-stream path
		base := strings.TrimSuffix(streamPath, "/stream")
		return c.Wait(ctx, base, opts)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		// Some resources may not support stream yet
		if resp.StatusCode == 404 || resp.StatusCode == 405 {
			base := strings.TrimSuffix(streamPath, "/stream")
			return c.Wait(ctx, base, opts)
		}
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(data)}
	}

	deadline := time.Time{}
	if opts.Timeout > 0 {
		deadline = time.Now().Add(opts.Timeout)
	}

	scanner := bufio.NewScanner(resp.Body)
	// SSE payloads can be large (full task objects)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 4*1024*1024)

	var last Task
	for scanner.Scan() {
		if !deadline.IsZero() && time.Now().After(deadline) {
			if last != nil {
				return last, fmt.Errorf("stream wait timed out after %s (status=%s)", opts.Timeout, last.Status())
			}
			return nil, fmt.Errorf("stream wait timed out after %s", opts.Timeout)
		}
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var t Task
		dec := json.NewDecoder(strings.NewReader(payload))
		dec.UseNumber()
		if err := dec.Decode(&t); err != nil {
			continue
		}
		last = t
		if opts.OnUpdate != nil {
			opts.OnUpdate(t)
		}
		if t.Terminal() {
			return t, nil
		}
	}
	if err := scanner.Err(); err != nil {
		if last != nil && last.Terminal() {
			return last, nil
		}
		// stream dropped mid-flight — poll once for final state
		base := strings.TrimSuffix(streamPath, "/stream")
		return c.Wait(ctx, base, opts)
	}
	if last != nil {
		if last.Terminal() {
			return last, nil
		}
		base := strings.TrimSuffix(streamPath, "/stream")
		return c.Wait(ctx, base, opts)
	}
	base := strings.TrimSuffix(streamPath, "/stream")
	return c.Wait(ctx, base, opts)
}
