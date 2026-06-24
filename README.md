# CorgiSign Go SDK

The official Go client for the [CorgiSign](https://corgisign.example) public API
(the `/api/v1` surface). It is built for server-to-server integrations that
authenticate with an **organisation API key** (a `cs_live_...` secret), never a
user session.

```
go get github.com/Corgi-Star/corgisign-go
```

Requires Go 1.23+. The SDK has zero third-party dependencies (standard library
only).

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	corgisign "github.com/Corgi-Star/corgisign-go"
)

func main() {
	c := corgisign.New(corgisign.Options{
		APIKey:  os.Getenv("CORGISIGN_API_KEY"), // cs_live_...
		BaseURL: "https://api.corgisign.example",
	})
	ctx := context.Background()

	templates, err := c.Templates.List(ctx)
	if err != nil {
		log.Fatal(err)
	}

	env, err := c.Envelopes.Create(ctx, corgisign.CreateEnvelope{
		TemplateID: templates[0].ID,
		Title:      "Policy acknowledgement",
		Recipients: []corgisign.Recipient{
			{Role: "signer", Name: "Jane Q. Public", Email: "jane@example.com"},
		},
		Send: true,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(env.ID, env.Status)
}
```

## Authentication

Mint an organisation API key in the CorgiSign app (Organisation -> Settings ->
API keys) or via `POST /api/orgs/{orgId}/api-keys`. The raw secret is shown
exactly once. Pass it as `Options.APIKey`; the SDK sends it on every request as
the `X-API-Key` header.

A key may be pinned to a single team and restricted to a subset of scopes
(`templates:read`, `envelopes:read`, `envelopes:write`, `webhooks:write`). The
server enforces the key's organisation, team and scopes on every call.

## Client surface

Construct a client with `corgisign.New(corgisign.Options{...})`:

| Option        | Purpose                                                    |
|---------------|------------------------------------------------------------|
| `APIKey`      | Organisation API key (required).                           |
| `BaseURL`     | API root, e.g. `https://api.corgisign.example` (required). |
| `HTTPClient`  | Custom `*http.Client` (optional; default 30s timeout).     |
| `UserAgent`   | Override the default `corgisign-go/<version>` UA.          |

### Templates

```go
templates, err := c.Templates.List(ctx)                 // []corgisign.Template
templates, err := c.Templates.List(ctx, corgisign.WithTeamID(teamID))
```

Each `Template` includes its recipient placeholders, so you can build the role
mapping for envelope creation.

### Envelopes

```go
env, err  := c.Envelopes.Create(ctx, corgisign.CreateEnvelope{...})  // *Envelope
env, err  := c.Envelopes.Send(ctx, id)                               // *Envelope
env, err  := c.Envelopes.Get(ctx, id)                                // *Envelope
envs, err := c.Envelopes.List(ctx, corgisign.ListEnvelopesParams{    // []Envelope
	Status: corgisign.StatusSent,
	Limit:  50,
})
pdf, err  := c.Envelopes.DownloadSigned(ctx, id)                     // []byte (sealed PDF)
```

Create an envelope from a **template** (with a recipient role mapping and
optional field prefills) or from an **uploaded document**. Provide exactly one
of `TemplateID` or `Document`.

Template mode:

```go
env, err := c.Envelopes.Create(ctx, corgisign.CreateEnvelope{
	TemplateID: tmpl.ID,
	Title:      "Policy ACK - Jane",
	Recipients: []corgisign.Recipient{
		{Role: "signer", Name: "Jane Q. Public", Email: "jane@example.com"},
	},
	Fields: []corgisign.FieldPrefill{
		{Role: "signer", Type: corgisign.FieldText, Value: "POLICY-12345"},
	},
	Send: true,
})
```

Document mode (base64 PDF; `DocumentFromBytes` does the encoding):

```go
pdf, _ := os.ReadFile("contract.pdf")
env, err := c.Envelopes.Create(ctx, corgisign.CreateEnvelope{
	TeamID:   teamID,
	Title:    "Contract",
	Document: corgisign.DocumentFromBytes("contract.pdf", pdf),
	Recipients: []corgisign.Recipient{
		{Role: "signer", Name: "Bob", Email: "bob@example.com"},
	},
	Send: true,
})
```

The create response carries each recipient's one-time `SigningToken`.

Once the envelope completes, pull back the executed, PAdES-sealed PDF. The seal
is produced at completion, so poll `Get` (or subscribe to the
`envelope.completed` webhook) first; `DownloadSigned` returns an `*Error` with
`IsConflict()` true while the envelope has not completed. Pass an envelope +
document id to `DownloadDocument` to target a specific document.

```go
env, _ := c.Envelopes.Get(ctx, id)
if env.Status == corgisign.StatusCompleted {
	pdf, err := c.Envelopes.DownloadSigned(ctx, id)
	if err != nil {
		log.Fatal(err)
	}
	os.WriteFile("signed.pdf", pdf, 0o644)
}
```

### Webhooks (registration)

```go
wh, err := c.Webhooks.Register(ctx, corgisign.RegisterWebhook{
	URL:    "https://rpmic.example/hooks/corgi",
	Events: []string{"envelope.completed"},
})
// wh.Secret is the HMAC signing secret, returned exactly once. Persist it.
```

## Verifying inbound webhooks

CorgiSign signs every delivery with HMAC-SHA256 over the exact request body and
sends `X-CorgiSign-Signature: sha256=<hex>`. Verify it with the sibling
`webhooks` package:

```go
import "github.com/Corgi-Star/corgisign-go/webhooks"

func handler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	sig := r.Header.Get(webhooks.SignatureHeader)
	if !webhooks.Verify(body, sig, secret) {
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}
	evt, _ := webhooks.Parse(body) // evt.Event, evt.Timestamp, evt.Data
	// ...
}
```

Or in one step: `evt, err := webhooks.ParseRequest(r, secret)` verifies and
decodes, returning `webhooks.ErrInvalidSignature` on a bad signature.

## Idempotency

Any create/send may carry an idempotency key. A retry with the same key replays
the original response instead of acting twice:

```go
env, err := c.Envelopes.Create(ctx, req,
	corgisign.WithIdempotencyKey("policy-ack-12345"))
```

Reusing a key with a different body returns a `422`; replaying while the first
request is still in flight returns a `409`.

## Errors

Every non-2xx response is returned as a `*corgisign.Error`:

```go
env, err := c.Envelopes.Send(ctx, id)
var apiErr *corgisign.Error
if errors.As(err, &apiErr) {
	switch {
	case apiErr.IsRateLimited():   // 429; back off for apiErr.RetryAfter
	case apiErr.IsNotFound():      // 404
	case apiErr.IsConflict():      // 409 (e.g. not a draft)
	case apiErr.IsUnprocessable(): // 422 (e.g. invalid mapping)
	}
	log.Printf("status=%d msg=%s", apiErr.StatusCode, apiErr.Message)
}
```

| Status | Meaning                                              |
|--------|-----------------------------------------------------|
| `400`  | Malformed request (bad id / query param)            |
| `401`  | Missing / invalid / revoked API key                 |
| `403`  | Wrong organisation or team, or missing scope        |
| `404`  | Unknown envelope or template                        |
| `409`  | Invalid state, or idempotency request in flight     |
| `422`  | Invalid recipient mapping / payload                 |
| `429`  | Rate limit exceeded (see `Error.RetryAfter`)        |

## Context and cancellation

Every method takes a `context.Context` as its first argument and honours
deadlines and cancellation.

## Examples

A complete, runnable rpmic-style integration lives in
[`examples/rpmic`](examples/rpmic/main.go):

```
export CORGISIGN_API_KEY=cs_live_...
export CORGISIGN_BASE_URL=http://localhost:8080
go run ./examples/rpmic
```

## License

See the repository root.
