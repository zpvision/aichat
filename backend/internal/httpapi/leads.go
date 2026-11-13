package httpapi

import (
	"net/http"
	"site-assistant/internal/core"
	"site-assistant/internal/leads"
	"strconv"
)

func (a *Server) leadRoutes(m *http.ServeMux, s *leads.Service) {
	base := "/api/v1/organizations/{id}"
	settings := base + "/assistants/{assistantId}/lead-settings"
	m.HandleFunc("GET "+settings, a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		v, e := s.Settings(r.Context(), u.ID, r.PathValue("id"), r.PathValue("assistantId"))
		a.result(w, 200, v, e)
	}))
	m.HandleFunc("PUT "+settings, a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		var in leads.Settings
		if !a.decodeLimit(w, r, &in, 256<<10) {
			return
		}
		v, e := s.SaveSettings(r.Context(), u.ID, r.PathValue("id"), r.PathValue("assistantId"), in)
		a.result(w, 200, v, e)
	}))
	item := base + "/conversations/{conversationId}/collection"
	m.HandleFunc("GET "+item, a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		v, e := s.StaffView(r.Context(), u.ID, r.PathValue("id"), r.PathValue("conversationId"))
		a.result(w, 200, v, e)
	}))
	for _, kind := range []string{"start", "edit", "cancel", "submit"} {
		kind := kind
		m.HandleFunc("POST "+item+"/"+kind, a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
			var in leads.Input
			if !a.decodeLimit(w, r, &in, 128<<10) {
				return
			}
			v, e := s.StaffAction(r.Context(), u.ID, r.PathValue("id"), r.PathValue("conversationId"), kind, in)
			a.result(w, 200, v, e)
		}))
	}
	m.HandleFunc("GET "+base+"/leads", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
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
		v, e := s.List(r.Context(), u.ID, r.PathValue("id"), leads.Filter{Status: q.Get("status"), Site: q.Get("site"), Assistant: q.Get("assistant"), Assigned: q.Get("assigned"), From: q.Get("from"), To: q.Get("to"), Cursor: q.Get("cursor"), Limit: limit})
		a.result(w, 200, v, e)
	}))
	m.HandleFunc("GET "+base+"/leads/{leadId}", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		v, e := s.Detail(r.Context(), u.ID, r.PathValue("id"), r.PathValue("leadId"))
		a.result(w, 200, v, e)
	}))
	m.HandleFunc("PATCH "+base+"/leads/{leadId}", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		var in leads.LeadPatch
		if !a.decodeLimit(w, r, &in, 128<<10) {
			return
		}
		v, e := s.Patch(r.Context(), u.ID, r.PathValue("id"), r.PathValue("leadId"), in)
		a.result(w, 200, v, e)
	}))
	m.HandleFunc("DELETE "+base+"/leads/{leadId}", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		e := s.Delete(r.Context(), u.ID, r.PathValue("id"), r.PathValue("leadId"))
		a.result(w, 200, map[string]bool{"ok": true}, e)
	}))
}
