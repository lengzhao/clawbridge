package bus

// Peer is the conversation-side endpoint (room / chat).
type Peer struct {
	Kind string `json:"kind,omitempty"`
	ID   string `json:"id,omitempty"`
}

// SenderInfo identifies the message sender for display and policy.
type SenderInfo struct {
	Platform    string `json:"platform,omitempty"`
	PlatformID  string `json:"platform_id,omitempty"`
	CanonicalID string `json:"canonical_id,omitempty"`
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

// InboundMessage is produced by drivers and consumed by the host.
type InboundMessage struct {
	ClientID   string            `json:"client_id"`            // = ClientConfig.ID
	SessionID  string            `json:"session_id"`           // driver-defined stable session key (opaque to host)
	MessageID  string            `json:"message_id,omitempty"`
	Sender     SenderInfo        `json:"sender"`
	Peer       Peer              `json:"peer"`
	Content    string            `json:"content,omitempty"`
	MediaPaths []string          `json:"media_paths,omitempty"`
	ReceivedAt int64             `json:"received_at,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// Recipient is the outbound delivery target.
type Recipient struct {
	SessionID string `json:"session_id"`
	UserID    string `json:"user_id,omitempty"`
	Kind      string `json:"kind,omitempty"`
}

// MediaPart is one attachment; Path is a Media Locator.
type MediaPart struct {
	Path        string `json:"path"`
	Caption     string `json:"caption,omitempty"`
	Filename    string `json:"filename,omitempty"`
	ContentType string `json:"content_type,omitempty"`
}

// OutboundMessage is built by the host and sent by drivers.
type OutboundMessage struct {
	ClientID  string            `json:"client_id"`
	To        Recipient         `json:"to"`
	Text      string            `json:"text,omitempty"`
	Parts     []MediaPart       `json:"parts,omitempty"`
	ReplyToID string            `json:"reply_to_id,omitempty"`
	ThreadID  string            `json:"thread_id,omitempty"`
	// MessageID is the platform message id when referencing an existing message (e.g. EditMessage target).
	// Send implementations ignore it; optional for PublishOutbound.
	MessageID string            `json:"message_id,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// Well-known string values for UpdateStatusRequest.State (drivers may accept extension values).
const (
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
)

// UpdateStatusState is the message-level status for Bridge.UpdateStatus and similar helpers.
// Use the UpdateStatus* constants; for driver-specific state strings, convert with UpdateStatusState("…").
type UpdateStatusState string

// Typed well-known states (wire value equals the string constants above).
const (
	UpdateStatusProcessing UpdateStatusState = StatusProcessing
	UpdateStatusCompleted  UpdateStatusState = StatusCompleted
	UpdateStatusFailed     UpdateStatusState = StatusFailed
)

// UpdateStatusRequest updates UI state for one existing message (message scope, not chat typing).
type UpdateStatusRequest struct {
	ClientID  string            `json:"client_id"`
	To        Recipient         `json:"to"`
	MessageID string            `json:"message_id"`
	State     string            `json:"state"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// EditMessageRequest edits an already sent message. Empty MessageID means last successful Send
// for the same ClientID + To (RecipientKey); see public-api §2.2.1.
type EditMessageRequest struct {
	ClientID  string            `json:"client_id"`
	To        Recipient         `json:"to"`
	MessageID string            `json:"message_id,omitempty"`
	Text      string            `json:"text,omitempty"`
	Parts     []MediaPart       `json:"parts,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// RecipientKey is a stable compound key for (SessionID, Kind, UserID), used for last-sent tracking.
func RecipientKey(to Recipient) string {
	return to.SessionID + "\x00" + to.Kind + "\x00" + to.UserID
}
