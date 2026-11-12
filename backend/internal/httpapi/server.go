package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"site-assistant/internal/config"
	"site-assistant/internal/core"
	"site-assistant/internal/knowledge"
	"strings"
	"sync"
	"time"
)

type bucket struct {
	count int
	until time.Time
}
type Server struct {
	service     *core.Service
	config      config.Config
	mu          sync.Mutex
	attempts    map[string]bucket
	authSlots   chan struct{}
	uploadSlots chan struct{}
}

func New(s *core.Service, c config.Config, modules ...*knowledge.Service) http.Handler {
	a := &Server{service: s, config: c, attempts: map[string]bucket{}, authSlots: make(chan struct{}, 4), uploadSlots: make(chan struct{}, 2)}
	m := http.NewServeMux()
	for _, k := range modules {
		a.knowledgeRoutes(m, k)
	}
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, map[string]string{"status": "ok"}) })
	m.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second*2)
		defer cancel()
		if s.Store.DB.Ping(ctx) != nil {
			a.err(w, core.Fail("not_ready", 503))
			return
		}
		respond(w, 200, map[string]string{"status": "ready"})
	})
	m.HandleFunc("GET /api/v1/config", func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]string{"brand_name": c.Brand, "region": c.Region, "default_locale": c.Locale})
	})
	m.HandleFunc("POST /api/v1/auth/register", a.register)
	m.HandleFunc("POST /api/v1/auth/login", a.login)
	m.HandleFunc("POST /api/v1/auth/logout", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		cookie, _ := r.Cookie("session")
		if e := s.Store.DeleteSession(r.Context(), core.TokenHash(cookie.Value)); e != nil {
			a.err(w, e)
			return
		}
		a.cookie(w, "", -1)
		respond(w, 200, map[string]bool{"ok": true})
	}))
	for _, path := range []string{"GET /api/v1/auth/me", "GET /api/v1/profile"} {
		m.HandleFunc(path, a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) { respond(w, 200, u) }))
	}
	m.HandleFunc("PATCH /api/v1/profile", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		var in struct {
			Name   *string `json:"name"`
			Locale *string `json:"preferred_locale"`
		}
		if !a.decode(w, r, &in) {
			return
		}
		v, e := s.Profile(r.Context(), u.ID, in.Name, in.Locale)
		a.result(w, 200, v, e)
	}))
	m.HandleFunc("GET /api/v1/organizations", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		v, e := s.Store.Organizations(r.Context(), u.ID)
		a.result(w, 200, v, e)
	}))
	m.HandleFunc("POST /api/v1/organizations", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		var in core.OrganizationInput
		if !a.decode(w, r, &in) {
			return
		}
		v, e := s.SaveOrganization(r.Context(), u.ID, "", in)
		a.result(w, 201, v, e)
	}))
	m.HandleFunc("GET /api/v1/organizations/{id}", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		v, e := s.Organization(r.Context(), u.ID, r.PathValue("id"))
		a.result(w, 200, v, e)
	}))
	m.HandleFunc("PATCH /api/v1/organizations/{id}", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		var in core.OrganizationInput
		if !a.decode(w, r, &in) {
			return
		}
		v, e := s.SaveOrganization(r.Context(), u.ID, r.PathValue("id"), in)
		a.result(w, 200, v, e)
	}))
	m.HandleFunc("GET /api/v1/organizations/{id}/assistants", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		v, e := s.Assistants(r.Context(), u.ID, r.PathValue("id"))
		a.result(w, 200, v, e)
	}))
	m.HandleFunc("GET /api/v1/organizations/{id}/assistants/{assistantId}", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		v, e := s.Assistant(r.Context(), u.ID, r.PathValue("id"), r.PathValue("assistantId"))
		a.result(w, 200, v, e)
	}))
	for _, method := range []string{"POST", "PATCH"} {
		path := method + " /api/v1/organizations/{id}/assistants"
		if method == "PATCH" {
			path += "/{assistantId}"
		}
		m.HandleFunc(path, a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
			var in core.AssistantInput
			if !a.decode(w, r, &in) {
				return
			}
			v, e := s.SaveAssistant(r.Context(), u.ID, r.PathValue("id"), r.PathValue("assistantId"), in)
			status := 200
			if r.Method == "POST" {
				status = 201
			}
			a.result(w, status, v, e)
		}))
	}
	m.HandleFunc("GET /api/v1/organizations/{id}/sites", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		v, e := s.Sites(r.Context(), u.ID, r.PathValue("id"))
		a.result(w, 200, v, e)
	}))
	m.HandleFunc("POST /api/v1/organizations/{id}/sites", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		var in core.SiteInput
		if !a.decode(w, r, &in) {
			return
		}
		v, e := s.AddSite(r.Context(), u.ID, r.PathValue("id"), in)
		a.result(w, 201, v, e)
	}))
	m.HandleFunc("DELETE /api/v1/organizations/{id}/sites/{siteId}", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		e := s.DeleteSite(r.Context(), u.ID, r.PathValue("id"), r.PathValue("siteId"))
		a.result(w, 200, map[string]bool{"ok": true}, e)
	}))
	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { a.err(w, core.Fail("not_found", 404)) })
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "same-origin")
		defer func() {
			if recover() != nil {
				slog.Error("request panic")
				a.err(w, core.Fail("internal", 500))
			}
		}()
		if r.Method != "GET" && r.Method != "HEAD" {
			multipartUpload := r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/knowledge/upload") && strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data;")
			if r.Header.Get("Origin") != c.PublicURL || r.Header.Get("X-CSRF-Protection") != "1" || (!strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") && !multipartUpload) {
				a.err(w, core.Fail("csrf", 403))
				return
			}
		}
		m.ServeHTTP(w, r)
	})
}
func (a *Server) protect(next func(http.ResponseWriter, *http.Request, core.User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, e := r.Cookie("session")
		if e != nil {
			a.err(w, core.Fail("unauthorized", 401))
			return
		}
		u, e := a.service.Store.Session(r.Context(), core.TokenHash(cookie.Value))
		if e != nil {
			a.err(w, core.Fail("unauthorized", 401))
			return
		}
		next(w, r, u)
	}
}
func (a *Server) decode(w http.ResponseWriter, r *http.Request, v any) bool {
	return a.decodeLimit(w, r, v, 32*1024)
}
func (a *Server) decodeLimit(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		a.err(w, core.Fail("validation", 422))
		return false
	}
	if d.Decode(&struct{}{}) != io.EOF {
		a.err(w, core.Fail("validation", 422))
		return false
	}
	return true
}
func (a *Server) result(w http.ResponseWriter, status int, v any, e error) {
	if e != nil {
		a.err(w, e)
		return
	}
	respond(w, status, v)
}
func (a *Server) err(w http.ResponseWriter, e error) {
	e = core.TranslateError(e)
	var f *core.Fault
	errors.As(e, &f)
	if f.Status == 500 {
		slog.Error("request failed", "code", f.Code)
	}
	respond(w, f.Status, map[string]any{"error": map[string]string{"code": f.Code}})
}
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func (a *Server) cookie(w http.ResponseWriter, token string, age int) {
	http.SetCookie(w, &http.Cookie{Name: "session", Value: token, Path: "/", MaxAge: age, HttpOnly: true, Secure: a.config.Production, SameSite: http.SameSiteLaxMode})
}
func (a *Server) allow(w http.ResponseWriter, r *http.Request) bool {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	for k, v := range a.attempts {
		if now.After(v.until) {
			delete(a.attempts, k)
		}
	}
	b := a.attempts[ip]
	if b.count >= 10 || len(a.attempts) > 10000 {
		w.Header().Set("Retry-After", "900")
		a.err(w, core.Fail("rate_limited", 429))
		return false
	}
	if b.count == 0 {
		b.until = now.Add(15 * time.Minute)
	}
	b.count++
	a.attempts[ip] = b
	return true
}
func (a *Server) credentials(w http.ResponseWriter, r *http.Request, register bool) {
	if !a.allow(w, r) {
		return
	}
	select {
	case a.authSlots <- struct{}{}:
		defer func() { <-a.authSlots }()
	default:
		a.err(w, core.Fail("rate_limited", 429))
		return
	}
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Locale   string `json:"preferred_locale"`
	}
	if !a.decode(w, r, &in) {
		return
	}
	var u core.User
	var e error
	if register {
		u, e = a.service.Register(r.Context(), in.Email, in.Password, in.Locale)
	} else {
		u, e = a.service.Login(r.Context(), in.Email, in.Password)
	}
	if e != nil {
		a.err(w, e)
		return
	}
	if register {
		respond(w, 202, map[string]bool{"ok": true})
		return
	}
	token, e := a.service.NewSession(r.Context(), u.ID)
	if e != nil {
		a.err(w, e)
		return
	}
	a.cookie(w, token, 604800)
	respond(w, 200, u)
}
func (a *Server) register(w http.ResponseWriter, r *http.Request) { a.credentials(w, r, true) }
func (a *Server) login(w http.ResponseWriter, r *http.Request)    { a.credentials(w, r, false) }
