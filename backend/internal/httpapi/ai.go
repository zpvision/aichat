package httpapi

import (
	"net/http"
	"site-assistant/internal/ai"
	"site-assistant/internal/core"
	"strconv"
)

func (a *Server) aiRoutes(m *http.ServeMux, s *ai.Service) {
	base := "/api/v1/organizations/{id}/assistants/{assistantId}"
	scope := func(r *http.Request) ai.Scope {
		return ai.Scope{OrganizationID: r.PathValue("id"), AssistantID: r.PathValue("assistantId"), ConversationID: r.PathValue("conversationId")}
	}
	m.HandleFunc("GET "+base+"/ai/settings", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		v, e := s.Settings(r.Context(), u.ID, scope(r))
		a.result(w, 200, v, e)
	}))
	m.HandleFunc("PATCH "+base+"/ai/settings", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		var in ai.SettingsPatch
		if !a.decode(w, r, &in) {
			return
		}
		v, e := s.UpdateSettings(r.Context(), u.ID, scope(r), in)
		a.result(w, 200, v, e)
	}))
	path := base + "/preview/conversations"
	m.HandleFunc("GET "+path, a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		v, e := s.List(r.Context(), u.ID, scope(r))
		a.result(w, 200, v, e)
	}))
	m.HandleFunc("POST "+path, a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		v, e := s.Create(r.Context(), u.ID, scope(r))
		a.result(w, 201, v, e)
	}))
	m.HandleFunc("GET "+path+"/{conversationId}", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		before := int64(0)
		var e error
		if raw := r.URL.Query().Get("before"); raw != "" {
			before, e = strconv.ParseInt(raw, 10, 64)
			if e != nil || before < 1 {
				a.err(w, core.Fail("validation", 422))
				return
			}
		}
		v, e := s.Detail(r.Context(), u.ID, scope(r), before)
		a.result(w, 200, v, e)
	}))
	m.HandleFunc("DELETE "+path+"/{conversationId}", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		e := s.Delete(r.Context(), u.ID, scope(r))
		a.result(w, 200, map[string]bool{"ok": true}, e)
	}))
	m.HandleFunc("POST "+path+"/{conversationId}/cancel", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		e := s.Cancel(r.Context(), u.ID, scope(r))
		a.result(w, 200, map[string]bool{"ok": true}, e)
	}))
	m.HandleFunc("POST "+path+"/{conversationId}/messages", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		var in ai.MessageInput
		if !a.decode(w, r, &in) {
			return
		}
		v, e := s.Message(r.Context(), u.ID, scope(r), in, false, nil)
		a.result(w, 200, v, ai.ErrorIfPresent(e))
	}))
	m.HandleFunc("POST "+path+"/{conversationId}/messages/stream", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		var in ai.MessageInput
		if !a.decode(w, r, &in) {
			return
		}
		started := false
		sender := newEventSender(w)
		send := func(event ai.Event) error { started = true; return sender(event) }
		v, e := s.Message(r.Context(), u.ID, scope(r), in, true, send)
		if e != nil {
			e = ai.Error(e)
			if !started {
				a.err(w, e)
				return
			}
			f := e.(*core.Fault)
			send(ai.Event{Type: "error", Data: map[string]string{"code": f.Code}})
			return
		}
		send(ai.Event{Type: "complete", Data: v})
	}))
}
