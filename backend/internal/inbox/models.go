package inbox

import (
	"encoding/json"
	"site-assistant/internal/ai"
	"time"
)

type Conversation struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organization_id"`
	AssistantID    string     `json:"assistant_id"`
	SiteID         string     `json:"site_id"`
	Mode           string     `json:"control_mode"`
	Version        int64      `json:"state_version"`
	Assigned       *string    `json:"assigned_user_id"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	LastMessageAt  *time.Time `json:"last_message_at"`
	LastVisitorAt  *time.Time `json:"last_visitor_at"`
	RequestedAt    *time.Time `json:"requested_at"`
	ClosedAt       *time.Time `json:"closed_at"`
	Site           string     `json:"site"`
	Assistant      string     `json:"assistant"`
	Assignee       string     `json:"assignee"`
	LastContent    string     `json:"last_content"`
	Unread         int        `json:"unread"`
	Generating     bool       `json:"generating"`
}
type Event struct {
	ID        string    `json:"id"`
	Sequence  int64     `json:"sequence"`
	Kind      string    `json:"kind"`
	Actor     *string   `json:"actor_id"`
	Target    *string   `json:"target_id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}
type Page struct {
	Conversation Conversation `json:"conversation"`
	Messages     []ai.Message `json:"messages"`
	Events       []Event      `json:"events"`
	Cursor       int64        `json:"cursor"`
	NextBefore   int64        `json:"next_before"`
	More         bool         `json:"more"`
}
type Change struct {
	Type     string          `json:"type"`
	Sequence int64           `json:"sequence"`
	Value    json.RawMessage `json:"value"`
}
type Filter struct {
	Mode, Site, Assistant, Assigned, Query, From, To, Cursor string
	Limit                                                    int
}
type List struct {
	Items      []Conversation `json:"items"`
	NextCursor string         `json:"next_cursor"`
}
type Action struct {
	RequestID string `json:"request_id"`
	Version   int64  `json:"version"`
	Assignee  string `json:"assignee"`
	Content   string `json:"content"`
}
type Member struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}
type Invitation struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organization_id"`
	Role           string     `json:"role"`
	ExpiresAt      time.Time  `json:"expires_at"`
	RevokedAt      *time.Time `json:"revoked_at"`
	AcceptedAt     *time.Time `json:"accepted_at"`
	URL            string     `json:"url,omitempty"`
}
