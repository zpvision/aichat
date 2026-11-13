package httpapi

import (
	"net"
	"net/http"
	"site-assistant/internal/core"
	"site-assistant/internal/integrations"
	"strconv"
	"strings"
	"time"
)

func (a *Server) integrationRoutes(m *http.ServeMux, s *integrations.Service) {
	base := "/api/v1/organizations/{id}/integrations"
	for _, kind := range []string{"keys", "webhooks", "deliveries"} {
		m.HandleFunc("GET "+base+"/"+kind, a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
			org := r.PathValue("id")
			if e := s.Access(r.Context(), u.ID, org); e != nil {
				a.err(w, e)
				return
			}
			if before := r.URL.Query().Get("before"); before != "" && !integrations.ValidID(before) {
				a.err(w, core.Fail("validation", 422))
				return
			}
			v, e := s.Store.Rows(r.Context(), org, kind, r.URL.Query().Get("before"))
			a.result(w, 200, v, e)
		}))
	}
	m.HandleFunc("POST "+base+"/keys", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		var in integrations.KeyInput
		if !a.decode(w, r, &in) {
			return
		}
		v, e := s.CreateKey(r.Context(), u.ID, r.PathValue("id"), in)
		a.result(w, 201, v, e)
	}))
	m.HandleFunc("POST "+base+"/webhooks", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		var in integrations.EndpointInput
		if !a.decode(w, r, &in) {
			return
		}
		v, e := s.CreateEndpoint(r.Context(), u.ID, r.PathValue("id"), in)
		a.result(w, 201, v, e)
	}))
	type callback func(http.ResponseWriter, *http.Request, string, string)
	guard := func(fn callback) http.HandlerFunc {
		return a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
			org := r.PathValue("id")
			if e := s.Access(r.Context(), u.ID, org); e != nil {
				a.err(w, e)
				return
			}
			if !integrations.ValidID(r.PathValue("itemId")) {
				a.err(w, core.Fail("validation", 422))
				return
			}
			fn(w, r, org, r.PathValue("itemId"))
		})
	}
	m.HandleFunc("DELETE "+base+"/keys/{itemId}", guard(func(w http.ResponseWriter, r *http.Request, org, id string) {
		e := s.Store.Revoke(r.Context(), org, id)
		a.result(w, 200, map[string]bool{"ok": true}, e)
	}))
	m.HandleFunc("PATCH "+base+"/webhooks/{itemId}", guard(func(w http.ResponseWriter, r *http.Request, org, id string) {
		var in struct {
			Enabled *bool `json:"enabled"`
		}
		if !a.decode(w, r, &in) {
			return
		}
		if in.Enabled == nil {
			a.err(w, core.Fail("validation", 422))
			return
		}
		e := s.Store.Toggle(r.Context(), org, id, *in.Enabled)
		a.result(w, 200, map[string]bool{"ok": true}, e)
	}))
	m.HandleFunc("POST "+base+"/webhooks/{itemId}/rotate", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		v, e := s.Rotate(r.Context(), u.ID, r.PathValue("id"), r.PathValue("itemId"))
		a.result(w, 200, v, e)
	}))
	m.HandleFunc("POST "+base+"/webhooks/{itemId}/test", guard(func(w http.ResponseWriter, r *http.Request, org, id string) {
		v, e := s.Store.Test(r.Context(), org, id)
		a.result(w, 202, map[string]string{"delivery_id": v}, e)
	}))
	m.HandleFunc("POST "+base+"/deliveries/{itemId}/retry", guard(func(w http.ResponseWriter, r *http.Request, org, id string) {
		e := s.Store.Retry(r.Context(), org, id)
		a.result(w, 202, map[string]bool{"ok": true}, e)
	}))
	m.HandleFunc("GET "+base+"/deliveries/{itemId}/attempts", guard(func(w http.ResponseWriter, r *http.Request, org, id string) {
		v, e := s.Store.Attempts(r.Context(), org, id)
		a.result(w, 200, v, e)
	}))
	external := "/api/v1/external/leads"
	auth := func(scope string, fn func(http.ResponseWriter, *http.Request, integrations.Principal)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ip, _, _ := net.SplitHostPort(r.RemoteAddr)
			a.mu.Lock()
			now := time.Now()
			for k, b := range a.attempts {
				if now.After(b.until) {
					delete(a.attempts, k)
				}
			}
			bucketKey := "external:" + ip
			b, exists := a.attempts[bucketKey]
			space := exists || len(a.attempts) < 10000
			allowed := b.count < 600 && space
			if b.count == 0 {
				b.until = now.Add(time.Minute)
			}
			b.count++
			if space {
				a.attempts[bucketKey] = b
			}
			a.mu.Unlock()
			if !allowed {
				w.Header().Set("Retry-After", "60")
				a.err(w, core.Fail("api_rate", 429))
				return
			}
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
				a.err(w, core.Fail("api_unauthorized", 401))
				return
			}
			p, e := s.Authenticate(r.Context(), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), scope)
			if e != nil {
				if f, ok := e.(*core.Fault); ok && f.Status == 429 {
					w.Header().Set("Retry-After", "60")
				}
				a.err(w, e)
				return
			}
			fn(w, r, p)
		}
	}
	m.HandleFunc("GET "+external, auth("leads:read", func(w http.ResponseWriter, r *http.Request, p integrations.Principal) {
		limit := 50
		if r.URL.Query().Has("limit") {
			var e error
			limit, e = strconv.Atoi(r.URL.Query().Get("limit"))
			if e != nil {
				a.err(w, core.Fail("validation", 422))
				return
			}
		}
		v, e := s.Store.Feed(r.Context(), p.OrganizationID, r.URL.Query().Get("cursor"), limit)
		a.result(w, 200, v, e)
	}))
	m.HandleFunc("GET "+external+"/{leadId}", auth("leads:read", func(w http.ResponseWriter, r *http.Request, p integrations.Principal) {
		v, e := s.Store.Lead(r.Context(), p.OrganizationID, r.PathValue("leadId"))
		a.result(w, 200, v, e)
	}))
	m.HandleFunc("POST "+external, auth("leads:write", func(w http.ResponseWriter, r *http.Request, p integrations.Principal) {
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			a.err(w, core.Fail("validation", 422))
			return
		}
		var in integrations.CreateInput
		if !a.decodeLimit(w, r, &in, 128<<10) {
			return
		}
		v, e := s.CreateLead(r.Context(), p, r.Header.Get("Idempotency-Key"), in)
		a.result(w, 201, v, e)
	}))
}
