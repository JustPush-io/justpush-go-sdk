// Package justpush is the official Go SDK for JustPush (https://justpush.io).
// It sends push notifications to your iOS and Android devices.
//
//	client, err := justpush.New("YOUR_API_TOKEN")
//	if err != nil { ... }
//	result, err := client.Send(ctx, &justpush.Message{Title: "Backups", Message: "The nightly backup finished"})
package justpush

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// Version of this SDK, sent in the User-Agent header.
	Version = "1.0.1"
	// DefaultBaseURL is the JustPush API.
	DefaultBaseURL = "https://api.justpush.io"
	// DefaultTimeout applies when the client creates its own http.Client.
	DefaultTimeout = 10 * time.Second

	userAgent = "justpush-go/" + Version
)

// Client sends push notifications through the JustPush API. It is safe for concurrent use.
type Client struct {
	token   string
	baseURL string
	http    *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL points the client at another API host, e.g. a test server.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(baseURL, "/") }
}

// WithHTTPClient uses your own http.Client, e.g. for proxies or tracing.
// Its timeout applies instead of DefaultTimeout.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		if hc != nil {
			c.http = hc
		}
	}
}

// WithTimeout sets how long a request may take. Use a context deadline for per-call limits.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		hc := *c.http // copy, so a shared client passed to WithHTTPClient isn't changed
		hc.Timeout = d
		c.http = &hc
	}
}

// New creates a client for the given API token. Get your token from the JustPush app.
func New(token string, opts ...Option) (*Client, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, invalid("a JustPush API token is required")
	}
	c := &Client{
		token:   token,
		baseURL: DefaultBaseURL,
		http:    &http.Client{Timeout: DefaultTimeout},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// Send queues a push notification and returns its key. The message is checked first, so an
// invalid one returns a *ValidationError without making a request.
func (c *Client) Send(ctx context.Context, m *Message) (*SendResult, error) {
	body, err := m.payload()
	if err != nil {
		return nil, err
	}
	var created struct {
		Key string `json:"key"`
	}
	header, err := c.do(ctx, http.MethodPost, "/messages", body, &created)
	if err != nil {
		return nil, err
	}
	if created.Key == "" {
		return nil, errors.New("justpush: the API accepted the message but returned no key")
	}
	return &SendResult{Key: created.Key, RateLimit: rateLimitFromHeaders(header)}, nil
}

// GetMessage fetches a message you sent, e.g. to check whether it was acknowledged.
func (c *Client) GetMessage(ctx context.Context, key string) (*MessageDetails, error) {
	path, err := pathWith("/messages/", key, "key")
	if err != nil {
		return nil, err
	}
	var details MessageDetails
	if _, err := c.do(ctx, http.MethodGet, path, nil, &details); err != nil {
		return nil, err
	}
	return &details, nil
}

// VerifyToken checks that the token is valid without sending anything or using quota.
// It returns an error matching ErrUnauthorized for an invalid token.
func (c *Client) VerifyToken(ctx context.Context) error {
	// There is no account endpoint. Ask for a message that can't exist: the API answers
	// 404 for a valid token and 401 otherwise.
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	_, err := c.do(ctx, http.MethodGet, "/messages/verify-"+hex.EncodeToString(nonce), nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// CreateTopic creates a topic. Title is required.
func (c *Client) CreateTopic(ctx context.Context, in TopicInput) (*Topic, error) {
	body, err := in.payload(true)
	if err != nil {
		return nil, err
	}
	var topic Topic
	if _, err := c.do(ctx, http.MethodPost, "/topics", body, &topic); err != nil {
		return nil, err
	}
	return &topic, nil
}

// GetTopic fetches one of your topics by its UUID.
func (c *Client) GetTopic(ctx context.Context, uuid string) (*Topic, error) {
	path, err := pathWith("/topics/", uuid, "uuid")
	if err != nil {
		return nil, err
	}
	var topic Topic
	if _, err := c.do(ctx, http.MethodGet, path, nil, &topic); err != nil {
		return nil, err
	}
	return &topic, nil
}

// UpdateTopic renames a topic or changes its avatar. Empty fields are left unchanged.
func (c *Client) UpdateTopic(ctx context.Context, uuid string, in TopicInput) (*Topic, error) {
	path, err := pathWith("/topics/", uuid, "uuid")
	if err != nil {
		return nil, err
	}
	body, err := in.payload(false)
	if err != nil {
		return nil, err
	}
	var topic Topic
	if _, err := c.do(ctx, http.MethodPut, path, body, &topic); err != nil {
		return nil, err
	}
	return &topic, nil
}

func pathWith(prefix, value, name string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", invalid("%s is required", name)
	}
	return prefix + url.PathEscape(value), nil
}

// do sends a request and decodes a successful JSON response into out (when non-nil).
func (c *Client) do(ctx context.Context, method, path string, in, out any) (http.Header, error) {
	var reqBody io.Reader
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, &ConnectionError{Err: err}
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &ConnectionError{Err: err}
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, apiError(resp, raw)
	}
	if out != nil {
		if err := json.Unmarshal(unwrapData(raw), out); err != nil {
			return nil, fmt.Errorf("justpush: unexpected response from the API: %w", err)
		}
	}
	return resp.Header, nil
}

// unwrapData returns the "data" object Laravel resources are sometimes wrapped in, or raw itself.
func unwrapData(raw []byte) []byte {
	var wrapped struct {
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(raw, &wrapped) == nil && len(wrapped.Data) > 0 && wrapped.Data[0] == '{' {
		return wrapped.Data
	}
	return raw
}

func apiError(resp *http.Response, raw []byte) *APIError {
	e := &APIError{
		StatusCode: resp.StatusCode,
		Message:    fmt.Sprintf("JustPush API returned HTTP %d", resp.StatusCode),
		Body:       raw,
	}

	// The API has several error shapes:
	// {"error": {"message": "..."}}, {"error": "..."} and {"message": "...", "errors": {...}}.
	var body struct {
		Error   json.RawMessage     `json:"error"`
		Message string              `json:"message"`
		Errors  map[string][]string `json:"errors"`
	}
	if json.Unmarshal(raw, &body) == nil {
		var nested struct {
			Message string `json:"message"`
		}
		var plain string
		switch {
		case json.Unmarshal(body.Error, &nested) == nil && nested.Message != "":
			e.Message = nested.Message
		case json.Unmarshal(body.Error, &plain) == nil && plain != "":
			e.Message = plain
		case body.Message != "":
			e.Message = body.Message
		}
		e.Errors = body.Errors
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		if seconds, err := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After"))); err == nil {
			e.RetryAfter = time.Duration(seconds) * time.Second
		}
	}
	return e
}
