package justpush

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"
)

// Topic groups messages in the app. APIToken can be passed as Message.TopicToken.
type Topic struct {
	UUID            string
	Title           string
	Slug            string
	Avatar          string
	HasCustomAvatar bool
	APIToken        string
	// Raw holds the full API response.
	Raw json.RawMessage
}

// TopicInput creates or updates a topic. Give it an avatar from a public URL or from image bytes.
type TopicInput struct {
	// Title is required when creating a topic, at most 100 characters. The default topic
	// can't be renamed.
	Title       string
	AvatarURL   string
	AvatarImage []byte
}

type topicBody struct {
	Title  string      `json:"title,omitempty"`
	Avatar *avatarBody `json:"avatar,omitempty"`
}

type avatarBody struct {
	ExternalURL string `json:"external_url,omitempty"`
	Body        string `json:"body,omitempty"`
}

func (t TopicInput) payload(create bool) (*topicBody, error) {
	if create && t.Title == "" {
		return nil, invalid("a topic needs a title")
	}
	if utf8.RuneCountInString(t.Title) > 100 {
		return nil, invalid("a topic title can be at most 100 characters")
	}
	if t.AvatarURL != "" && len(t.AvatarImage) > 0 {
		return nil, invalid("set either AvatarURL or AvatarImage, not both")
	}
	body := &topicBody{Title: t.Title}
	if t.AvatarURL != "" {
		body.Avatar = &avatarBody{ExternalURL: t.AvatarURL}
	} else if len(t.AvatarImage) > 0 {
		body.Avatar = &avatarBody{Body: base64.StdEncoding.EncodeToString(t.AvatarImage)}
	}
	return body, nil
}

type topicJSON struct {
	UUID            string   `json:"uuid"`
	Title           string   `json:"title"`
	Slug            string   `json:"slug"`
	Avatar          string   `json:"avatar"`
	HasCustomAvatar flexBool `json:"has_custom_avatar"`
	APIToken        string   `json:"api_token"`
}

func (t *Topic) UnmarshalJSON(data []byte) error {
	var v topicJSON
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*t = Topic{
		UUID:            v.UUID,
		Title:           v.Title,
		Slug:            v.Slug,
		Avatar:          v.Avatar,
		HasCustomAvatar: bool(v.HasCustomAvatar),
		APIToken:        v.APIToken,
		Raw:             append(json.RawMessage(nil), data...),
	}
	return nil
}

// MessageDetails is a message as stored by JustPush.
type MessageDetails struct {
	Key                     string
	Title                   string
	Message                 string
	Priority                Priority
	Sound                   Sound
	Topic                   *Topic
	RequiresAcknowledgement bool
	IsAcknowledged          bool
	PendingActions          json.RawMessage
	// The times are zero when the API leaves them out.
	ReceivedAt  time.Time
	ProcessedAt time.Time
	ExpiresAt   time.Time
	// Raw holds the full API response.
	Raw json.RawMessage
}

type messageJSON struct {
	Key                     string          `json:"key"`
	Title                   string          `json:"title"`
	Message                 string          `json:"message"`
	Priority                flexInt         `json:"priority"`
	Sound                   Sound           `json:"sound"`
	Topic                   *Topic          `json:"topic"`
	RequiresAcknowledgement flexBool        `json:"requires_acknowledgement"`
	Acknowledgement         json.RawMessage `json:"acknowledgement"`
	PendingActions          json.RawMessage `json:"pending_actions"`
	ReceivedAt              flexTime        `json:"received_at"`
	ProcessedAt             flexTime        `json:"processed_at"`
	ExpiresAt               flexTime        `json:"expires_at"`
}

func (m *MessageDetails) UnmarshalJSON(data []byte) error {
	var v messageJSON
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	var ack struct {
		IsAcknowledged flexBool `json:"is_acknowledged"`
	}
	_ = json.Unmarshal(v.Acknowledgement, &ack) // absent or not an object: not acknowledged

	*m = MessageDetails{
		Key:                     v.Key,
		Title:                   v.Title,
		Message:                 v.Message,
		Priority:                Priority(v.Priority),
		Sound:                   v.Sound,
		Topic:                   v.Topic,
		RequiresAcknowledgement: bool(v.RequiresAcknowledgement),
		IsAcknowledged:          bool(ack.IsAcknowledged),
		PendingActions:          v.PendingActions,
		ReceivedAt:              time.Time(v.ReceivedAt),
		ProcessedAt:             time.Time(v.ProcessedAt),
		ExpiresAt:               time.Time(v.ExpiresAt),
		Raw:                     append(json.RawMessage(nil), data...),
	}
	return nil
}

// The API isn't strict about scalar types (e.g. has_custom_avatar may be "1" or true),
// so these accept the common spellings rather than failing the whole response.

type flexBool bool

func (b *flexBool) UnmarshalJSON(data []byte) error {
	switch strings.Trim(strings.ToLower(string(data)), `"`) {
	case "true", "1":
		*b = true
	default:
		*b = false
	}
	return nil
}

type flexInt int

func (n *flexInt) UnmarshalJSON(data []byte) error {
	var f float64
	if json.Unmarshal([]byte(strings.Trim(string(data), `"`)), &f) == nil {
		*n = flexInt(f)
	}
	return nil
}

type flexTime time.Time

var timeLayouts = []string{time.RFC3339Nano, "2006-01-02T15:04:05.000000", "2006-01-02 15:04:05"}

func (t *flexTime) UnmarshalJSON(data []byte) error {
	var s string
	if json.Unmarshal(data, &s) != nil || s == "" {
		return nil
	}
	for _, layout := range timeLayouts {
		if parsed, err := time.Parse(layout, s); err == nil {
			*t = flexTime(parsed)
			return nil
		}
	}
	return nil // unknown format: leave zero, the value is still in Raw
}
