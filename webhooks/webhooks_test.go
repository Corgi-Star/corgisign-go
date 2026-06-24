package webhooks

import (
	"bytes"
	"net/http/httptest"
	"testing"
)

func TestSignVerifyRoundTrip(t *testing.T) {
	body := []byte(`{"event":"envelope.completed","data":{"envelopeId":"e1"}}`)
	secret := "whsec_test"
	sig := Sign(body, secret)

	if !Verify(body, sig, secret) {
		t.Fatal("Verify rejected a valid signature")
	}
	// Without the sha256= prefix it must still verify.
	if !Verify(body, sig[len("sha256="):], secret) {
		t.Fatal("Verify rejected a valid bare-hex signature")
	}
	if Verify(body, sig, "wrong") {
		t.Fatal("Verify accepted a wrong secret")
	}
	if Verify(append(body, '!'), sig, secret) {
		t.Fatal("Verify accepted a tampered body")
	}
	if Verify(body, "", secret) || Verify(body, sig, "") {
		t.Fatal("Verify accepted empty signature/secret")
	}
}

func TestParseRequest(t *testing.T) {
	body := []byte(`{"event":"envelope.sent","timestamp":"2026-06-09T10:00:00Z","data":{"envelopeId":"e1"}}`)
	secret := "whsec_test"
	r := httptest.NewRequest("POST", "/hook", bytes.NewReader(body))
	r.Header.Set(SignatureHeader, Sign(body, secret))

	evt, err := ParseRequest(r, secret)
	if err != nil {
		t.Fatal(err)
	}
	if evt.Event != "envelope.sent" || evt.Data["envelopeId"] != "e1" {
		t.Fatalf("unexpected event: %+v", evt)
	}

	bad := httptest.NewRequest("POST", "/hook", bytes.NewReader(body))
	bad.Header.Set(SignatureHeader, "sha256=deadbeef")
	if _, err := ParseRequest(bad, secret); err != ErrInvalidSignature {
		t.Fatalf("want ErrInvalidSignature, got %v", err)
	}
}
