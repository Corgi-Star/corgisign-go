package corgisign

import "time"

// EnvelopeStatus is the lifecycle state of an envelope.
type EnvelopeStatus string

// Envelope lifecycle states.
const (
	StatusDraft           EnvelopeStatus = "draft"
	StatusSent            EnvelopeStatus = "sent"
	StatusPartiallySigned EnvelopeStatus = "partially_signed"
	StatusCompleted       EnvelopeStatus = "completed"
	StatusDeclined        EnvelopeStatus = "declined"
	StatusExpired         EnvelopeStatus = "expired"
	StatusVoided          EnvelopeStatus = "voided"
)

// Recipient role values understood by the API.
const (
	RoleSigner   = "signer"
	RoleApprover = "approver"
	RoleCC       = "cc"
	RoleInPerson = "in_person"
)

// Common field type values.
const (
	FieldSignature = "signature"
	FieldInitials  = "initials"
	FieldName      = "name"
	FieldEmail     = "email"
	FieldDate      = "date"
	FieldText      = "text"
	FieldNumber    = "number"
	FieldCheckbox  = "checkbox"
)

// TemplatePlaceholder is a recipient slot defined on a template. Map a real
// Recipient onto it by Role or by ID (as Recipient.PlaceholderID) when creating
// an envelope.
type TemplatePlaceholder struct {
	ID           string `json:"id"`
	Role         string `json:"role"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	SigningOrder int    `json:"signingOrder"`
}

// Template is a reusable envelope blueprint, including its recipient
// placeholders.
type Template struct {
	ID          string                `json:"id"`
	TeamID      string                `json:"teamId"`
	Title       string                `json:"title"`
	Description string                `json:"description"`
	Recipients  []TemplatePlaceholder `json:"recipients"`
	CreatedAt   time.Time             `json:"createdAt"`
}

// FieldState reports a field's metadata and whether it has been filled. The
// stored value itself is never exposed by the API.
type FieldState struct {
	ID          string `json:"id"`
	DocumentID  string `json:"documentId"`
	RecipientID string `json:"recipientId"`
	Type        string `json:"type"`
	Page        int    `json:"page"`
	Required    bool   `json:"required"`
	// Filled reports whether the field has a value.
	Filled bool `json:"filled"`
}

// Recipient is used both as input (when creating an envelope) and as output
// (on a returned envelope). When creating from a template, set Role (or
// PlaceholderID), Name and Email; the remaining fields are populated by the
// server on responses.
type Recipient struct {
	// --- Request fields ---

	// Role is the template placeholder role to map onto, or the recipient's role
	// in document mode (signer, approver, cc, in_person).
	Role string `json:"role,omitempty"`
	// PlaceholderID targets a specific template placeholder by ID.
	PlaceholderID string `json:"placeholderId,omitempty"`
	Name          string `json:"name,omitempty"`
	Email         string `json:"email,omitempty"`
	// SigningOrder is honoured in document mode.
	SigningOrder int `json:"signingOrder,omitempty"`

	// --- Response fields ---

	ID       string     `json:"id,omitempty"`
	Status   string     `json:"status,omitempty"`
	ViewedAt *time.Time `json:"viewedAt,omitempty"`
	SignedAt *time.Time `json:"signedAt,omitempty"`
	// SigningToken is the raw per-recipient signing token, returned only in the
	// create response.
	SigningToken string       `json:"signingToken,omitempty"`
	Fields       []FieldState `json:"fields,omitempty"`
}

// Envelope is a signing package: documents, recipients and lifecycle status.
type Envelope struct {
	ID          string         `json:"id"`
	TeamID      string         `json:"teamId"`
	Title       string         `json:"title"`
	Status      EnvelopeStatus `json:"status"`
	CreatedAt   time.Time      `json:"createdAt"`
	SentAt      *time.Time     `json:"sentAt,omitempty"`
	CompletedAt *time.Time     `json:"completedAt,omitempty"`
	Recipients  []Recipient    `json:"recipients"`
}

// FieldPrefill seeds a recipient's field with a value at creation time. The
// value is stored encrypted at rest.
type FieldPrefill struct {
	// Role of the recipient whose field to prefill.
	Role string `json:"role"`
	// Type of field to fill (text, number, date, name, email, ...).
	Type string `json:"type"`
	// Value to store (encrypted at rest).
	Value string `json:"value"`
}

// DocumentUpload is an inline PDF used to create an envelope in document mode.
type DocumentUpload struct {
	Name string `json:"name"`
	// ContentBase64 is the base64-encoded PDF bytes. Build one from raw bytes
	// with DocumentFromBytes.
	ContentBase64 string `json:"contentBase64"`
}

// CreateEnvelope is the request body for Envelopes.Create. Provide exactly one
// of TemplateID or Document.
type CreateEnvelope struct {
	// TemplateID selects template mode.
	TemplateID string `json:"templateId,omitempty"`
	// TeamID is required in document mode.
	TeamID string `json:"teamId,omitempty"`
	Title  string `json:"title,omitempty"`
	// Recipients map real signers onto template placeholders (template mode) or
	// define the signers (document mode).
	Recipients []Recipient `json:"recipients,omitempty"`
	// Fields prefills recipient field values (template mode).
	Fields []FieldPrefill `json:"fields,omitempty"`
	// Document selects document mode (a base64 PDF + recipients).
	Document *DocumentUpload `json:"document,omitempty"`
	// Send delivers the envelope immediately after creation.
	Send bool `json:"send,omitempty"`
}

// ListEnvelopesParams filters Envelopes.List. The zero value lists recent
// envelopes for the key's scope.
type ListEnvelopesParams struct {
	// Status filters by lifecycle state. Empty means any.
	Status EnvelopeStatus
	// TeamID filters by team (org-wide keys only; ignored for pinned keys).
	TeamID string
	// Limit caps the result count (server max 200; default 50).
	Limit int
}

// Identity describes the API key (or OAuth token) that authenticated a request,
// as returned by Client.WhoAmI. It carries no secret material — only the key's
// organisation/team scoping and capability scopes.
type Identity struct {
	OrganisationID   string     `json:"organisationId"`
	OrganisationName string     `json:"organisationName,omitempty"`
	TeamID           *string    `json:"teamId,omitempty"`
	UserID           *string    `json:"userId,omitempty"`
	Name             string     `json:"name"`
	Prefix           string     `json:"prefix"`
	Environment      string     `json:"environment"`
	Scopes           []string   `json:"scopes"`
	LastUsedAt       *time.Time `json:"lastUsedAt,omitempty"`
}

// Webhook is a registered outbound webhook subscription.
type Webhook struct {
	ID             string   `json:"id"`
	OrganisationID string   `json:"organisationId"`
	TeamID         *string  `json:"teamId,omitempty"`
	URL            string   `json:"url"`
	Events         []string `json:"events"`
	Enabled        bool     `json:"enabled"`
	// Secret is the HMAC signing secret, returned exactly once at creation. Store
	// it and pass it to webhooks.Verify to authenticate inbound deliveries.
	Secret string `json:"secret"`
}

// RegisterWebhook is the request body for Webhooks.Register.
type RegisterWebhook struct {
	URL string `json:"url"`
	// Events to subscribe to (e.g. "envelope.completed"). Empty subscribes to
	// the server default set.
	Events []string `json:"events,omitempty"`
	// Secret optionally pins the signing secret; one is minted when omitted.
	Secret string `json:"secret,omitempty"`
	// TeamID optionally scopes the hook to a team (forced for team-pinned keys).
	TeamID string `json:"teamId,omitempty"`
}
