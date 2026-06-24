// Command rpmic is a runnable example of the CorgiSign Go SDK modelled on the
// rpmic insurance platform's use case: a policy-acknowledgement document sent to
// a consumer for signature, driven entirely server-to-server by an organisation
// API key.
//
// It (1) lists templates, (2) creates + sends an envelope from a template with a
// mapped signer and a prefilled policy number, (3) polls the envelope status,
// (4) registers a webhook so rpmic is notified on completion, and (5) shows how
// rpmic's HTTP handler verifies an inbound webhook signature.
//
// Run it against a CorgiSign instance:
//
//	export CORGISIGN_API_KEY=cs_live_…
//	export CORGISIGN_BASE_URL=http://localhost:8080
//	go run ./examples/rpmic
package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	corgisign "github.com/Corgi-Star/corgisign-go"
	"github.com/Corgi-Star/corgisign-go/webhooks"
)

func main() {
	apiKey := os.Getenv("CORGISIGN_API_KEY")
	baseURL := os.Getenv("CORGISIGN_BASE_URL")
	if apiKey == "" || baseURL == "" {
		log.Fatal("set CORGISIGN_API_KEY and CORGISIGN_BASE_URL")
	}

	c := corgisign.New(corgisign.Options{APIKey: apiKey, BaseURL: baseURL})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1) Discover a template and the role to map our signer onto.
	templates, err := c.Templates.List(ctx)
	if err != nil {
		log.Fatalf("list templates: %v", err)
	}
	if len(templates) == 0 {
		log.Fatal("no templates available to this API key")
	}
	tmpl := templates[0]
	role := corgisign.RoleSigner
	if len(tmpl.Recipients) > 0 {
		role = tmpl.Recipients[0].Role
	}
	fmt.Printf("using template %q (%s), mapping role %q\n", tmpl.Title, tmpl.ID, role)

	// 2) Create + send the envelope. The Idempotency-Key makes a retry safe:
	//    rpmic keys it by the policy id so a redelivered job never double-sends.
	policyID := "POLICY-12345"
	env, err := c.Envelopes.Create(ctx, corgisign.CreateEnvelope{
		TemplateID: tmpl.ID,
		Title:      "Policy acknowledgement - Jane Q. Public",
		Recipients: []corgisign.Recipient{
			{Role: role, Name: "Jane Q. Public", Email: "jane@example.com"},
		},
		Fields: []corgisign.FieldPrefill{
			{Role: role, Type: corgisign.FieldText, Value: policyID},
		},
		Send: true,
	}, corgisign.WithIdempotencyKey("policy-ack-"+policyID))
	if err != nil {
		log.Fatalf("create envelope: %v", err)
	}
	fmt.Printf("envelope %s status=%s\n", env.ID, env.Status)
	for _, r := range env.Recipients {
		// The signing token is returned once; rpmic can build a direct signing URL.
		fmt.Printf("  recipient %s <%s> token=%s\n", r.Name, r.Email, r.SigningToken)
	}

	// 3) Read status back.
	got, err := c.Envelopes.Get(ctx, env.ID)
	if err != nil {
		log.Fatalf("get envelope: %v", err)
	}
	fmt.Printf("re-fetched %s: status=%s, %d recipient(s)\n", got.ID, got.Status, len(got.Recipients))

	// 3b) Once the holder has signed, pull back the executed, PAdES-sealed PDF.
	if got.Status == corgisign.StatusCompleted {
		pdf, derr := c.Envelopes.DownloadSigned(ctx, got.ID)
		if derr != nil {
			log.Printf("download signed PDF: %v", derr)
		} else {
			fmt.Printf("downloaded signed PDF (%d bytes)\n", len(pdf))
			// e.g. os.WriteFile("policy-"+policyID+".pdf", pdf, 0o644)
		}
	}

	// 4) Register a webhook for completion notifications.
	wh, err := c.Webhooks.Register(ctx, corgisign.RegisterWebhook{
		URL:    "https://rpmic.example/hooks/corgi",
		Events: []string{"envelope.completed"},
	})
	if err != nil {
		// Registration needs the webhooks:write scope; treat as non-fatal here.
		log.Printf("register webhook (needs webhooks:write): %v", err)
	} else {
		fmt.Printf("webhook %s registered; store secret %q to verify deliveries\n", wh.ID, wh.Secret)
	}

	// 5) How rpmic's webhook receiver authenticates an inbound delivery.
	demoWebhookReceiver()
}

// demoWebhookReceiver shows the verification half of the integration without
// standing up a real server: it crafts a signed delivery and validates it.
func demoWebhookReceiver() {
	secret := "whsec_example"
	handler := func(w http.ResponseWriter, r *http.Request) {
		evt, err := webhooks.ParseRequest(r, secret)
		if err != nil {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
		fmt.Printf("verified inbound webhook: event=%s data=%v\n", evt.Event, evt.Data)
		w.WriteHeader(http.StatusOK)
	}

	body := []byte(`{"event":"envelope.completed","data":{"envelopeId":"env_123"}}`)
	req := httptest.NewRequest(http.MethodPost, "/hooks/corgi", bytes.NewReader(body))
	req.Header.Set(webhooks.SignatureHeader, webhooks.Sign(body, secret))
	handler(httptest.NewRecorder(), req)
}
