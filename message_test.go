package justpush

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// Request bodies must match what the backend's MessageCreateRequest accepts.

func payloadJSON(t *testing.T, m *Message) map[string]any {
	t.Helper()
	body, err := m.payload()
	if err != nil {
		t.Fatalf("payload: %v", err)
	}
	data, _ := json.Marshal(body)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	return out
}

func assertJSON(t *testing.T, got map[string]any, want string) {
	t.Helper()
	var w map[string]any
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("bad want JSON: %v", err)
	}
	if !reflect.DeepEqual(got, w) {
		g, _ := json.MarshalIndent(got, "", "  ")
		t.Fatalf("body mismatch\n got: %s\nwant: %s", g, want)
	}
}

func TestMinimalMessage(t *testing.T) {
	assertJSON(t, payloadJSON(t, &Message{Message: "Hi"}), `{"message": "Hi"}`)
	assertJSON(t, payloadJSON(t, &Message{Title: "T"}), `{"title": "T"}`)
}

func TestNeedsMessageOrTitle(t *testing.T) {
	for _, m := range []*Message{nil, {}, {Topic: "Home"}} {
		if _, err := m.payload(); !errors.Is(err, ErrValidation) {
			t.Errorf("payload(%+v) = %v, want ErrValidation", m, err)
		}
	}
}

func TestFullMessageUsesBackendFieldNames(t *testing.T) {
	got := payloadJSON(t, &Message{
		Message:  "Water detected",
		Title:    "Leak",
		Topic:    "Home",
		Priority: PriorityHighest,
		Sound:    "SIREN",
		Buttons:  []Button{{CTA: "Open", URL: "https://example.com", ActionRequired: true}},
		ButtonGroups: []ButtonGroup{
			{Name: "Links", CTA: "More", Buttons: []Button{{CTA: "A", URL: "https://a.example"}}},
		},
		Images: []Image{
			ImageFromURL("https://example.com/a.jpg", "Cam"),
			ImageFromBytes([]byte("\x89PNG"), ""),
		},
		Expiry: time.Minute,
		Acknowledge: &Acknowledgement{
			Retry: true, Interval: 30 * time.Second, MaxRetries: 5,
			CallbackURL: "https://example.com/cb", CallbackParams: map[string]any{"a": 1},
		},
	})
	assertJSON(t, got, `{
		"title": "Leak",
		"message": "Water detected",
		"topic": "Home",
		"priority": 2,
		"sound": "siren",
		"expiry_ttl": 60,
		"buttons": [{"cta": "Open", "url": "https://example.com", "action_required": true}],
		"button_groups": [
			{"name": "Links", "cta": "More", "action_required": false,
			 "buttons": [{"cta": "A", "url": "https://a.example"}]}
		],
		"images": [
			{"url": "https://example.com/a.jpg", "caption": "Cam"},
			{"body": "iVBORw=="}
		],
		"requires_acknowledgement": true,
		"acknowledgement": {
			"requires_retry": true,
			"interval": 30,
			"max_retries": 5,
			"callback": {"required": true, "url": "https://example.com/cb", "params": "{\"a\":1}"}
		}
	}`)
}

func TestAcknowledgeWithoutDetails(t *testing.T) {
	got := payloadJSON(t, &Message{Message: "x", Acknowledge: &Acknowledgement{}})
	assertJSON(t, got, `{"message": "x", "requires_acknowledgement": true}`)
}

func TestTopicToken(t *testing.T) {
	assertJSON(t, payloadJSON(t, &Message{Message: "x", TopicToken: "tok"}), `{"message": "x", "topic_token": "tok"}`)
}

func TestParsePriority(t *testing.T) {
	cases := map[string]Priority{"2": 2, "-2": -2, "high": 1, "LOWEST": -2, " normal ": 0, "-1": -1}
	for in, want := range cases {
		got, err := ParsePriority(in)
		if err != nil || got != want {
			t.Errorf("ParsePriority(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"3", "-3", "loud", "", "1.5"} {
		if _, err := ParsePriority(in); !errors.Is(err, ErrValidation) {
			t.Errorf("ParsePriority(%q) err = %v, want ErrValidation", in, err)
		}
	}
	if PriorityHigh.String() != "high" {
		t.Errorf("PriorityHigh.String() = %q", PriorityHigh.String())
	}
}

func TestPriorityRange(t *testing.T) {
	for _, p := range []Priority{3, -3} {
		if _, err := (&Message{Message: "x", Priority: p}).payload(); !errors.Is(err, ErrValidation) {
			t.Errorf("priority %d: err = %v, want ErrValidation", p, err)
		}
	}
	got := payloadJSON(t, &Message{Message: "x", Priority: PriorityLow})
	assertJSON(t, got, `{"message": "x", "priority": -1}`)
}

func TestSounds(t *testing.T) {
	if got := payloadJSON(t, &Message{Message: "x", Sound: SoundCashRegister})["sound"]; got != "cashregister" {
		t.Errorf("sound = %v", got)
	}
	if got := payloadJSON(t, &Message{Message: "x", Sound: "Cosmic"})["sound"]; got != "cosmic" {
		t.Errorf("sound = %v", got)
	}
	if _, err := (&Message{Message: "x", Sound: "kazoo"}).payload(); !errors.Is(err, ErrValidation) {
		t.Errorf("unknown sound err = %v", err)
	}
}

func TestLimits(t *testing.T) {
	button := Button{CTA: "b", URL: "https://x"}
	cases := map[string]Message{
		"11 buttons":           {Buttons: repeat(button, 11)},
		"5 groups":             {ButtonGroups: repeat(ButtonGroup{Name: "g", CTA: "c"}, 5)},
		"11 buttons in group":  {ButtonGroups: []ButtonGroup{{Name: "g", CTA: "c", Buttons: repeat(button, 11)}}},
		"11 images":            {Images: repeat(ImageFromURL("https://x", ""), 11)},
		"negative expiry":      {Expiry: -time.Second},
		"topic and token":      {Topic: "a", TopicToken: "b"},
		"short interval":       {Acknowledge: &Acknowledgement{Retry: true, Interval: 5 * time.Second}},
		"too many retries":     {Acknowledge: &Acknowledgement{Retry: true, MaxRetries: 256}},
		"image without source": {Images: []Image{{Caption: "c"}}},
		"image with both":      {Images: []Image{{URL: "https://x", Body: "abc"}}},
		"bad callback params":  {Acknowledge: &Acknowledgement{CallbackURL: "u", CallbackParams: map[string]any{"f": func() {}}}},
	}
	for name, m := range cases {
		m.Message = "x"
		if _, err := m.payload(); !errors.Is(err, ErrValidation) {
			t.Errorf("%s: err = %v, want ErrValidation", name, err)
		}
	}
}

func repeat[T any](v T, n int) []T {
	out := make([]T, n)
	for i := range out {
		out[i] = v
	}
	return out
}

func TestImageFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.png")
	if err := os.WriteFile(path, []byte("\x89PNG"), 0o600); err != nil {
		t.Fatal(err)
	}
	img, err := ImageFromFile(path, "")
	if err != nil || img.Body != "iVBORw==" {
		t.Fatalf("ImageFromFile = %+v, %v", img, err)
	}
	if _, err := ImageFromFile(filepath.Join(t.TempDir(), "missing.png"), ""); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}

func TestTopicPayloads(t *testing.T) {
	check := func(in TopicInput, create bool, want string) {
		t.Helper()
		body, err := in.payload(create)
		if err != nil {
			t.Fatalf("payload(%+v): %v", in, err)
		}
		data, _ := json.Marshal(body)
		var got map[string]any
		_ = json.Unmarshal(data, &got)
		assertJSON(t, got, want)
	}
	check(TopicInput{Title: "Home"}, true, `{"title": "Home"}`)
	check(TopicInput{Title: "Home", AvatarURL: "https://x/a.png"}, true,
		`{"title": "Home", "avatar": {"external_url": "https://x/a.png"}}`)
	check(TopicInput{AvatarImage: []byte("\x89PNG")}, false, `{"avatar": {"body": "iVBORw=="}}`)

	long := string(repeat('x', 101))
	for _, in := range []TopicInput{{}, {Title: long}, {Title: "a", AvatarURL: "u", AvatarImage: []byte("i")}} {
		if _, err := in.payload(true); !errors.Is(err, ErrValidation) {
			t.Errorf("payload(%+v) err = %v, want ErrValidation", in, err)
		}
	}
}

func TestTokenRequired(t *testing.T) {
	for _, token := range []string{"", "   "} {
		if _, err := New(token); !errors.Is(err, ErrValidation) {
			t.Errorf("New(%q) err = %v, want ErrValidation", token, err)
		}
	}
}
