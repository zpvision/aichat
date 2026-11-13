package httpapi

import (
	"net/http"
	"site-assistant/internal/billing"
	"site-assistant/internal/core"
)

func (a *Server) billingRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/organizations/{id}/billing", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		org := r.PathValue("id")
		if _, e := a.service.Access(r.Context(), u.ID, org, true, false); e != nil {
			a.err(w, e)
			return
		}
		v, e := billing.Snapshot(r.Context(), a.service.Store.DB, org)
		a.result(w, 200, v, e)
	}))
}
