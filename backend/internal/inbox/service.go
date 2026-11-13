package inbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"site-assistant/internal/ai"
	"site-assistant/internal/core"
	"strings"
	"unicode/utf8"
)

type Service struct {
	Auth   *core.Service
	AI     *ai.Service
	Store  *Store
	AppURL string
}

func New(auth *core.Service, engine *ai.Service, url string) *Service {
	return &Service{auth, engine, &Store{auth.Store.DB}, url}
}

var uuid = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

func validText(s string, n int) bool {
	return utf8.ValidString(s) && strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= n && !strings.ContainsRune(s, 0)
}
func digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (s *Service) Access(ctx context.Context, user, org, id string) (string, Conversation, error) {
	role, e := s.Auth.Access(ctx, user, org, false, false)
	if e != nil {
		return role, Conversation{}, e
	}
	if !uuid.MatchString(id) {
		return role, Conversation{}, core.Fail("not_found", 404)
	}
	c, e := s.Store.Conversation(ctx, org, id, user, role)
	return role, c, e
}
func (s *Service) List(ctx context.Context, user, org string, f Filter) (List, error) {
	role, e := s.Auth.Access(ctx, user, org, false, false)
	if e != nil {
		return List{}, e
	}
	if f.Limit == 0 {
		f.Limit = 30
	}
	if f.Limit < 1 || f.Limit > 50 || utf8.RuneCountInString(f.Query) > 200 || strings.ContainsRune(f.Query, 0) {
		return List{}, core.Fail("validation", 422)
	}
	for _, v := range []string{f.Site, f.Assistant} {
		if v != "" && !uuid.MatchString(v) {
			return List{}, core.Fail("validation", 422)
		}
	}
	if f.Assigned != "" && f.Assigned != "me" && !uuid.MatchString(f.Assigned) {
		return List{}, core.Fail("validation", 422)
	}
	if f.Assigned == "me" {
		f.Assigned = user
	}
	if f.Mode != "" && f.Mode != "ai_active" && f.Mode != "human_active" && f.Mode != "human_requested" && f.Mode != "closed" {
		return List{}, core.Fail("validation", 422)
	}
	return s.Store.List(ctx, org, user, role, f)
}
func (s *Service) Page(ctx context.Context, user, org, id string, after, before int64) (Page, error) {
	role, _, e := s.Access(ctx, user, org, id)
	if e != nil {
		return Page{}, e
	}
	return s.Store.Page(ctx, org, id, user, role, after, before, false)
}
func (s *Service) Action(ctx context.Context, user, org, id, kind string, in Action) (Conversation, error) {
	role, _, e := s.Access(ctx, user, org, id)
	if e != nil {
		return Conversation{}, e
	}
	if !uuid.MatchString(in.RequestID) || in.Version < 1 || (kind == "note" && !validText(in.Content, 4000)) || (kind != "note" && in.Content != "") || (kind == "assign" && in.Assignee != "" && !uuid.MatchString(in.Assignee)) {
		return Conversation{}, core.Fail("validation", 422)
	}
	c, active, e := s.Store.Action(ctx, org, id, user, role, kind, in)
	if e == nil && active != "" {
		s.AI.SignalID(active)
	}
	return c, e
}
func (s *Service) Reply(ctx context.Context, user, org, id string, in Action) (ai.Message, error) {
	role, _, e := s.Access(ctx, user, org, id)
	if e != nil {
		return ai.Message{}, e
	}
	if !uuid.MatchString(in.RequestID) || in.Version < 1 || !validText(in.Content, 2000) {
		return ai.Message{}, core.Fail("validation", 422)
	}
	return s.Store.Reply(ctx, org, id, user, role, in)
}
func (s *Service) Read(ctx context.Context, user, org, id string, sequence int64) error {
	_, _, e := s.Access(ctx, user, org, id)
	if e != nil {
		return e
	}
	if sequence < 0 {
		return core.Fail("validation", 422)
	}
	return s.Store.Read(ctx, org, id, user, sequence)
}
func (s *Service) VisitorPage(ctx context.Context, widget, session, token, origin string, after, before int64) (Page, error) {
	v, e := s.AI.Store.WebsiteScope(ctx, widget, session, token, origin)
	if e != nil {
		return Page{}, e
	}
	return s.Store.Page(ctx, v.OrganizationID, v.ConversationID, "", "", after, before, true)
}
func (s *Service) RequestHuman(ctx context.Context, widget, session, token, origin string, in Action) (Conversation, error) {
	v, e := s.AI.Store.WebsiteScope(ctx, widget, session, token, origin)
	if e != nil {
		return Conversation{}, e
	}
	if !uuid.MatchString(in.RequestID) {
		return Conversation{}, core.Fail("validation", 422)
	}
	c, active, e := s.Store.Action(ctx, v.OrganizationID, v.ConversationID, "", "visitor", "human_requested", in)
	if e == nil && active != "" {
		s.AI.SignalID(active)
	}
	return c, e
}
func (s *Service) Members(ctx context.Context, user, org string) ([]Member, error) {
	if _, e := s.Auth.Access(ctx, user, org, false, false); e != nil {
		return nil, e
	}
	return s.Store.Members(ctx, org)
}
func (s *Service) Summary(ctx context.Context, user, org string) (map[string]int, error) {
	role, e := s.Auth.Access(ctx, user, org, false, false)
	if e != nil {
		return nil, e
	}
	return s.Store.Summary(ctx, org, user, role)
}
