// Send a push notification. Run: JUSTPUSH_TOKEN=... go run ./examples/send
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/JustPush-io/justpush-go-sdk"
)

func main() {
	client, err := justpush.New(os.Getenv("JUSTPUSH_TOKEN"))
	if err != nil {
		log.Fatal(err)
	}

	result, err := client.Send(context.Background(), &justpush.Message{
		Title:   "Hello 👋",
		Message: "Sent from the JustPush Go SDK",
		Buttons: []justpush.Button{{CTA: "Open JustPush", URL: "https://justpush.io"}},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Queued %s, %d messages left this month\n", result.Key, result.RateLimit.Remaining)
}
