// Command smoke is a live end-to-end check of the CorgiSign Go SDK. It logs in
// as an admin with a session cookie, mints a scoped cs_live_ API key, then
// drives the SDK through the full flow: list templates, create + send an
// envelope from a template, read it back, list envelopes, register a webhook,
// and round-trip a webhook signature.
//
// It talks to a running CorgiSign instance. Defaults match local dev:
//
//	CORGISIGN_BASE_URL   (default http://localhost:8080)
//	CORGISIGN_ADMIN_EMAIL (default admin@corgi.test)
//	CORGISIGN_ADMIN_PASS  (default corgidemo123)
//
//	go run ./cmd/smoke
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"os"
	"time"

	corgisign "github.com/Corgi-Star/corgisign-go-sdk"
	"github.com/Corgi-Star/corgisign-go-sdk/webhooks"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	base := env("CORGISIGN_BASE_URL", "http://localhost:8080")
	email := env("CORGISIGN_ADMIN_EMAIL", "admin@corgi.test")
	pass := env("CORGISIGN_ADMIN_PASS", "corgidemo123")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	jar, _ := cookiejar.New(nil)
	admin := &http.Client{Jar: jar, Timeout: 15 * time.Second}

	step("login as %s", email)
	if err := postJSON(ctx, admin, base+"/api/auth/login", map[string]string{"email": email, "password": pass}, nil); err != nil {
		log.Fatalf("login failed: %v", err)
	}

	step("resolve organisation via /api/me")
	var me struct {
		Organisations []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"organisations"`
	}
	if err := getJSON(ctx, admin, base+"/api/me", &me); err != nil {
		log.Fatalf("/api/me failed: %v", err)
	}
	if len(me.Organisations) == 0 {
		log.Fatal("admin belongs to no organisations")
	}
	orgID := me.Organisations[0].ID
	fmt.Printf("    org=%s (%s)\n", me.Organisations[0].Name, orgID)

	step("mint a scoped cs_live_ API key")
	var key struct {
		Token  string `json:"token"`
		Prefix string `json:"prefix"`
	}
	mint := map[string]any{
		"name":   "go-sdk-smoke",
		"scopes": []string{"templates:read", "envelopes:read", "envelopes:write", "webhooks:write"},
	}
	if err := postJSON(ctx, admin, base+"/api/orgs/"+orgID+"/api-keys", mint, &key); err != nil {
		log.Fatalf("mint key failed: %v", err)
	}
	if key.Token == "" {
		log.Fatal("no key token returned")
	}
	fmt.Printf("    minted key prefix=%s\n", key.Prefix)

	// --- everything below uses ONLY the SDK + the minted key ---
	c := corgisign.New(corgisign.Options{APIKey: key.Token, BaseURL: base})

	step("SDK: Templates.List")
	templates, err := c.Templates.List(ctx)
	if err != nil {
		log.Fatalf("Templates.List: %v", err)
	}
	fmt.Printf("    %d template(s)\n", len(templates))
	if len(templates) == 0 {
		log.Fatal("no templates available; seed the instance first")
	}
	// Pick the first template that has a recipient placeholder to map onto.
	var tmpl *corgisign.Template
	for i := range templates {
		if len(templates[i].Recipients) > 0 {
			tmpl = &templates[i]
			break
		}
	}
	if tmpl == nil {
		log.Fatal("no template with a recipient placeholder; seed the instance first")
	}
	role := tmpl.Recipients[0].Role
	if role == "" {
		role = corgisign.RoleSigner
	}
	fmt.Printf("    using %q (%s), role=%s\n", tmpl.Title, tmpl.ID, role)

	step("SDK: Envelopes.Create (template mode, send=true, idempotent)")
	idemKey := fmt.Sprintf("smoke-%d", time.Now().UnixNano())
	created, err := c.Envelopes.Create(ctx, corgisign.CreateEnvelope{
		TemplateID: tmpl.ID,
		Title:      "SDK smoke - " + time.Now().Format(time.RFC3339),
		Recipients: []corgisign.Recipient{
			{Role: role, Name: "Jane Q. Public", Email: "jane.smoke@example.com"},
		},
		Fields: []corgisign.FieldPrefill{
			{Role: role, Type: corgisign.FieldText, Value: "POLICY-SMOKE-1"},
		},
		Send: true,
	}, corgisign.WithIdempotencyKey(idemKey))
	if err != nil {
		log.Fatalf("Envelopes.Create: %v", err)
	}
	fmt.Printf("    envelope=%s status=%s recipients=%d\n", created.ID, created.Status, len(created.Recipients))
	for _, r := range created.Recipients {
		fmt.Printf("      %s <%s> tokenPresent=%v\n", r.Name, r.Email, r.SigningToken != "")
	}

	step("SDK: idempotent replay returns the same envelope")
	replay, err := c.Envelopes.Create(ctx, corgisign.CreateEnvelope{
		TemplateID: tmpl.ID,
		Title:      "SDK smoke - " + time.Now().Format(time.RFC3339),
		Recipients: []corgisign.Recipient{
			{Role: role, Name: "Jane Q. Public", Email: "jane.smoke@example.com"},
		},
		Fields: []corgisign.FieldPrefill{
			{Role: role, Type: corgisign.FieldText, Value: "POLICY-SMOKE-1"},
		},
		Send: true,
	}, corgisign.WithIdempotencyKey(idemKey))
	if err != nil {
		log.Fatalf("idempotent replay: %v", err)
	}
	fmt.Printf("    replay envelope=%s (same=%v)\n", replay.ID, replay.ID == created.ID)

	step("SDK: Envelopes.Get")
	got, err := c.Envelopes.Get(ctx, created.ID)
	if err != nil {
		log.Fatalf("Envelopes.Get: %v", err)
	}
	fmt.Printf("    status=%s sentAt=%v\n", got.Status, got.SentAt != nil)

	step("SDK: Envelopes.List (limit=5)")
	list, err := c.Envelopes.List(ctx, corgisign.ListEnvelopesParams{Limit: 5})
	if err != nil {
		log.Fatalf("Envelopes.List: %v", err)
	}
	fmt.Printf("    %d envelope(s) returned\n", len(list))

	step("SDK: Webhooks.Register")
	wh, err := c.Webhooks.Register(ctx, corgisign.RegisterWebhook{
		URL:    "https://rpmic.example/hooks/corgi-smoke",
		Events: []string{"envelope.completed"},
	})
	if err != nil {
		log.Fatalf("Webhooks.Register: %v", err)
	}
	fmt.Printf("    webhook=%s secretPresent=%v\n", wh.ID, wh.Secret != "")

	step("webhooks.Verify round-trip against the returned secret")
	payload := []byte(`{"event":"envelope.completed","data":{"envelopeId":"` + created.ID + `"}}`)
	sig := webhooks.Sign(payload, wh.Secret)
	if !webhooks.Verify(payload, sig, wh.Secret) {
		log.Fatal("webhooks.Verify rejected a valid signature")
	}
	if webhooks.Verify(payload, sig, "wrong-secret") {
		log.Fatal("webhooks.Verify accepted a wrong secret")
	}
	fmt.Println("    verify ok (valid accepted, wrong rejected)")

	step("typed error: Envelopes.Get on a bogus id -> *corgisign.Error")
	_, err = c.Envelopes.Get(ctx, "00000000-0000-0000-0000-000000000000")
	if apiErr, ok := err.(*corgisign.Error); ok {
		fmt.Printf("    got *corgisign.Error status=%d notFound=%v msg=%q\n", apiErr.StatusCode, apiErr.IsNotFound(), apiErr.Message)
	} else {
		fmt.Printf("    WARN: expected *corgisign.Error, got %v\n", err)
	}

	step("SDK: DownloadSigned on the incomplete envelope -> 409 conflict")
	_, err = c.Envelopes.DownloadSigned(ctx, created.ID)
	if apiErr, ok := err.(*corgisign.Error); ok && apiErr.IsConflict() {
		fmt.Printf("    got expected 409 (not completed): %q\n", apiErr.Message)
	} else {
		fmt.Printf("    WARN: expected *corgisign.Error 409, got %v\n", err)
	}

	fmt.Println("\nSMOKE PASS")
}

func step(format string, args ...any) {
	fmt.Printf("==> "+format+"\n", args...)
}

func postJSON(ctx context.Context, c *http.Client, url string, body, out any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return doJSON(c, req, out)
}

func getJSON(ctx context.Context, c *http.Client, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	return doJSON(c, req, out)
}

func doJSON(c *http.Client, req *http.Request, out any) error {
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}
	if out != nil && len(b) > 0 {
		return json.Unmarshal(b, out)
	}
	return nil
}
