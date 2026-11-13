package httpapi

import (
	"net/http"
	"site-assistant/internal/core"
	"site-assistant/internal/inbox"
	"strconv"
)

func (a *Server) inboxRoutes(m *http.ServeMux, s *inbox.Service) {
	base := "/api/v1/organizations/{id}"
	m.HandleFunc("GET "+base+"/inbox-summary", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		v, e := s.Summary(r.Context(), u.ID, r.PathValue("id"))
		a.result(w, 200, v, e)
	}))
	m.HandleFunc("GET "+base+"/conversations", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		q := r.URL.Query()
		limit := 30
		var e error
		if q.Has("limit") {
			limit, e = strconv.Atoi(q.Get("limit"))
			if e != nil {
				a.err(w, core.Fail("validation", 422))
				return
			}
		}
		v, e := s.List(r.Context(), u.ID, r.PathValue("id"), inbox.Filter{Mode: q.Get("mode"), Site: q.Get("site"), Assistant: q.Get("assistant"), Assigned: q.Get("assigned"), Query: q.Get("q"), From: q.Get("from"), To: q.Get("to"), Cursor: q.Get("cursor"), Limit: limit})
		a.result(w, 200, v, e)
	}))
	path := base + "/conversations/{conversationId}"
	for _, suffix := range []string{"", "/messages"} {
		m.HandleFunc("GET "+path+suffix, a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
			after, before, e := changeCursors(r)
			if e != nil {
				a.err(w, e)
				return
			}
			v, e := s.Page(r.Context(), u.ID, r.PathValue("id"), r.PathValue("conversationId"), after, before)
			a.result(w, 200, v, e)
		}))
	}
	for _, kind := range []string{"take", "assign", "resume-ai", "close", "notes"} {
		kind := kind
		m.HandleFunc("POST "+path+"/"+kind, a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
			var in inbox.Action
			if !a.decode(w, r, &in) {
				return
			}
			action := kind
			if action == "notes" {
				action = "note"
			}
			v, e := s.Action(r.Context(), u.ID, r.PathValue("id"), r.PathValue("conversationId"), action, in)
			a.result(w, 200, v, e)
		}))
	}
	m.HandleFunc("POST "+path+"/messages", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		var in inbox.Action
		if !a.decode(w, r, &in) {
			return
		}
		v, e := s.Reply(r.Context(), u.ID, r.PathValue("id"), r.PathValue("conversationId"), in)
		a.result(w, 200, v, e)
	}))
	m.HandleFunc("POST "+path+"/read", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		var in struct {
			Sequence int64 `json:"sequence"`
		}
		if !a.decode(w, r, &in) {
			return
		}
		e := s.Read(r.Context(), u.ID, r.PathValue("id"), r.PathValue("conversationId"), in.Sequence)
		a.result(w, 200, map[string]bool{"ok": true}, e)
	}))
	m.HandleFunc("GET "+base+"/members", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		v, e := s.Members(r.Context(), u.ID, r.PathValue("id"))
		a.result(w, 200, v, e)
	}))
	m.HandleFunc("GET "+base+"/invitations", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		v, e := s.Invitations(r.Context(), u.ID, r.PathValue("id"))
		a.result(w, 200, v, e)
	}))
	m.HandleFunc("POST "+base+"/invitations", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		var in struct {
			Role string `json:"role"`
		}
		if !a.decode(w, r, &in) {
			return
		}
		v, e := s.CreateInvitation(r.Context(), u.ID, r.PathValue("id"), in.Role)
		a.result(w, 201, v, e)
	}))
	m.HandleFunc("DELETE "+base+"/invitations/{invitationId}", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		e := s.Revoke(r.Context(), u.ID, r.PathValue("id"), r.PathValue("invitationId"))
		a.result(w, 200, map[string]bool{"ok": true}, e)
	}))
	m.HandleFunc("POST /api/v1/invitations/accept", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		var in struct {
			Token string `json:"token"`
		}
		if !a.decode(w, r, &in) {
			return
		}
		org, e := s.Accept(r.Context(), u.ID, in.Token)
		a.result(w, 200, map[string]string{"organization_id": org}, e)
	}))
}
func changeCursors(r *http.Request) (int64, int64, error) {
	var after, before int64
	for _, p := range []struct {
		k string
		v *int64
	}{{"after", &after}, {"before", &before}} {
		if raw := r.URL.Query().Get(p.k); raw != "" {
			v, e := strconv.ParseInt(raw, 10, 64)
			if e != nil || v < 0 {
				return 0, 0, core.Fail("validation", 422)
			}
			*p.v = v
		}
	}
	if after > 0 && before > 0 {
		return 0, 0, core.Fail("validation", 422)
	}
	return after, before, nil
}
