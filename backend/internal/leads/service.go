package leads

import (
	"context"
	"encoding/json"
	"site-assistant/internal/ai"
	"site-assistant/internal/core"
	"site-assistant/internal/inbox"
	"time"
)

type Service struct {
	Auth      *core.Service
	AI        *ai.Service
	Inbox     *inbox.Service
	Store     *Store
	Extractor StructuredExtractor
	slots     chan struct{}
}

func (s *Service) Enabled(ctx context.Context, org, assistant string) (bool, error) {
	v, e := s.Store.Settings(ctx, s.Store.DB, org, assistant)
	return v.Enabled, e
}

func New(auth *core.Service, engine *ai.Service, box *inbox.Service) *Service {
	s := &Service{Auth: auth, AI: engine, Inbox: box, Store: &Store{DB: auth.Store.DB}, slots: make(chan struct{}, 2)}
	switch engine.Config.Provider {
	case "mock":
		s.Extractor = &MockStructuredExtractor{}
	case "ollama":
		c := engine.Config
		c.Timeout = 8 * time.Second
		c.MaxOutputTokens = 2048
		s.Extractor = &OllamaStructuredExtractor{ai.NewOllama(c)}
	}
	engine.Collection = s
	return s
}
func (s *Service) Settings(ctx context.Context, user, org, assistant string) (Settings, error) {
	if _, e := s.Auth.Access(ctx, user, org, true, false); e != nil {
		return Settings{}, e
	}
	if _, e := s.Auth.Assistant(ctx, user, org, assistant); e != nil {
		return Settings{}, e
	}
	return s.Store.Settings(ctx, s.Store.DB, org, assistant)
}
func (s *Service) SaveSettings(ctx context.Context, user, org, assistant string, in Settings) (Settings, error) {
	if _, e := s.Settings(ctx, user, org, assistant); e != nil {
		return Settings{}, e
	}
	if e := ValidateSettings(&in); e != nil {
		return Settings{}, e
	}
	return s.Store.SaveSettings(ctx, org, assistant, in)
}
func (s *Service) scope(ctx context.Context, user, org, id string) (ai.Scope, string, error) {
	role, c, e := s.Inbox.Access(ctx, user, org, id)
	return ai.Scope{OrganizationID: org, AssistantID: c.AssistantID, ConversationID: id}, role, e
}
func (s *Service) view(d Collection, c inbox.Conversation, conf Settings, privacy string, public bool) View {
	if d.State != "idle" {
		conf = d.Snapshot
		privacy = d.PrivacyURL
	}
	fields := []Field{}
	for _, f := range conf.Fields {
		if !f.Active {
			continue
		}
		f.Description = ""
		fields = append(fields, f)
	}
	missing := []string{}
	if d.State == "collecting" || d.State == "confirmation" {
		missing = Missing(conf, d.Values)
	}
	lead := d.LeadID
	if public {
		lead = nil
	}
	return View{State: d.State, Version: d.Version, ConversationVersion: c.Version, Values: d.Values, Fields: fields, Missing: missing, Paused: c.Mode != "ai_active", Enabled: conf.Enabled, PrivacyURL: privacy, Success: conf.Success, Error: d.Error, LeadID: lead, Demo: s.AI.Config.Provider == "mock"}
}
func (s *Service) View(ctx context.Context, v ai.Scope, public bool) (View, error) {
	return s.viewLocked(ctx, v, public, "", "")
}
func (s *Service) viewLocked(ctx context.Context, v ai.Scope, public bool, user, role string) (View, error) {
	c, d, conf, privacy, e := s.Store.readView(ctx, v, user, role)
	if e != nil {
		return View{}, e
	}
	return s.view(d, c, conf, privacy, public), nil
}

func (s *Service) StaffView(ctx context.Context, user, org, id string) (View, error) {
	v, role, e := s.scope(ctx, user, org, id)
	if e != nil {
		return View{}, e
	}
	return s.viewLocked(ctx, v, false, user, role)
}
func (s *Service) StaffAction(ctx context.Context, user, org, id, kind string, in Input) (View, error) {
	v, role, e := s.scope(ctx, user, org, id)
	if e != nil {
		return View{}, e
	}
	_, c, e := s.Store.Mutate(ctx, v, user, role, kind, in)
	_ = c
	if e != nil {
		return View{}, e
	}
	return s.StaffView(ctx, user, org, id)
}
func (s *Service) VisitorScope(ctx context.Context, widget, session, token, origin string) (ai.Scope, error) {
	return s.AI.Store.WebsiteScope(ctx, widget, session, token, origin)
}
func (s *Service) VisitorView(ctx context.Context, widget, session, token, origin string) (View, error) {
	v, e := s.VisitorScope(ctx, widget, session, token, origin)
	if e != nil {
		return View{}, e
	}
	return s.View(ctx, v, true)
}
func (s *Service) VisitorAction(ctx context.Context, widget, session, token, origin, kind string, in Input) (View, error) {
	v, e := s.VisitorScope(ctx, widget, session, token, origin)
	if e != nil {
		return View{}, e
	}
	if _, _, e = s.Store.Mutate(ctx, v, "", "visitor", kind, in); e != nil {
		return View{}, e
	}
	return s.View(ctx, v, true)
}
func encode(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
