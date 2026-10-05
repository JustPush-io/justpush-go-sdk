package justpush

import (
	"encoding/base64"
	"encoding/json"
	"math"
	"os"
	"time"
)

// Message is a push notification to send. Message or Title is required; everything else is optional.
type Message struct {
	Message string
	// Title is cut to 255 characters by the API.
	Title string

	// Topic is a topic name: an existing topic with that name is used, otherwise a new one is created.
	Topic string
	// TopicToken targets a topic by its API token (Topic.APIToken) instead. Without Topic or
	// TopicToken, the message goes to your default topic.
	TopicToken string

	Priority Priority
	Sound    Sound

	// Buttons shown under the message, at most 10.
	Buttons []Button
	// ButtonGroups, at most 4, each opening a list of up to 10 buttons.
	ButtonGroups []ButtonGroup
	// Images, at most 10. The first one becomes the banner of the push notification.
	Images []Image

	// Expiry hides the message after this long, rounded down to whole seconds. Zero means never.
	Expiry time.Duration

	// Acknowledge asks for the message to be acknowledged on a device. A pointer to an empty
	// Acknowledgement just marks it as requiring acknowledgement.
	Acknowledge *Acknowledgement
}

// Button opens URL when tapped. With ActionRequired the message stays pending until someone
// taps it. The API cuts CTA to 25 characters.
type Button struct {
	CTA            string
	URL            string
	ActionRequired bool
}

// ButtonGroup is a single button (CTA) that opens a named list of buttons.
type ButtonGroup struct {
	Name           string
	CTA            string
	Buttons        []Button // at most 10; their ActionRequired is ignored
	ActionRequired bool
}

// Image is attached to a message, either from a public URL or as base64 image data.
// Build one with ImageFromURL, ImageFromBytes or ImageFromFile.
type Image struct {
	URL     string
	Body    string // base64-encoded image data
	Caption string
}

// ImageFromURL attaches an image that JustPush downloads from a public URL.
func ImageFromURL(url, caption string) Image {
	return Image{URL: url, Caption: caption}
}

// ImageFromBytes attaches raw image bytes (JPEG, PNG, …), e.g. a camera snapshot.
func ImageFromBytes(data []byte, caption string) Image {
	return Image{Body: base64.StdEncoding.EncodeToString(data), Caption: caption}
}

// ImageFromFile attaches an image file from disk.
func ImageFromFile(path, caption string) (Image, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Image{}, err
	}
	return ImageFromBytes(data, caption), nil
}

// Acknowledgement configures how a message is acknowledged.
type Acknowledgement struct {
	// Retry keeps re-sending the message until it is acknowledged.
	Retry bool
	// Interval between retries, 10s to 65535s, rounded down to whole seconds. Zero uses the API default.
	Interval time.Duration
	// MaxRetries is the most retries to make, 1 to 255. Zero uses the API default.
	MaxRetries int

	// CallbackURL is called by JustPush once the message is acknowledged, with CallbackParams.
	CallbackURL    string
	CallbackParams map[string]any
}

// The JSON shapes below follow the backend's MessageCreateRequest.

type messageBody struct {
	Title                   string           `json:"title,omitempty"`
	Message                 string           `json:"message,omitempty"`
	Topic                   string           `json:"topic,omitempty"`
	TopicToken              string           `json:"topic_token,omitempty"`
	Priority                *int             `json:"priority,omitempty"`
	Sound                   Sound            `json:"sound,omitempty"`
	ExpiryTTL               int64            `json:"expiry_ttl,omitempty"`
	Buttons                 []buttonBody     `json:"buttons,omitempty"`
	ButtonGroups            []groupBody      `json:"button_groups,omitempty"`
	Images                  []imageBody      `json:"images,omitempty"`
	RequiresAcknowledgement bool             `json:"requires_acknowledgement,omitempty"`
	Acknowledgement         *acknowledgeBody `json:"acknowledgement,omitempty"`
}

type buttonBody struct {
	CTA            string `json:"cta"`
	URL            string `json:"url"`
	ActionRequired *bool  `json:"action_required,omitempty"`
}

type groupBody struct {
	Name           string       `json:"name"`
	CTA            string       `json:"cta"`
	ActionRequired bool         `json:"action_required"`
	Buttons        []buttonBody `json:"buttons"`
}

type imageBody struct {
	URL     string `json:"url,omitempty"`
	Body    string `json:"body,omitempty"`
	Caption string `json:"caption,omitempty"`
}

type acknowledgeBody struct {
	RequiresRetry bool          `json:"requires_retry,omitempty"`
	Interval      int64         `json:"interval,omitempty"`
	MaxRetries    int           `json:"max_retries,omitempty"`
	Callback      *callbackBody `json:"callback,omitempty"`
}

type callbackBody struct {
	Required bool   `json:"required"`
	URL      string `json:"url"`
	Params   string `json:"params,omitempty"` // the API expects a JSON-encoded string
}

// payload builds the request body, returning a *ValidationError for anything the API would reject.
func (m *Message) payload() (*messageBody, error) {
	if m == nil || (m.Message == "" && m.Title == "") {
		return nil, invalid("a message needs a message or a title")
	}
	if m.Topic != "" && m.TopicToken != "" {
		return nil, invalid("set either Topic or TopicToken, not both")
	}

	body := &messageBody{
		Title:      m.Title,
		Message:    m.Message,
		Topic:      m.Topic,
		TopicToken: m.TopicToken,
	}

	if err := m.Priority.validate(); err != nil {
		return nil, err
	}
	if m.Priority != PriorityNormal {
		p := int(m.Priority)
		body.Priority = &p
	}

	if m.Sound != "" {
		sound, err := ParseSound(string(m.Sound))
		if err != nil {
			return nil, err
		}
		body.Sound = sound
	}

	if m.Expiry < 0 || m.Expiry/time.Second > math.MaxInt32 {
		return nil, invalid("expiry must be between 0 and %d seconds", math.MaxInt32)
	}
	body.ExpiryTTL = int64(m.Expiry / time.Second)

	if len(m.Buttons) > 10 {
		return nil, invalid("a message can have at most 10 buttons")
	}
	for _, b := range m.Buttons {
		required := b.ActionRequired
		body.Buttons = append(body.Buttons, buttonBody{CTA: b.CTA, URL: b.URL, ActionRequired: &required})
	}

	if len(m.ButtonGroups) > 4 {
		return nil, invalid("a message can have at most 4 button groups")
	}
	for _, g := range m.ButtonGroups {
		if len(g.Buttons) > 10 {
			return nil, invalid("a button group can hold at most 10 buttons")
		}
		group := groupBody{Name: g.Name, CTA: g.CTA, ActionRequired: g.ActionRequired, Buttons: []buttonBody{}}
		for _, b := range g.Buttons {
			group.Buttons = append(group.Buttons, buttonBody{CTA: b.CTA, URL: b.URL})
		}
		body.ButtonGroups = append(body.ButtonGroups, group)
	}

	if len(m.Images) > 10 {
		return nil, invalid("a message can have at most 10 images")
	}
	for _, img := range m.Images {
		if (img.URL == "") == (img.Body == "") {
			return nil, invalid("an image needs either a URL or a Body, not both")
		}
		body.Images = append(body.Images, imageBody(img))
	}

	if m.Acknowledge != nil {
		body.RequiresAcknowledgement = true
		ack, err := m.Acknowledge.payload()
		if err != nil {
			return nil, err
		}
		body.Acknowledgement = ack
	}

	return body, nil
}

func (a *Acknowledgement) payload() (*acknowledgeBody, error) {
	body := &acknowledgeBody{}
	if a.Retry {
		interval := int64(a.Interval / time.Second)
		if a.Interval != 0 && (interval < 10 || interval > 65535) {
			return nil, invalid("acknowledgement interval must be 10–65535 seconds")
		}
		if a.MaxRetries < 0 || a.MaxRetries > 255 {
			return nil, invalid("acknowledgement MaxRetries must be 0–255")
		}
		body.RequiresRetry = true
		body.Interval = interval
		body.MaxRetries = a.MaxRetries
	}
	if a.CallbackURL != "" {
		body.Callback = &callbackBody{Required: true, URL: a.CallbackURL}
		if a.CallbackParams != nil {
			params, err := json.Marshal(a.CallbackParams)
			if err != nil {
				return nil, invalid("acknowledgement CallbackParams can't be encoded as JSON: %v", err)
			}
			body.Callback.Params = string(params)
		}
	}
	if *body == (acknowledgeBody{}) {
		return nil, nil
	}
	return body, nil
}
