<p align="center">
  <a href="https://justpush.io"><img src="https://cdn.justpush.io/core/app%20icon_nobackground.svg" width="150" height="auto" alt="JustPush"></a>
</p>

<p align="center">
  <a href="https://justpush.io">Website</a> ·
  <a href="https://docs.justpush.io">Docs</a> ·
  <a href="https://apps.apple.com/us/app/justpush-io/id6738397112">iOS app</a> ·
  <a href="https://play.google.com/store/apps/details?id=com.justpush">Android app</a> ·
  <a href="https://studio.justpush.io">Studio</a> ·
  <a href="https://justpush.io/recipes">Recipes</a>
</p>

<p align="center">
  <a href="https://pkg.go.dev/github.com/JustPush-io/justpush-go-sdk"><img src="https://pkg.go.dev/badge/github.com/JustPush-io/justpush-go-sdk.svg" alt="Go Reference"></a>
  <a href="https://github.com/JustPush-io/justpush-go-sdk/actions/workflows/ci.yml"><img src="https://github.com/JustPush-io/justpush-go-sdk/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
</p>

## JustPush — Go SDK

The official Go SDK for [JustPush](https://justpush.io). Send push notifications to your
iOS and Android devices from any Go program, server or CLI.

- Standard library only, no dependencies
- Typed options for buttons, button groups, images, sounds and acknowledgements
- Checks the message before sending it, so you get a clear error instead of a rejected request
- Errors that work with `errors.Is` / `errors.As`, and rate-limit information on every send

## Installation

```bash
go get github.com/JustPush-io/justpush-go-sdk
```

Go 1.21 or newer.

## Send a message

```go
import "github.com/JustPush-io/justpush-go-sdk"

client, err := justpush.New("YOUR_API_TOKEN")
if err != nil {
    log.Fatal(err)
}

result, err := client.Send(ctx, &justpush.Message{
    Title:   "Backups",
    Message: "The nightly backup finished",
})
if err != nil {
    log.Fatal(err)
}

fmt.Println(result.Key)                   // use it with GetMessage()
fmt.Println(result.RateLimit.Remaining)   // messages left this month
```

Get your API token from the JustPush app.

### Everything a message can do

```go
snapshot, err := justpush.ImageFromFile("snapshot.jpg", "Kitchen")
if err != nil {
    log.Fatal(err)
}

_, err = client.Send(ctx, &justpush.Message{
    Title:    "💧 Leak",
    Message:  "Water detected under the washing machine",
    Topic:    "Home",                      // topic name; created if it doesn't exist yet
    Priority: justpush.PriorityHighest,
    Sound:    justpush.SoundSiren,
    Buttons:  []justpush.Button{{CTA: "Open camera", URL: "https://example.com/cam"}},
    Images:   []justpush.Image{snapshot},
    Expiry:   time.Hour,                   // hide the message after an hour
    Acknowledge: &justpush.Acknowledgement{
        Retry: true, Interval: time.Minute, MaxRetries: 10,
        CallbackURL:    "https://example.com/acknowledged",
        CallbackParams: map[string]any{"sensor": "kitchen"},
    },
})
```

| Field | |
| --- | --- |
| `Message`, `Title` | At least one is required. Titles over 255 characters are cut short. |
| `Topic` | A topic **name**. An existing topic with that name is used, or a new one is created. |
| `TopicToken` | Target a topic by its API token instead (`Topic.APIToken`). |
| `Priority` | `PriorityLowest` (-2) … `PriorityHighest` (2). Use `ParsePriority("high")` for names. |
| `Sound` | A `Sound` constant, or its name in any case. `SoundNone` is silent. |
| `Buttons` | Up to 10 `Button{CTA, URL, ActionRequired}`. Labels over 25 characters are cut. |
| `ButtonGroups` | Up to 4 `ButtonGroup{Name, CTA, Buttons}`, each with up to 10 buttons. |
| `Images` | Up to 10, from `ImageFromURL`, `ImageFromBytes` or `ImageFromFile`. The first one becomes the notification banner. |
| `Expiry` | How long until the message is hidden, in whole seconds. |
| `Acknowledge` | `&Acknowledgement{}` to require acknowledgement; set `Retry` for retries (`Interval` 10–65535 s, `MaxRetries` up to 255) and `CallbackURL` for a callback. |

Zero values aren't sent, so the API's defaults apply (normal priority, default sound, no expiry).

### Check a message

```go
details, err := client.GetMessage(ctx, result.Key)
fmt.Println(details.IsAcknowledged, details.ProcessedAt)
```

### Check a token

```go
err := client.VerifyToken(ctx)   // errors.Is(err, justpush.ErrUnauthorized) if the token is invalid
```

This sends nothing and uses no quota, so it's safe for setup screens.

## Topics

```go
topic, err := client.CreateTopic(ctx, justpush.TopicInput{Title: "Servers", AvatarURL: "https://example.com/server.png"})
client.UpdateTopic(ctx, topic.UUID, justpush.TopicInput{Title: "Production servers"})
client.GetTopic(ctx, topic.UUID)
client.Send(ctx, &justpush.Message{Message: "Disk almost full", TopicToken: topic.APIToken})
```

## Configuration

```go
client, err := justpush.New(token,
    justpush.WithTimeout(5*time.Second),        // default 10 s
    justpush.WithHTTPClient(myHTTPClient),      // proxies, tracing, …
    justpush.WithBaseURL("http://localhost:8080"),
)
```

Every method takes a `context.Context`, so you can also cancel a call or give it a deadline.
A `Client` is safe to share between goroutines.

## Errors

Check errors with `errors.Is`:

| Error | When |
| --- | --- |
| `ErrValidation` | The message is invalid, either caught before sending (`*ValidationError`) or a 422 from the API (`*APIError` with per-field `Errors`). |
| `ErrUnauthorized` | 401: the token is missing or wrong. |
| `ErrForbidden` | 403: not allowed, e.g. a topic you don't own, or a plan limit. |
| `ErrNotFound` | 404: unknown message key or topic. |
| `ErrSubscriptionExpired` | 410: the subscription expired. |
| `ErrRateLimited` | 429: too many requests; see `APIError.RetryAfter`. |
| `ErrConnection` | Network error, timeout or cancelled context (`*ConnectionError`). |

Any error status from the API is an `*APIError` with `StatusCode`, `Message` and the raw `Body`:

```go
var apiErr *justpush.APIError
if errors.As(err, &apiErr) && errors.Is(err, justpush.ErrRateLimited) {
    time.Sleep(apiErr.RetryAfter)
}
```

<!-- justpush-packages:start -->
## JustPush products

**Apps**
- [JustPush for iOS](https://apps.apple.com/us/app/justpush-io/id6738397112)
- [JustPush for Android](https://play.google.com/store/apps/details?id=com.justpush)
- [JustPush Studio](https://studio.justpush.io): build integrations that turn webhooks into notifications
- [Recipes](https://justpush.io/recipes): ready-made integrations for Shopify, Magento and more
- [Playground](https://playground.justpush.io): try the API in your browser

**SDKs**
- [PHP](https://github.com/JustPush-io/justpush-sdk-php) · [Packagist](https://packagist.org/packages/justpush/justpush-php-sdk)
- [JavaScript](https://github.com/JustPush-io/justpush-sdk-js) · [npm](https://www.npmjs.com/package/@justpush.io/justpush-js-sdk)
- [TypeScript](https://github.com/JustPush-io/justpush-sdk-ts) · [npm](https://www.npmjs.com/package/@justpush.io/justpush-ts-sdk)
- [Python](https://github.com/JustPush-io/justpush-sdk-python) · [PyPI](https://pypi.org/project/justpush/)
- [Go](https://github.com/JustPush-io/justpush-go-sdk) · [pkg.go.dev](https://pkg.go.dev/github.com/JustPush-io/justpush-go-sdk) (this one)

**Packages**
- [Laravel notification channel](https://github.com/JustPush-io/justpush-laravel-notifications) · [Packagist](https://packagist.org/packages/justpush/laravel-notification-channel)
- [Magento 2](https://github.com/JustPush-io/justpush-magento) · [Packagist](https://packagist.org/packages/justpush/magento-module-notify)
- [PrestaShop](https://github.com/JustPush-io/justpush-prestashop) · [Download](https://github.com/JustPush-io/justpush-prestashop/releases/latest)

**Developer tools**
- [API reference](https://docs.justpush.io/introduction/dev-tools/api-reference) · [OpenAPI](https://docs.justpush.io/introduction/dev-tools/openapi)
- [Postman collection](https://github.com/JustPush-io/postman) · [Paw collection](https://github.com/JustPush-io/paw)
- [MCP server for Claude](https://docs.justpush.io/introduction/dev-tools/mcp-server)
<!-- justpush-packages:end -->

## Development

```bash
go vet ./... && go test -race ./...
```

Releases are git tags (`v0.1.0`, …); the Go module proxy picks them up automatically.

## Changelog

See [CHANGELOG.md](./CHANGELOG.md).
