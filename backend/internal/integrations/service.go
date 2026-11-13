package integrations

import (
	"context"
	"encoding/json"
	"regexp"
	"site-assistant/internal/core"
	"site-assistant/internal/leads"
	"strings"
	"time"
	"unicode/utf8"
)

type Service struct {
	Auth   *core.Service
	Store  *Store
	Config Config
}

var requestKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,100}$`)
var idPattern = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)

func ValidID(v string) bool { return idPattern.MatchString(v) }

func New(auth *core.Service, c Config) *Service { return &Service{auth, &Store{DB: auth.Store.DB}, c} }

type KeyInput struct {
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
	Days   int      `json:"days"`
}
type EndpointInput struct {
	URL string `json:"url"`
}
type Principal struct {
	OrganizationID, KeyID string
	Scopes                []string
}
type CreateInput struct {
	AssistantID string                     `json:"assistant_id"`
	SiteID      string                     `json:"site_id"`
	Values      map[string]json.RawMessage `json:"values"`
}
type ExternalLead struct {
	ID             string                     `json:"id"`
	AssistantID    string                     `json:"assistant_id"`
	SiteID         *string                    `json:"site_id"`
	ConversationID *string                    `json:"conversation_id"`
	Name           string                     `json:"name"`
	Email          string                     `json:"email"`
	Phone          string                     `json:"phone"`
	Data           map[string]json.RawMessage `json:"data"`
	Status         string                     `json:"status"`
	Version        int64                      `json:"version"`
	CreatedAt      time.Time                  `json:"created_at"`
	UpdatedAt      time.Time                  `json:"updated_at"`
}
type Event struct {
	ID        string    `json:"event_id"`
	Type      string    `json:"event_type"`
	LeadID    *string   `json:"lead_id"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Service) Access(ctx context.Context, user, org string) error {
	_, e := s.Auth.Access(ctx, user, org, true, false)
	return e
}
func (s *Service) CreateKey(ctx context.Context, user, org string, in KeyInput) (map[string]any, error) {
	if e := s.Access(ctx, user, org); e != nil {
		return nil, e
	}
	in.Name = strings.TrimSpace(in.Name)
	if len(in.Name) < 1 || !utf8.ValidString(in.Name) || utf8.RuneCountInString(in.Name) > 100 || strings.ContainsRune(in.Name, 0) || in.Days < 1 || in.Days > 365 || len(in.Scopes) < 1 || len(in.Scopes) > 2 {
		return nil, core.Fail("validation", 422)
	}
	seen := map[string]bool{}
	for _, v := range in.Scopes {
		if v != "leads:read" && v != "leads:write" || seen[v] {
			return nil, core.Fail("validation", 422)
		}
		seen[v] = true
	}
	secret := "sa_" + token()
	id := core.ID()
	if e := s.Store.CreateKey(ctx, org, id, in, digest(secret)); e != nil {
		return nil, e
	}
	return map[string]any{"id": id, "token": secret}, nil
}
func (s *Service) Authenticate(ctx context.Context, secret, scope string) (Principal, error) {
	if len(secret) != 46 || !strings.HasPrefix(secret, "sa_") {
		return Principal{}, core.Fail("api_unauthorized", 401)
	}
	p, e := s.Store.Authenticate(ctx, digest(secret))
	if e != nil {
		return p, e
	}
	for _, v := range p.Scopes {
		if v == scope {
			return p, nil
		}
	}
	return p, core.Fail("api_scope", 403)
}
func (s *Service) CreateEndpoint(ctx context.Context, user, org string, in EndpointInput) (map[string]any, error) {
	if e := s.Access(ctx, user, org); e != nil {
		return nil, e
	}
	in.URL = strings.TrimSpace(in.URL)
	if e := s.Config.ValidateURL(in.URL); e != nil {
		return nil, e
	}
	id, secret := core.ID(), token()
	b, e := seal(s.Config.Key, org, id, secret)
	if e != nil {
		return nil, e
	}
	if e = s.Store.CreateEndpoint(ctx, org, id, in.URL, b); e != nil {
		return nil, e
	}
	return map[string]any{"id": id, "secret": secret}, nil
}
func (s *Service) Rotate(ctx context.Context, user, org, id string) (map[string]string, error) {
	if e := s.Access(ctx, user, org); e != nil {
		return nil, e
	}
	secret := token()
	b, e := seal(s.Config.Key, org, id, secret)
	if e == nil {
		e = s.Store.Rotate(ctx, org, id, b)
	}
	return map[string]string{"secret": secret}, e
}
func (s *Service) CreateLead(ctx context.Context, p Principal, key string, in CreateInput) (ExternalLead, error) {
	if !requestKeyPattern.MatchString(key) || len(in.Values) > 12 || !ValidID(in.AssistantID) || in.SiteID != "" && !ValidID(in.SiteID) {
		return ExternalLead{}, core.Fail("validation", 422)
	}
	body, _ := json.Marshal(in)
	hash := digest(string(body))
	if v, found, e := s.Store.Replay(ctx, p.OrganizationID, key, hash); found || e != nil {
		return v, e
	}
	conf, e := s.Store.LeadSettings(ctx, p.OrganizationID, in.AssistantID)
	if e != nil {
		return ExternalLead{}, e
	}
	values := map[string]leads.Value{}
	for key, raw := range in.Values {
		var field *leads.Field
		for i := range conf.Fields {
			if conf.Fields[i].Key == key && conf.Fields[i].Active {
				field = &conf.Fields[i]
				break
			}
		}
		if field == nil {
			return ExternalLead{}, core.Fail("lead_unknown_field", 422)
		}
		v, code := leads.ValidateValue(*field, raw)
		if code != "" {
			return ExternalLead{}, core.Fail(code, 422)
		}
		values[key] = leads.Value{Value: v, Valid: true, Source: "integration", UpdatedAt: time.Now().UTC()}
	}
	if len(leads.Missing(conf, values)) > 0 {
		return ExternalLead{}, core.Fail("lead_incomplete", 422)
	}
	return s.Store.CreateLead(ctx, p.OrganizationID, key, hash, in, conf, values)
}
