package justpush

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Priority is a message's priority. Higher priorities break through quiet hours on the device.
type Priority int

const (
	PriorityLowest  Priority = -2
	PriorityLow     Priority = -1
	PriorityNormal  Priority = 0 // the default; a zero Priority is not sent at all
	PriorityHigh    Priority = 1
	PriorityHighest Priority = 2
)

var priorityNames = map[string]Priority{
	"lowest":  PriorityLowest,
	"low":     PriorityLow,
	"normal":  PriorityNormal,
	"high":    PriorityHigh,
	"highest": PriorityHighest,
}

// ParsePriority accepts a name ("high", "LOWEST") or a number from -2 to 2 ("-1").
func ParsePriority(s string) (Priority, error) {
	text := strings.ToLower(strings.TrimSpace(s))
	if p, ok := priorityNames[text]; ok {
		return p, nil
	}
	if n, err := strconv.Atoi(text); err == nil {
		p := Priority(n)
		return p, p.validate()
	}
	return 0, invalid("invalid priority %q", s)
}

func (p Priority) validate() error {
	if p < PriorityLowest || p > PriorityHighest {
		return invalid("priority must be between -2 and 2, got %d", int(p))
	}
	return nil
}

func (p Priority) String() string {
	for name, v := range priorityNames {
		if v == p {
			return name
		}
	}
	return strconv.Itoa(int(p))
}

// Sound is a notification sound the JustPush apps can play.
// An empty Sound is not sent, so the app plays its default.
type Sound string

const (
	SoundNone         Sound = "none" // silent
	SoundDefault      Sound = "default"
	SoundBike         Sound = "bike"
	SoundBugle        Sound = "bugle"
	SoundCashRegister Sound = "cashregister"
	SoundClassical    Sound = "classical"
	SoundCosmic       Sound = "cosmic"
	SoundFalling      Sound = "falling"
	SoundGamelan      Sound = "gamelan"
	SoundIncoming     Sound = "incoming"
	SoundIntermission Sound = "intermission"
	SoundMagic        Sound = "magic"
	SoundMechanical   Sound = "mechanical"
	SoundPianoBar     Sound = "pianobar"
	SoundSiren        Sound = "siren"
	SoundSpaceAlarm   Sound = "spacealarm"
	SoundTugboat      Sound = "tugboat"
	SoundAlien        Sound = "alien"
	SoundClimb        Sound = "climb"
	SoundPersistent   Sound = "persistent"
	SoundEcho         Sound = "echo"
	SoundUpDown       Sound = "updown"
	SoundVibrate      Sound = "vibrate"
)

// Sounds lists every sound the API accepts.
var Sounds = []Sound{
	SoundNone, SoundDefault, SoundBike, SoundBugle, SoundCashRegister, SoundClassical,
	SoundCosmic, SoundFalling, SoundGamelan, SoundIncoming, SoundIntermission, SoundMagic,
	SoundMechanical, SoundPianoBar, SoundSiren, SoundSpaceAlarm, SoundTugboat, SoundAlien,
	SoundClimb, SoundPersistent, SoundEcho, SoundUpDown, SoundVibrate,
}

// ParseSound accepts a sound name in any case ("COSMIC", "cosmic").
func ParseSound(s string) (Sound, error) {
	sound := Sound(strings.ToLower(strings.TrimSpace(s)))
	for _, known := range Sounds {
		if sound == known {
			return sound, nil
		}
	}
	names := make([]string, len(Sounds))
	for i, known := range Sounds {
		names[i] = string(known)
	}
	return "", invalid("unknown sound %q; use one of: %s", s, strings.Join(names, ", "))
}

// RateLimit is your plan's monthly message allowance, read from the response headers.
type RateLimit struct {
	// Known reports whether the API sent rate-limit headers; the other fields are zero otherwise.
	Known     bool
	Limit     int
	Remaining int
	// Reset is the time until the allowance resets.
	Reset time.Duration
}

func rateLimitFromHeaders(h http.Header) RateLimit {
	limit, okLimit := headerInt(h, "X-Limit-App-Limit")
	remaining, okRemaining := headerInt(h, "X-Limit-App-Remaining")
	reset, _ := headerInt(h, "X-Limit-App-Reset")
	return RateLimit{
		Known:     okLimit || okRemaining,
		Limit:     limit,
		Remaining: remaining,
		Reset:     time.Duration(reset) * time.Second,
	}
}

func headerInt(h http.Header, name string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(h.Get(name)))
	return n, err == nil
}

// SendResult is what the API returns for a queued message.
type SendResult struct {
	// Key identifies the message; pass it to Client.GetMessage.
	Key       string
	RateLimit RateLimit
}
