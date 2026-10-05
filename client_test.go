package justpush

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const messageResponse = `{
	"user_uuid": "u1",
	"topic": {"uuid": "t1", "title": "Home", "slug": "home", "avatar": null,
	          "has_custom_avatar": false, "api_token": "tok"},
	"key": "abc123",
	"title": "Hi",
	"message": "There",
	"priority": 1,
	"sound": "default",
	"requires_acknowledgement": true,
	"acknowledgement": {"is_acknowledged": true},
	"pending_actions": 0,
	"received_at": "2026-09-30T10:00:00Z",
	"processed_at": "2026-09-30T10:00:01.123456Z",
	"expires_at": null
}`

const topicResponse = `{"uuid": "t1", "title": "Home", "slug": "home", "avatar": "https://x/a.png",
	"has_custom_avatar": "1", "api_token": "tok"}`

type recorded struct {
	Method string
	Path   string
	Header http.Header
	JSON   map[string]any
}

// fakeAPI is a local HTTP server that stands in for the JustPush API.
type fakeAPI struct {
	*httptest.Server
	mu       sync.Mutex
	requests []recorded
	status   int
	body     string
	headers  map[string]string
	delay    time.Duration
}

func newFakeAPI(t *testing.T) *fakeAPI {
	f := &fakeAPI{
		status: 201,
		body:   `{"status": 1, "key": "abc123"}`,
		headers: map[string]string{
			"X-Limit-App-Limit":     "10000",
			"X-Limit-App-Remaining": "9895",
			"X-Limit-App-Reset":     "234512",
		},
	}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		rec := recorded{Method: r.Method, Path: r.URL.EscapedPath(), Header: r.Header}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &rec.JSON)
		}
		f.mu.Lock()
		f.requests = append(f.requests, rec)
		status, body, headers, delay := f.status, f.body, f.headers, f.delay
		f.mu.Unlock()

		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
		}
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeAPI) respond(status int, body string, headers map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status, f.body = status, body
	if headers != nil {
		f.headers = headers
	}
}

func (f *fakeAPI) last() recorded {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[len(f.requests)-1]
}

func (f *fakeAPI) client(t *testing.T, opts ...Option) *Client {
	t.Helper()
	c, err := New(" tok ", append([]Option{WithBaseURL(f.URL + "/")}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

var ctx = context.Background()

func TestSend(t *testing.T) {
	api := newFakeAPI(t)
	result, err := api.client(t).Send(ctx, &Message{Message: "There", Title: "Hi", Priority: PriorityHigh})
	if err != nil {
		t.Fatal(err)
	}
	want := RateLimit{Known: true, Limit: 10000, Remaining: 9895, Reset: 234512 * time.Second}
	if result.Key != "abc123" || result.RateLimit != want {
		t.Fatalf("result = %+v", result)
	}
	req := api.last()
	if req.Method != "POST" || req.Path != "/messages" {
		t.Errorf("request = %s %s", req.Method, req.Path)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer tok" {
		t.Errorf("Authorization = %q", got)
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := req.Header.Get("User-Agent"); !strings.HasPrefix(got, "justpush-go/") {
		t.Errorf("User-Agent = %q", got)
	}
	assertJSON(t, req.JSON, `{"title": "Hi", "message": "There", "priority": 1}`)
}

func TestRateLimitHeadersMissing(t *testing.T) {
	api := newFakeAPI(t)
	api.respond(201, `{"key": "k"}`, map[string]string{})
	result, err := api.client(t).Send(ctx, &Message{Message: "x"})
	if err != nil || result.RateLimit.Known {
		t.Fatalf("result = %+v, %v", result, err)
	}
}

func TestSendWithoutKeyIsAnError(t *testing.T) {
	api := newFakeAPI(t)
	api.respond(201, `{"status": 1}`, nil)
	if _, err := api.client(t).Send(ctx, &Message{Message: "x"}); err == nil || !strings.Contains(err.Error(), "no key") {
		t.Fatalf("err = %v", err)
	}
}

func TestGetMessage(t *testing.T) {
	api := newFakeAPI(t)
	api.respond(200, messageResponse, nil)
	details, err := api.client(t).GetMessage(ctx, "abc 123/x")
	if err != nil {
		t.Fatal(err)
	}
	if got := api.last().Path; got != "/messages/abc%20123%2Fx" {
		t.Errorf("path = %q", got)
	}
	if details.Key != "abc123" || !details.IsAcknowledged || !details.RequiresAcknowledgement ||
		details.Priority != PriorityHigh || details.Sound != SoundDefault ||
		details.Topic == nil || details.Topic.APIToken != "tok" {
		t.Errorf("details = %+v", details)
	}
	if !details.ReceivedAt.Equal(time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)) ||
		details.ProcessedAt.Nanosecond() != 123456000 || !details.ExpiresAt.IsZero() {
		t.Errorf("times = %v %v %v", details.ReceivedAt, details.ProcessedAt, details.ExpiresAt)
	}
	if !strings.Contains(string(details.Raw), `"user_uuid": "u1"`) {
		t.Errorf("raw = %s", details.Raw)
	}
}

func TestTopics(t *testing.T) {
	api := newFakeAPI(t)
	client := api.client(t)

	api.respond(201, topicResponse, nil)
	topic, err := client.CreateTopic(ctx, TopicInput{Title: "Home", AvatarURL: "https://example.com/a.png"})
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, api.last().JSON, `{"title": "Home", "avatar": {"external_url": "https://example.com/a.png"}}`)
	if topic.UUID != "t1" || topic.APIToken != "tok" || !topic.HasCustomAvatar {
		t.Errorf("topic = %+v", topic)
	}

	api.respond(200, `{"data": `+topicResponse+`}`, nil)
	topic, err = client.GetTopic(ctx, "t1")
	if err != nil || topic.Title != "Home" {
		t.Fatalf("GetTopic = %+v, %v", topic, err)
	}
	if req := api.last(); req.Method != "GET" || req.Path != "/topics/t1" {
		t.Errorf("request = %s %s", req.Method, req.Path)
	}

	api.respond(200, topicResponse, nil)
	if _, err := client.UpdateTopic(ctx, "t1", TopicInput{Title: "New"}); err != nil {
		t.Fatal(err)
	}
	if req := api.last(); req.Method != "PUT" {
		t.Errorf("method = %s", req.Method)
	}
	assertJSON(t, api.last().JSON, `{"title": "New"}`)

	if _, err := client.GetTopic(ctx, " "); !errors.Is(err, ErrValidation) {
		t.Errorf("empty uuid err = %v", err)
	}
}

func TestErrors(t *testing.T) {
	cases := []struct {
		status  int
		body    string
		target  error
		message string
	}{
		{401, `{"error": {"type": "unauthorized", "message": "Unauthenticated"}}`, ErrUnauthorized, "Unauthenticated"},
		{403, `{"error": "Not your topic", "code": 403}`, ErrForbidden, "Not your topic"},
		{404, `{"error": {"type": "not_found", "message": "Message not found"}}`, ErrNotFound, "Message not found"},
		{410, `{"error": {"type": "subscription_expired", "message": "Expired"}}`, ErrSubscriptionExpired, "Expired"},
		{422, `{"message": "The sound is invalid.", "errors": {"sound": ["The sound is invalid."]}}`, ErrValidation, "The sound is invalid."},
		{429, `{"message": "Too Many Attempts."}`, ErrRateLimited, "Too Many Attempts."},
		{500, `<html>oops</html>`, nil, "JustPush API returned HTTP 500"},
	}
	for _, tc := range cases {
		api := newFakeAPI(t)
		api.respond(tc.status, tc.body, map[string]string{"Retry-After": "30"})
		_, err := api.client(t).Send(ctx, &Message{Message: "x"})

		var apiErr *APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("%d: err = %T %v, want *APIError", tc.status, err, err)
		}
		if tc.target != nil && !errors.Is(err, tc.target) {
			t.Errorf("%d: errors.Is(%v, %v) = false", tc.status, err, tc.target)
		}
		if apiErr.StatusCode != tc.status || apiErr.Message != tc.message {
			t.Errorf("%d: got status %d message %q", tc.status, apiErr.StatusCode, apiErr.Message)
		}
		if tc.status == 422 && apiErr.Errors["sound"][0] != "The sound is invalid." {
			t.Errorf("422 errors = %v", apiErr.Errors)
		}
		if tc.status == 429 && apiErr.RetryAfter != 30*time.Second {
			t.Errorf("429 RetryAfter = %v", apiErr.RetryAfter)
		}
	}
}

func TestConnectionError(t *testing.T) {
	client, _ := New("tok", WithBaseURL("http://127.0.0.1:9"))
	if _, err := client.Send(ctx, &Message{Message: "x"}); !errors.Is(err, ErrConnection) {
		t.Fatalf("err = %v, want ErrConnection", err)
	}
}

func TestTimeout(t *testing.T) {
	api := newFakeAPI(t)
	api.delay = 500 * time.Millisecond
	_, err := api.client(t, WithTimeout(50*time.Millisecond)).Send(ctx, &Message{Message: "x"})
	if !errors.Is(err, ErrConnection) {
		t.Fatalf("err = %v, want ErrConnection", err)
	}
}

func TestContextDeadline(t *testing.T) {
	api := newFakeAPI(t)
	api.delay = 500 * time.Millisecond
	short, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	_, err := api.client(t).Send(short, &Message{Message: "x"})
	if !errors.Is(err, ErrConnection) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want ErrConnection and DeadlineExceeded", err)
	}
}

func TestWithTimeoutDoesNotChangeSharedClient(t *testing.T) {
	shared := &http.Client{Timeout: time.Minute}
	_, _ = New("tok", WithHTTPClient(shared), WithTimeout(time.Second))
	if shared.Timeout != time.Minute {
		t.Fatalf("shared client timeout changed to %v", shared.Timeout)
	}
}

func TestValidationHappensBeforeAnyRequest(t *testing.T) {
	api := newFakeAPI(t)
	_, err := api.client(t).Send(ctx, &Message{Message: "x", Priority: 9})
	var vErr *ValidationError
	if !errors.As(err, &vErr) {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
	if len(api.requests) != 0 {
		t.Fatalf("made %d requests", len(api.requests))
	}
}

func TestVerifyToken(t *testing.T) {
	api := newFakeAPI(t)
	api.respond(404, `{"error": {"type": "not_found", "message": "Message not found"}}`, nil)
	if err := api.client(t).VerifyToken(ctx); err != nil {
		t.Fatal(err)
	}
	if req := api.last(); req.Method != "GET" || !strings.HasPrefix(req.Path, "/messages/verify-") {
		t.Errorf("request = %s %s", req.Method, req.Path)
	}

	api.respond(401, `{"error": {"type": "unauthorized", "message": "Unauthenticated"}}`, nil)
	if err := api.client(t).VerifyToken(ctx); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}
