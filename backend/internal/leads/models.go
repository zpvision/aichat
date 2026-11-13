package leads

import (
	"encoding/json"
	"time"
)

type Field struct {
	Key         string            `json:"key"`
	Label       string            `json:"label"`
	Labels      map[string]string `json:"labels"`
	Description string            `json:"description"`
	Type        string            `json:"type"`
	Required    bool              `json:"required"`
	Active      bool              `json:"active"`
	Position    int               `json:"position"`
	Options     []string          `json:"options"`
	MinLength   int               `json:"min_length"`
	MaxLength   int               `json:"max_length"`
	Min         *float64          `json:"min"`
	Max         *float64          `json:"max"`
	Integer     bool              `json:"integer"`
}
type Settings struct {
	Enabled        bool              `json:"enabled"`
	Revision       int64             `json:"revision"`
	Title          string            `json:"title"`
	Goal           string            `json:"goal"`
	RequireContact bool              `json:"require_contact"`
	Success        map[string]string `json:"success_messages"`
	Fields         []Field           `json:"fields"`
}
type Value struct {
	Value     json.RawMessage `json:"value"`
	Valid     bool            `json:"valid"`
	Error     string          `json:"error_code"`
	Source    string          `json:"source"`
	MessageID string          `json:"message_id,omitempty"`
	Sequence  int64           `json:"sequence"`
	UpdatedAt time.Time       `json:"updated_at"`
}
type Collection struct {
	ID               string           `json:"id"`
	State            string           `json:"state"`
	Version          int64            `json:"version"`
	Snapshot         Settings         `json:"snapshot"`
	PrivacyURL       string           `json:"privacy_url"`
	Values           map[string]Value `json:"values"`
	Attempts         int              `json:"extract_attempts"`
	LastAttempt      *string          `json:"last_attempt_message_id"`
	Error            string           `json:"error_code"`
	LeadID           *string          `json:"lead_id"`
	ConfirmedVersion *int64           `json:"confirmed_version"`
	SubmittedAt      *time.Time       `json:"submitted_at"`
}
type Input struct {
	Version             int64                      `json:"version"`
	ConversationVersion int64                      `json:"conversation_version"`
	Values              map[string]json.RawMessage `json:"values,omitempty"`
}
type View struct {
	State               string            `json:"state"`
	Version             int64             `json:"version"`
	ConversationVersion int64             `json:"conversation_version"`
	Values              map[string]Value  `json:"values"`
	Fields              []Field           `json:"fields"`
	Missing             []string          `json:"missing"`
	Paused              bool              `json:"paused"`
	Enabled             bool              `json:"enabled"`
	PrivacyURL          string            `json:"privacy_url"`
	Success             map[string]string `json:"success_messages"`
	Error               string            `json:"error_code"`
	LeadID              *string           `json:"lead_id,omitempty"`
	Demo                bool              `json:"demo"`
}
type Lead struct {
	ID                  string                     `json:"id"`
	OrganizationID      string                     `json:"organization_id"`
	AssistantID         string                     `json:"assistant_id"`
	SiteID              string                     `json:"site_id"`
	ConversationID      string                     `json:"conversation_id"`
	Name                string                     `json:"name"`
	Email               string                     `json:"email"`
	Phone               string                     `json:"phone"`
	Data                map[string]json.RawMessage `json:"data"`
	Provenance          map[string]Value           `json:"provenance"`
	ContactKeys         map[string]string          `json:"contact_keys"`
	Snapshot            Settings                   `json:"snapshot"`
	Values              map[string]Value           `json:"values"`
	PrivacyURL          string                     `json:"privacy_url"`
	Status              string                     `json:"status"`
	Assigned            *string                    `json:"assigned_user_id"`
	Source              string                     `json:"source"`
	Version             int64                      `json:"version"`
	Site                string                     `json:"site"`
	CreatedAt           time.Time                  `json:"created_at"`
	UpdatedAt           time.Time                  `json:"updated_at"`
	CanOpenConversation bool                       `json:"can_open_conversation"`
}
type Event struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Actor     *string   `json:"actor_id"`
	Status    string    `json:"status"`
	Target    *string   `json:"target_id"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
}
type Detail struct {
	Lead   Lead    `json:"lead"`
	Events []Event `json:"events"`
}
type LeadPatch struct {
	Version  int64                      `json:"version"`
	Status   *string                    `json:"status"`
	Assigned *string                    `json:"assigned_user_id"`
	Values   map[string]json.RawMessage `json:"values"`
	Note     *string                    `json:"note"`
}
type Filter struct {
	Status, Site, Assistant, Assigned, From, To, Cursor string
	Limit                                               int
}

func Defaults() Settings {
	return Settings{Title: "", Goal: "", RequireContact: true, Revision: 1, Success: map[string]string{}, Fields: []Field{{Key: "full_name", Label: "", Type: "text", Active: true, MaxLength: 120}, {Key: "phone", Type: "phone", Active: true, Position: 1}, {Key: "email", Type: "email", Active: true, Position: 2}, {Key: "company_name", Type: "text", Active: true, MaxLength: 160, Position: 3}, {Key: "inn", Type: "inn_ru", Position: 4}, {Key: "request_description", Type: "text", Active: true, MaxLength: 1000, Position: 5}}}
}
