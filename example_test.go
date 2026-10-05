package justpush_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/JustPush-io/justpush-go-sdk"
)

func Example() {
	client, err := justpush.New("YOUR_API_TOKEN")
	if err != nil {
		log.Fatal(err)
	}
	result, err := client.Send(context.Background(), &justpush.Message{
		Title:   "Backups",
		Message: "The nightly backup finished",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result.Key, result.RateLimit.Remaining)
}

func ExampleClient_Send() {
	client, _ := justpush.New("YOUR_API_TOKEN")
	snapshot, err := justpush.ImageFromFile("snapshot.jpg", "Kitchen")
	if err != nil {
		log.Fatal(err)
	}

	_, err = client.Send(context.Background(), &justpush.Message{
		Title:    "💧 Leak",
		Message:  "Water detected under the washing machine",
		Topic:    "Home",
		Priority: justpush.PriorityHighest,
		Sound:    justpush.SoundSiren,
		Buttons:  []justpush.Button{{CTA: "Open camera", URL: "https://example.com/cam"}},
		Images:   []justpush.Image{snapshot},
		Expiry:   time.Hour,
		Acknowledge: &justpush.Acknowledgement{
			Retry: true, Interval: time.Minute, MaxRetries: 10,
			CallbackURL:    "https://example.com/acknowledged",
			CallbackParams: map[string]any{"sensor": "kitchen"},
		},
	})

	var apiErr *justpush.APIError
	switch {
	case errors.Is(err, justpush.ErrRateLimited) && errors.As(err, &apiErr):
		log.Printf("rate limited, retry in %s", apiErr.RetryAfter)
	case err != nil:
		log.Fatal(err)
	}
}
