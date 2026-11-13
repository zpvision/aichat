package widget

import (
	"site-assistant/internal/ai"
	"site-assistant/internal/inbox"
	"time"
)

type Copy struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	Greeting string `json:"greeting"`
}
type Settings struct {
	Texts           map[string]Copy `json:"texts"`
	Primary         string          `json:"primary"`
	Secondary       string          `json:"secondary"`
	UserColor       string          `json:"user_color"`
	LauncherColor   string          `json:"launcher_color"`
	Position        string          `json:"position"`
	OffsetX         int             `json:"offset_x"`
	OffsetY         int             `json:"offset_y"`
	ShowLauncher    bool            `json:"show_launcher"`
	RememberSession bool            `json:"remember_session"`
	AllowNew        bool            `json:"allow_new"`
	DefaultLocale   string          `json:"default_locale"`
	AutoLocale      bool            `json:"auto_locale"`
	PrivacyURL      string          `json:"privacy_url"`
}

func Defaults() Settings {
	return Settings{Texts: map[string]Copy{}, Primary: "#1890FF", Secondary: "#637BFF", UserColor: "#EAF4FF", LauncherColor: "#1890FF", Position: "right", OffsetX: 24, OffsetY: 24, ShowLauncher: true, RememberSession: true, AllowNew: true, DefaultLocale: "en", AutoLocale: true}
}

type Widget struct {
	ID                string     `json:"id"`
	OrganizationID    string     `json:"organization_id"`
	AssistantID       string     `json:"assistant_id"`
	SiteID            string     `json:"site_id"`
	Domain            string     `json:"domain"`
	Enabled           bool       `json:"enabled"`
	Origins           []string   `json:"origins"`
	Settings          Settings   `json:"settings"`
	LogoKey           string     `json:"-"`
	VerificationToken string     `json:"verification_token"`
	VerifiedAt        *time.Time `json:"verified_at"`
	CreatedAt         time.Time  `json:"created_at"`
}
type Admin struct {
	Widget              Widget `json:"widget"`
	Snippet             string `json:"snippet"`
	VerificationPath    string `json:"verification_path"`
	VerificationContent string `json:"verification_content"`
	LogoURL             string `json:"logo_url"`
	PublicURL           string `json:"public_url"`
	LocalDevelopment    bool   `json:"local_development"`
}
type Input struct {
	Enabled  *bool     `json:"enabled"`
	Origins  *[]string `json:"origins"`
	Settings *Settings `json:"settings"`
}
type PublicConfig struct {
	ID              string   `json:"id"`
	Settings        Settings `json:"settings"`
	LogoURL         string   `json:"logo_url"`
	Provider        string   `json:"provider"`
	Demo            bool     `json:"demo"`
	Available       bool     `json:"available"`
	MaxMessageChars int      `json:"max_message_chars"`
}
type Session struct {
	ID        string    `json:"id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}
type PublicMessage struct {
	Sequence  int64     `json:"sequence"`
	ChangeSeq int64     `json:"change_seq"`
	ID        string    `json:"id"`
	RequestID string    `json:"request_id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

func PublicPage(p inbox.Page) map[string]any {
	messages := []PublicMessage{}
	for _, m := range p.Messages {
		messages = append(messages, FromMessage(m, m.RequestID))
	}
	events := []map[string]any{}
	for _, ev := range p.Events {
		events = append(events, map[string]any{"id": ev.ID, "sequence": ev.Sequence, "kind": ev.Kind, "created_at": ev.CreatedAt})
	}
	return map[string]any{"messages": messages, "events": events, "control_mode": p.Conversation.Mode, "state_version": p.Conversation.Version, "cursor": p.Cursor, "next_before": p.NextBefore, "more": p.More}
}
func FromMessage(m ai.Message, requestID string) PublicMessage {
	return PublicMessage{ID: m.ID, RequestID: requestID, Role: m.Role, Content: m.Content, Status: m.Status, CreatedAt: m.CreatedAt, Sequence: m.Sequence, ChangeSeq: m.ChangeSeq}
}

func publicMessages(d ai.Detail) []PublicMessage {
	out := []PublicMessage{}
	for _, m := range d.Messages {
		v := PublicMessage{ID: m.ID, Role: m.Role, Content: m.Content, Status: m.Status, CreatedAt: m.CreatedAt}
		for _, e := range d.Executions {
			if e.ID == m.ExecutionID {
				v.RequestID = e.RequestID
				break
			}
		}
		out = append(out, v)
	}
	return out
}
