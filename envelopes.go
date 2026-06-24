package corgisign

import (
	"context"
	"encoding/base64"
	"net/url"
	"strconv"
)

// EnvelopesService accesses the envelope endpoints.
type EnvelopesService struct {
	c *Client
}

// Create creates an envelope from a template or an uploaded document. Provide
// exactly one of CreateEnvelope.TemplateID or CreateEnvelope.Document. Set
// CreateEnvelope.Send to deliver immediately. The returned envelope's
// recipients each carry a one-time SigningToken.
//
// Pass WithIdempotencyKey to make a retried create safe.
func (s *EnvelopesService) Create(ctx context.Context, req CreateEnvelope, opts ...RequestOption) (*Envelope, error) {
	var env Envelope
	if err := s.c.do(ctx, "POST", "/envelopes", req, &env, opts); err != nil {
		return nil, err
	}
	return &env, nil
}

// Get fetches an envelope's status, recipients and per-field state.
func (s *EnvelopesService) Get(ctx context.Context, id string, opts ...RequestOption) (*Envelope, error) {
	var env Envelope
	if err := s.c.do(ctx, "GET", "/envelopes/"+url.PathEscape(id), nil, &env, opts); err != nil {
		return nil, err
	}
	return &env, nil
}

// Send delivers a draft envelope to its recipients. It returns an *Error with
// IsConflict true if the envelope is not a draft, or IsUnprocessable if it has
// no signer/approver recipients.
func (s *EnvelopesService) Send(ctx context.Context, id string, opts ...RequestOption) (*Envelope, error) {
	var env Envelope
	if err := s.c.do(ctx, "POST", "/envelopes/"+url.PathEscape(id)+"/send", nil, &env, opts); err != nil {
		return nil, err
	}
	return &env, nil
}

// List returns envelopes matching params (status / team / limit). Additional
// RequestOptions may be passed for headers.
func (s *EnvelopesService) List(ctx context.Context, params ListEnvelopesParams, opts ...RequestOption) ([]Envelope, error) {
	if params.Status != "" {
		opts = append(opts, WithQuery("status", string(params.Status)))
	}
	if params.TeamID != "" {
		opts = append(opts, WithQuery("teamId", params.TeamID))
	}
	if params.Limit > 0 {
		opts = append(opts, WithQuery("limit", strconv.Itoa(params.Limit)))
	}
	var out struct {
		Envelopes []Envelope `json:"envelopes"`
	}
	if err := s.c.do(ctx, "GET", "/envelopes", nil, &out, opts); err != nil {
		return nil, err
	}
	return out.Envelopes, nil
}

// DownloadSigned downloads the executed, PAdES-sealed PDF for a completed
// envelope (its primary document, i.e. the lowest-position one) and returns the
// raw bytes. The seal is produced when the envelope completes, so poll Get until
// it reports "completed" (or subscribe to the envelope.completed webhook) first.
//
// On a non-2xx the returned error is an *Error: IsConflict is true while the
// envelope has not completed, and IsNotFound is true for an unknown envelope or
// an unavailable seal.
func (s *EnvelopesService) DownloadSigned(ctx context.Context, id string, opts ...RequestOption) ([]byte, error) {
	return s.c.doDownload(ctx, "/envelopes/"+url.PathEscape(id)+"/signed", opts)
}

// DownloadDocument downloads the sealed PDF of one specific document within an
// envelope (use it for multi-document envelopes). It has the same gating as
// DownloadSigned.
func (s *EnvelopesService) DownloadDocument(ctx context.Context, envelopeID, documentID string, opts ...RequestOption) ([]byte, error) {
	return s.c.doDownload(ctx,
		"/envelopes/"+url.PathEscape(envelopeID)+"/documents/"+url.PathEscape(documentID)+"/download", opts)
}

// DocumentFromBytes builds a DocumentUpload from raw PDF bytes, base64-encoding
// them for transport.
func DocumentFromBytes(name string, pdf []byte) *DocumentUpload {
	return &DocumentUpload{
		Name:          name,
		ContentBase64: base64.StdEncoding.EncodeToString(pdf),
	}
}
