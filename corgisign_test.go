package corgisign

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestClient(h http.Handler) (*Client, *httptest.Server) {
	srv := httptest.NewServer(h)
	c := New(Options{APIKey: "cs_live_test", BaseURL: srv.URL})
	return c, srv
}

func TestTemplatesList(t *testing.T) {
	c, srv := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/templates" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("X-API-Key"); got != "cs_live_test" {
			t.Errorf("X-API-Key = %q", got)
		}
		_, _ = io.WriteString(w, `{"templates":[{"id":"t1","title":"NDA","recipients":[{"id":"p1","role":"signer"}]}]}`)
	}))
	defer srv.Close()

	tmpls, err := c.Templates.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tmpls) != 1 || tmpls[0].ID != "t1" || tmpls[0].Recipients[0].Role != "signer" {
		t.Fatalf("unexpected templates: %+v", tmpls)
	}
}

func TestEnvelopesCreateSendsBodyAndIdempotency(t *testing.T) {
	c, srv := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q", r.Method)
		}
		if got := r.Header.Get("Idempotency-Key"); got != "abc-123" {
			t.Errorf("Idempotency-Key = %q", got)
		}
		var body CreateEnvelope
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.TemplateID != "tmpl" || !body.Send || body.Recipients[0].Email != "jane@example.com" {
			t.Errorf("unexpected body: %+v", body)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":"env1","status":"sent","recipients":[{"id":"r1","signingToken":"tok"}]}`)
	}))
	defer srv.Close()

	env, err := c.Envelopes.Create(context.Background(), CreateEnvelope{
		TemplateID: "tmpl",
		Recipients: []Recipient{{Role: "signer", Name: "Jane", Email: "jane@example.com"}},
		Send:       true,
	}, WithIdempotencyKey("abc-123"))
	if err != nil {
		t.Fatal(err)
	}
	if env.Status != StatusSent || env.Recipients[0].SigningToken != "tok" {
		t.Fatalf("unexpected envelope: %+v", env)
	}
}

func TestEnvelopesListQuery(t *testing.T) {
	c, srv := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("status") != "completed" || q.Get("limit") != "10" {
			t.Errorf("query = %v", q)
		}
		_, _ = io.WriteString(w, `{"envelopes":[{"id":"e1","status":"completed"}]}`)
	}))
	defer srv.Close()

	envs, err := c.Envelopes.List(context.Background(), ListEnvelopesParams{Status: StatusCompleted, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 1 || envs[0].ID != "e1" {
		t.Fatalf("unexpected: %+v", envs)
	}
}

func TestErrorMapping(t *testing.T) {
	c, srv := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":"slow down"}`)
	}))
	defer srv.Close()

	_, err := c.Templates.List(context.Background())
	apiErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("want *Error, got %T", err)
	}
	if !apiErr.IsRateLimited() || apiErr.Message != "slow down" || apiErr.RetryAfter.Seconds() != 7 {
		t.Fatalf("unexpected error: %+v", apiErr)
	}
}

func TestEnvelopesDownloadSigned(t *testing.T) {
	c, srv := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/envelopes/env1/signed":
			if got := r.Header.Get("X-API-Key"); got != "cs_live_test" {
				t.Errorf("X-API-Key = %q", got)
			}
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = io.WriteString(w, "%PDF-1.7 sealed")
		case "/api/v1/envelopes/env1/documents/doc2/download":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = io.WriteString(w, "%PDF-1.7 doc2")
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	pdf, err := c.Envelopes.DownloadSigned(context.Background(), "env1")
	if err != nil {
		t.Fatal(err)
	}
	if string(pdf) != "%PDF-1.7 sealed" {
		t.Fatalf("unexpected signed bytes: %q", pdf)
	}

	doc, err := c.Envelopes.DownloadDocument(context.Background(), "env1", "doc2")
	if err != nil {
		t.Fatal(err)
	}
	if string(doc) != "%PDF-1.7 doc2" {
		t.Fatalf("unexpected document bytes: %q", doc)
	}
}

func TestEnvelopesDownloadSignedConflict(t *testing.T) {
	c, srv := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"error":"envelope is not completed yet"}`)
	}))
	defer srv.Close()

	_, err := c.Envelopes.DownloadSigned(context.Background(), "env1")
	apiErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("want *Error, got %T", err)
	}
	if !apiErr.IsConflict() || apiErr.Message != "envelope is not completed yet" {
		t.Fatalf("unexpected error: %+v", apiErr)
	}
}

func TestMissingConfig(t *testing.T) {
	c := New(Options{BaseURL: "http://x"})
	if _, err := c.Templates.List(context.Background()); err == nil {
		t.Fatal("expected error for missing APIKey")
	}
}
