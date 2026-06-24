// Package webhooks verifies and parses inbound CorgiSign webhook deliveries.
//
// CorgiSign signs every delivery with HMAC-SHA256 over the exact request body,
// sending the result in the X-CorgiSign-Signature header as "sha256=<hex>". The
// shared secret is the one returned (exactly once) when the webhook was
// registered.
//
//	func handler(w http.ResponseWriter, r *http.Request) {
//		body, _ := io.ReadAll(r.Body)
//		sig := r.Header.Get(webhooks.SignatureHeader)
//		if !webhooks.Verify(body, sig, secret) {
//			http.Error(w, "bad signature", http.StatusUnauthorized)
//			return
//		}
//		evt, _ := webhooks.Parse(body)
//		// ... handle evt.Event / evt.Data ...
//	}
//
// Or, in one step: evt, err := webhooks.ParseRequest(r, secret).
package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

// Header names set on every delivery.
const (
	// SignatureHeader carries "sha256=" + hex(HMAC-SHA256(body, secret)).
	SignatureHeader = "X-CorgiSign-Signature"
	// EventHeader names the event that triggered the delivery.
	EventHeader = "X-CorgiSign-Event"
)

// ErrInvalidSignature is returned by ParseRequest when verification fails.
var ErrInvalidSignature = errors.New("corgisign/webhooks: invalid signature")

// Event is a decoded webhook delivery body.
type Event struct {
	// Event is the event name, e.g. "envelope.completed".
	Event string `json:"event"`
	// Timestamp is when the delivery was generated.
	Timestamp time.Time `json:"timestamp"`
	// Data is the event payload (envelope/recipient identifiers and metadata).
	Data map[string]any `json:"data"`
}

// Sign computes the signature header value for body under secret. It matches
// what the CorgiSign dispatcher sends, and is useful in tests.
func Sign(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// Verify reports whether signature is a valid HMAC-SHA256 of body under secret.
// The comparison is constant-time. signature may include or omit the "sha256="
// prefix. An empty signature or secret is always rejected.
func Verify(body []byte, signature, secret string) bool {
	if signature == "" || secret == "" {
		return false
	}
	got := strings.TrimPrefix(signature, "sha256=")
	want := strings.TrimPrefix(Sign(body, secret), "sha256=")
	gotBytes, err := hex.DecodeString(got)
	if err != nil {
		return false
	}
	wantBytes, _ := hex.DecodeString(want)
	return hmac.Equal(gotBytes, wantBytes)
}

// Parse decodes a delivery body into an Event. It does not verify the
// signature; call Verify (or use ParseRequest) first.
func Parse(body []byte) (*Event, error) {
	var e Event
	if err := json.Unmarshal(body, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

// ParseRequest reads r's body, verifies its signature against secret and
// decodes the Event. It returns ErrInvalidSignature when verification fails.
// The request body is fully consumed.
func ParseRequest(r *http.Request, secret string) (*Event, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	if !Verify(body, r.Header.Get(SignatureHeader), secret) {
		return nil, ErrInvalidSignature
	}
	return Parse(body)
}
