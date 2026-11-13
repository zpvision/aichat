package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"site-assistant/internal/ai"
	"site-assistant/internal/core"
	"site-assistant/internal/inbox"
	"site-assistant/internal/leads"
	"site-assistant/internal/widget"
	"strings"
	"time"
)

func (a *Server) widgetRoutes(m *http.ServeMux, s *widget.Service) {
	a.inboxRoutes(m, s.Inbox)
	a.leadRoutes(m, s.Leads)
	admin := "/api/v1/organizations/{id}/sites/{siteId}/widget"
	scope := func(r *http.Request) (string, string) { return r.PathValue("id"), r.PathValue("siteId") }
	m.HandleFunc("GET "+admin, a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		org, site := scope(r)
		v, e := s.Admin(r.Context(), u.ID, org, site)
		a.result(w, 200, v, e)
	}))
	m.HandleFunc("POST "+admin, a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		org, site := scope(r)
		v, e := s.Create(r.Context(), u.ID, org, site)
		a.result(w, 201, v, e)
	}))
	m.HandleFunc("PATCH "+admin, a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		var in widget.Input
		if !a.decode(w, r, &in) {
			return
		}
		org, site := scope(r)
		v, e := s.Update(r.Context(), u.ID, org, site, in)
		a.result(w, 200, v, e)
	}))
	m.HandleFunc("POST "+admin+"/verify", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		org, site := scope(r)
		v, e := s.Verify(r.Context(), u.ID, org, site)
		a.result(w, 200, v, e)
	}))
	m.HandleFunc("POST "+admin+"/logo", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		org, site := scope(r)
		if e := s.Access(r.Context(), u.ID, org, site, true); e != nil {
			a.err(w, e)
			return
		}
		select {
		case a.uploadSlots <- struct{}{}:
			defer func() { <-a.uploadSlots }()
		default:
			a.err(w, core.Fail("upload_busy", 503))
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 300<<10)
		reader, e := r.MultipartReader()
		if e != nil {
			a.err(w, core.Fail("widget_logo", 422))
			return
		}
		part, e := reader.NextPart()
		if e != nil || part.FormName() != "file" {
			a.err(w, core.Fail("widget_logo", 422))
			return
		}
		b, e := io.ReadAll(io.LimitReader(part, (256<<10)+1))
		if e != nil {
			a.err(w, core.Fail("widget_logo", 422))
			return
		}
		v, e := s.UploadLogo(r.Context(), u.ID, org, site, b)
		a.result(w, 200, v, e)
	}))
	public := "/api/v1/public/widgets/{widgetId}"
	type callback func(http.ResponseWriter, *http.Request, widget.Widget, string)
	protect := func(next callback) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			id := r.PathValue("widgetId")
			ip, _, _ := net.SplitHostPort(r.RemoteAddr)
			if !s.Limiter.Allow("ip:"+ip, 180, time.Minute) {
				a.err(w, core.Fail("widget_rate", 429))
				return
			}
			requestOrigin := r.Header.Get("Origin")
			parent := requestOrigin
			if p := r.Header.Get("X-Widget-Origin"); p != "" {
				parent = p
				if requestOrigin == "" && r.Method == "GET" && r.Header.Get("Sec-Fetch-Site") == "same-origin" {
					requestOrigin = s.Config.PublicURL
				}
				if requestOrigin != s.Config.PublicURL {
					a.err(w, core.Fail("widget_origin", 403))
					return
				}
			}
			normalized, e := widget.NormalizeOrigin(parent)
			if e != nil || normalized != parent {
				a.err(w, core.Fail("widget_origin", 403))
				return
			}
			v, e := s.Resolve(r.Context(), id, parent)
			if e != nil {
				a.err(w, e)
				return
			}
			if requestOrigin != "" {
				w.Header().Set("Access-Control-Allow-Origin", requestOrigin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Widget-Origin")
			}
			if r.Method == "OPTIONS" {
				w.WriteHeader(204)
				return
			}
			if r.Method == "POST" && !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				a.err(w, core.Fail("validation", 422))
				return
			}
			next(w, r, v, parent)
		}
	}
	m.HandleFunc("OPTIONS "+public+"/{rest...}", protect(func(http.ResponseWriter, *http.Request, widget.Widget, string) {}))
	m.HandleFunc("GET "+public+"/config", protect(func(w http.ResponseWriter, r *http.Request, v widget.Widget, origin string) {
		conf := s.Public(v)
		enabled, e := s.Leads.Enabled(r.Context(), v.OrganizationID, v.AssistantID)
		conf.LeadEnabled = enabled
		a.result(w, 200, conf, e)
	}))
	m.HandleFunc("GET "+public+"/sessions/{sessionId}/collection", protect(func(w http.ResponseWriter, r *http.Request, v widget.Widget, origin string) {
		state, e := s.Leads.VisitorView(r.Context(), v.ID, r.PathValue("sessionId"), bearer(r), origin)
		a.result(w, 200, state, e)
	}))
	for _, kind := range []string{"start", "edit", "cancel", "submit"} {
		kind := kind
		m.HandleFunc("POST "+public+"/sessions/{sessionId}/collection/"+kind, protect(func(w http.ResponseWriter, r *http.Request, v widget.Widget, origin string) {
			var in leads.Input
			if !a.decodeLimit(w, r, &in, 128<<10) {
				return
			}
			state, e := s.Leads.VisitorAction(r.Context(), v.ID, r.PathValue("sessionId"), bearer(r), origin, kind, in)
			a.result(w, 200, state, e)
		}))
	}
	m.HandleFunc("POST "+public+"/sessions", protect(func(w http.ResponseWriter, r *http.Request, v widget.Widget, origin string) {
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		if !s.Limiter.Allow("new:"+v.ID+":"+ip, 5, time.Minute) {
			a.err(w, core.Fail("widget_rate", 429))
			return
		}
		var in struct{}
		if !a.decode(w, r, &in) {
			return
		}
		session, e := s.Session(r.Context(), v, origin)
		a.result(w, 201, session, e)
	}))
	m.HandleFunc("GET "+public+"/sessions/{sessionId}/messages", protect(func(w http.ResponseWriter, r *http.Request, v widget.Widget, origin string) {
		after, before, e := changeCursors(r)
		if e != nil {
			a.err(w, e)
			return
		}
		page, e := s.Inbox.VisitorPage(r.Context(), v.ID, r.PathValue("sessionId"), bearer(r), origin, after, before)
		a.result(w, 200, widget.PublicPage(page), e)
	}))
	m.HandleFunc("POST "+public+"/sessions/{sessionId}/request-human", protect(func(w http.ResponseWriter, r *http.Request, v widget.Widget, origin string) {
		var in inbox.Action
		if !a.decode(w, r, &in) {
			return
		}
		c, e := s.Inbox.RequestHuman(r.Context(), v.ID, r.PathValue("sessionId"), bearer(r), origin, in)
		a.result(w, 200, map[string]any{"control_mode": c.Mode, "state_version": c.Version}, e)
	}))
	m.HandleFunc("POST "+public+"/sessions/{sessionId}/cancel", protect(func(w http.ResponseWriter, r *http.Request, v widget.Widget, origin string) {
		e := s.AI.WebsiteCancel(r.Context(), v.ID, r.PathValue("sessionId"), bearer(r), origin)
		a.result(w, 200, map[string]bool{"ok": true}, e)
	}))
	m.HandleFunc("POST "+public+"/sessions/{sessionId}/messages", protect(func(w http.ResponseWriter, r *http.Request, v widget.Widget, origin string) {
		token := bearer(r)
		if !s.Limiter.Allow("message:"+v.ID+":"+core.TokenHash(token), 10, time.Minute) {
			a.err(w, core.Fail("widget_rate", 429))
			return
		}
		var in ai.MessageInput
		if !a.decode(w, r, &in) {
			return
		}
		sender := newEventSender(w)
		emit := func(ev ai.Event) error {
			switch ev.Type {
			case "start":
				return sender(ai.Event{Type: "start", Data: map[string]string{"request_id": in.RequestID}})
			case "delta":
				return sender(ev)
			}
			return nil
		}
		result, e := s.AI.WebsiteMessage(r.Context(), v.ID, r.PathValue("sessionId"), token, origin, in, true, emit)
		if e != nil {
			f := ai.Error(e).(*core.Fault)
			if f.Code == "plan_limit_exceeded" || f.Code == "subscription_inactive" {
				f = &core.Fault{Code: "ai_unavailable", Status: 503}
			}
			if w.Header().Get("Content-Type") == "text/event-stream; charset=utf-8" {
				sender(ai.Event{Type: "error", Data: map[string]string{"code": f.Code}})
			} else {
				a.err(w, f)
			}
			return
		}
		messages := []widget.PublicMessage{widget.FromMessage(result.UserMessage, in.RequestID)}
		if result.AssistantMessage.ID != "" {
			messages = append(messages, widget.FromMessage(result.AssistantMessage, in.RequestID))
		}
		complete := func() error {
			return sender(ai.Event{Type: "complete", Data: map[string]any{"request_id": in.RequestID, "messages": messages}})
		}
		if !result.Manual {
			scope, err := s.AI.Store.WebsiteScope(r.Context(), v.ID, r.PathValue("sessionId"), token, origin)
			if err == nil {
				err = s.AI.Store.Deliver(r.Context(), scope, result.Execution.ID, true, complete)
			}
			if err != nil {
				sender(ai.Event{Type: "error", Data: map[string]string{"code": "ai_canceled"}})
			}
		} else {
			complete()
		}
	}))
	m.HandleFunc("GET "+public+"/logo", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Store.Public(r.Context(), r.PathValue("widgetId"))
		if e != nil || v.LogoKey == "" {
			a.err(w, core.Fail("not_found", 404))
			return
		}
		b, e := s.Files.Read(r.Context(), v.LogoKey, 2<<20)
		if e != nil {
			a.err(w, core.Fail("not_found", 404))
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public,max-age=300")
		w.Write(b)
	})
	m.Handle("GET /widget/assets/", http.StripPrefix("/widget/assets/", http.FileServer(http.Dir(filepath.Join(s.Config.DistDir, "assets")))))
	for _, name := range []string{"loader.js", "loader.css"} {
		name := name
		m.HandleFunc("GET /widget/"+name, func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, filepath.Join(s.Config.DistDir, name))
		})
	}
	m.HandleFunc("GET /widget/{widgetId}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("widgetId")
		parent := r.URL.Query().Get("parent_origin")
		ancestors := s.Config.AppURL
		if id != "preview" {
			v, e := s.Resolve(r.Context(), id, parent)
			if e != nil {
				a.err(w, e)
				return
			}
			ancestors = strings.Join(v.Origins, " ")
		} else if parent != s.Config.AppURL {
			a.err(w, core.Fail("widget_origin", 403))
			return
		}
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors "+ancestors)
		b, e := os.ReadFile(filepath.Join(s.Config.DistDir, "index.html"))
		if e != nil {
			a.err(w, core.Fail("widget_assets", 503))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(b)
	})
}
func bearer(r *http.Request) string {
	v := r.Header.Get("Authorization")
	if !strings.HasPrefix(v, "Bearer ") {
		return ""
	}
	v = strings.TrimPrefix(v, "Bearer ")
	if len(v) != 43 {
		return ""
	}
	return v
}
func newEventSender(w http.ResponseWriter) func(ai.Event) error {
	controller := http.NewResponseController(w)
	return func(ev ai.Event) error {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("X-Accel-Buffering", "no")
		if e := controller.SetWriteDeadline(time.Now().Add(10 * time.Second)); e != nil && !errors.Is(e, http.ErrNotSupported) {
			return e
		}
		b, e := json.Marshal(ev.Data)
		if e != nil {
			return e
		}
		if _, e = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, b); e != nil {
			return e
		}
		return controller.Flush()
	}
}
