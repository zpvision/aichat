package widget

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"golang.org/x/net/idna"
	"image"
	_ "image/jpeg"
	"image/png"
	"net"
	"net/url"
	"regexp"
	"site-assistant/internal/ai"
	"site-assistant/internal/core"
	"site-assistant/internal/inbox"
	"site-assistant/internal/knowledge"
	"site-assistant/internal/leads"
	"strings"
	"unicode/utf8"
)

type Service struct {
	Auth     *core.Service
	AI       *ai.Service
	Store    *Store
	Config   Config
	Files    knowledge.FileStorage
	Importer *knowledge.Importer
	Limiter  *Limiter
	Inbox    *inbox.Service
	Leads    *leads.Service
}

func New(auth *core.Service, engine *ai.Service, c Config) *Service {
	engine.WebsiteLimits = c.Limits
	box := inbox.New(auth, engine, c.AppURL)
	return &Service{Auth: auth, AI: engine, Store: &Store{DB: auth.Store.DB}, Config: c, Files: knowledge.LocalStorage{Root: c.FileRoot}, Importer: knowledge.NewImporter(), Limiter: NewLimiter(), Inbox: box, Leads: leads.New(auth, engine, box)}
}
func (s *Service) Access(ctx context.Context, user, org, site string, write bool) error {
	if _, e := s.Auth.Access(ctx, user, org, write, false); e != nil {
		return e
	}
	_, _, e := s.Store.Site(ctx, org, site)
	return e
}
func randomToken() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func (s *Service) admin(w Widget) Admin {
	return Admin{Widget: w, Snippet: fmt.Sprintf(`<script src="%s/widget/loader.js" data-widget-id="%s" async></script>`, s.Config.PublicURL, w.ID), VerificationPath: "https://" + w.Domain + "/.well-known/site-assistant-verification.txt", VerificationContent: w.VerificationToken, LogoURL: s.logoURL(w), PublicURL: s.Config.PublicURL, LocalDevelopment: s.Config.AllowLocalhost}
}
func (s *Service) logoURL(w Widget) string {
	if w.LogoKey == "" {
		return ""
	}
	return s.Config.PublicURL + "/api/v1/public/widgets/" + w.ID + "/logo?v=" + w.LogoKey
}
func (s *Service) Admin(ctx context.Context, user, org, site string) (Admin, error) {
	if e := s.Access(ctx, user, org, site, false); e != nil {
		return Admin{}, e
	}
	w, e := s.Store.Admin(ctx, org, site)
	return s.admin(w), e
}
func (s *Service) Create(ctx context.Context, user, org, site string) (Admin, error) {
	if e := s.Access(ctx, user, org, site, true); e != nil {
		return Admin{}, e
	}
	a, domain, e := s.Store.Site(ctx, org, site)
	if e != nil {
		return Admin{}, e
	}
	w, e := s.Store.Create(ctx, Widget{ID: core.ID(), OrganizationID: org, AssistantID: a, SiteID: site, Domain: domain, Origins: []string{"https://" + domain}, Settings: Defaults(), VerificationToken: randomToken()})
	return s.admin(w), e
}
func NormalizeOrigin(raw string) (string, error) {
	u, e := url.Parse(raw)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "http" && u.Scheme != "https") {
		return "", core.Fail("widget_origin", 403)
	}
	if strings.ContainsAny(u.Host, "'\"\\ ;") {
		return "", core.Fail("widget_origin", 403)
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	var e2 error
	if net.ParseIP(host) == nil {
		host, e2 = idna.Lookup.ToASCII(host)
	}
	if e2 != nil {
		return "", core.Fail("widget_origin", 403)
	}
	port := u.Port()
	if port == "443" && u.Scheme == "https" || port == "80" && u.Scheme == "http" {
		port = ""
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return u.Scheme + "://" + host, nil
}
func localHost(host string) bool { return host == "localhost" || host == "127.0.0.1" || host == "::1" }
func (s *Service) origins(raw []string, domain string) ([]string, error) {
	if len(raw) < 1 || len(raw) > 10 {
		return nil, core.Fail("validation", 422)
	}
	out := []string{}
	seen := map[string]bool{}
	for _, r := range raw {
		o, e := NormalizeOrigin(r)
		if e != nil {
			return nil, core.Fail("validation", 422)
		}
		u, _ := url.Parse(o)
		host := u.Hostname()
		if localHost(host) {
			if !s.Config.AllowLocalhost || s.Config.Production {
				return nil, core.Fail("validation", 422)
			}
		} else {
			normalized, e := core.NormalizeDomain(host)
			if e != nil || normalized != domain && !strings.HasSuffix(normalized, "."+domain) || u.Scheme != "https" || u.Port() != "" && u.Port() != "443" {
				return nil, core.Fail("validation", 422)
			}
		}
		if !seen[o] {
			out = append(out, o)
			seen[o] = true
		}
	}
	return out, nil
}

var color = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func validate(v Settings) error {
	for _, c := range []string{v.Primary, v.Secondary, v.UserColor, v.LauncherColor} {
		if !color.MatchString(c) {
			return core.Fail("validation", 422)
		}
	}
	if v.Position != "left" && v.Position != "right" || v.OffsetX < 8 || v.OffsetX > 80 || v.OffsetY < 8 || v.OffsetY > 80 || !locale(v.DefaultLocale) || len(v.Texts) > 4 {
		return core.Fail("validation", 422)
	}
	for lang, t := range v.Texts {
		if !locale(lang) || utf8.RuneCountInString(t.Title) > 60 || utf8.RuneCountInString(t.Subtitle) > 140 || utf8.RuneCountInString(t.Greeting) > 400 || strings.ContainsRune(t.Title+t.Subtitle+t.Greeting, 0) {
			return core.Fail("validation", 422)
		}
	}
	if v.PrivacyURL != "" {
		u, e := url.Parse(v.PrivacyURL)
		if e != nil || u.Host == "" || u.User != nil || u.Scheme != "https" || len(v.PrivacyURL) > 1000 {
			return core.Fail("validation", 422)
		}
	}
	return nil
}
func locale(v string) bool { return v == "ru" || v == "en" || v == "cs" || v == "de" }
func (s *Service) Update(ctx context.Context, user, org, site string, in Input) (Admin, error) {
	if e := s.Access(ctx, user, org, site, true); e != nil {
		return Admin{}, e
	}
	w, e := s.Store.Admin(ctx, org, site)
	if e != nil {
		return Admin{}, e
	}
	if in.Settings != nil {
		if in.Settings.Texts == nil {
			in.Settings.Texts = map[string]Copy{}
		}
		if e = validate(*in.Settings); e != nil {
			return Admin{}, e
		}
		w.Settings = *in.Settings
	}
	if in.Origins != nil {
		w.Origins, e = s.origins(*in.Origins, w.Domain)
		if e != nil {
			return Admin{}, e
		}
	}
	if in.Enabled != nil {
		w.Enabled = *in.Enabled
	}
	if w.Enabled && s.Config.Production && w.VerifiedAt == nil {
		return Admin{}, core.Fail("widget_unverified", 422)
	}
	w, e = s.Store.Save(ctx, w)
	return s.admin(w), e
}
func (s *Service) Verify(ctx context.Context, user, org, site string) (Admin, error) {
	if e := s.Access(ctx, user, org, site, true); e != nil {
		return Admin{}, e
	}
	w, e := s.Store.Admin(ctx, org, site)
	if e != nil {
		return Admin{}, e
	}
	body, e := s.Importer.Verification(ctx, "https://"+w.Domain+"/.well-known/site-assistant-verification.txt")
	if e != nil || strings.TrimSpace(string(body)) != w.VerificationToken {
		return Admin{}, core.Fail("widget_verification", 422)
	}
	if e = s.Store.Verified(ctx, org, site); e != nil {
		return Admin{}, e
	}
	return s.Admin(ctx, user, org, site)
}
func (s *Service) UploadLogo(ctx context.Context, user, org, site string, b []byte) (Admin, error) {
	if e := s.Access(ctx, user, org, site, true); e != nil {
		return Admin{}, e
	}
	w, e := s.Store.Admin(ctx, org, site)
	if e != nil {
		return Admin{}, e
	}
	cfg, format, e := image.DecodeConfig(bytes.NewReader(b))
	if e != nil || len(b) > 256<<10 || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 512 || cfg.Height > 512 || (format != "png" && format != "jpeg") {
		return Admin{}, core.Fail("widget_logo", 422)
	}
	im, _, e := image.Decode(bytes.NewReader(b))
	if e != nil {
		return Admin{}, core.Fail("widget_logo", 422)
	}
	var out bytes.Buffer
	png.Encode(&out, im)
	key, e := s.Files.Put(ctx, out.Bytes())
	if e != nil {
		return Admin{}, e
	}
	if e = s.Store.Logo(ctx, org, site, key); e != nil {
		s.Files.Delete(ctx, key)
		return Admin{}, e
	}
	if w.LogoKey != "" {
		s.Files.Delete(ctx, w.LogoKey)
	}
	return s.Admin(ctx, user, org, site)
}
func (s *Service) Resolve(ctx context.Context, id, origin string) (Widget, error) {
	w, e := s.Store.Public(ctx, id)
	if e != nil || !w.Enabled || s.Config.Production && w.VerifiedAt == nil {
		return w, core.Fail("widget_unavailable", 404)
	}
	for _, o := range w.Origins {
		if o == origin {
			return w, nil
		}
	}
	return w, core.Fail("widget_origin", 403)
}
func (s *Service) Public(w Widget) PublicConfig {
	return PublicConfig{ID: w.ID, Settings: w.Settings, LogoURL: s.logoURL(w), Provider: s.AI.Config.Provider, Demo: s.AI.Config.Provider == "mock", Available: true, MaxMessageChars: 2000}
}
func (s *Service) Session(ctx context.Context, w Widget, origin string) (Session, error) {
	token := randomToken()
	v, e := s.Store.Session(ctx, w, origin, core.TokenHash(token), s.Config)
	v.Token = token
	return v, e
}
func (s *Service) History(ctx context.Context, w Widget, id, token, origin string) ([]PublicMessage, error) {
	d, e := s.AI.WebsiteHistory(ctx, w.ID, id, token, origin)
	if e != nil {
		return nil, e
	}
	return publicMessages(d), nil
}
