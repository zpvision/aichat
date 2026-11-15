package httpapi

import (
	"net/http"
	"site-assistant/internal/analytics"
	"site-assistant/internal/core"
	"time"
)

func (a *Server) analyticsRoutes(m *http.ServeMux) {
	s := &analytics.Service{Auth: a.service, Store: &analytics.Store{DB: a.service.Store.DB}}
	m.HandleFunc("GET /api/v1/organizations/{id}/analytics", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		f, e := analytics.Parse(r.URL.Query(), time.Now())
		if e != nil {
			a.err(w, e)
			return
		}
		data, e := s.Report(r.Context(), u.ID, r.PathValue("id"), f)
		a.result(w, 200, data, e)
	}))
}
