package httpapi

import (
	"io"
	"net/http"
	"site-assistant/internal/core"
	"site-assistant/internal/knowledge"
)

func (a *Server) knowledgeRoutes(m *http.ServeMux, k *knowledge.Service) {
	base := "/api/v1/organizations/{id}/assistants/{assistantId}/knowledge"
	scope := func(r *http.Request) (string, string, string) {
		return r.PathValue("id"), r.PathValue("assistantId"), r.PathValue("documentId")
	}
	m.HandleFunc("GET "+base, a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		org, as, _ := scope(r)
		v, e := k.List(r.Context(), u.ID, org, as)
		a.result(w, 200, v, e)
	}))
	m.HandleFunc("GET "+base+"/{documentId}", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		org, as, id := scope(r)
		v, e := k.Get(r.Context(), u.ID, org, as, id)
		a.result(w, 200, v, e)
	}))
	m.HandleFunc("GET "+base+"/{documentId}/original", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		org, as, id := scope(r)
		b, e := k.Original(r.Context(), u.ID, org, as, id)
		if e != nil {
			a.err(w, e)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename=original")
		w.Write(b)
	}))
	m.HandleFunc("POST "+base+"/articles", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		var in knowledge.Input
		if !a.knowledgeDecode(w, r, &in) {
			return
		}
		org, as, _ := scope(r)
		v, e := k.Article(r.Context(), u.ID, org, as, in)
		a.result(w, 202, v, e)
	}))
	m.HandleFunc("PATCH "+base+"/{documentId}", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		var in knowledge.Input
		if !a.knowledgeDecode(w, r, &in) {
			return
		}
		org, as, id := scope(r)
		v, e := k.Update(r.Context(), u.ID, org, as, id, in)
		a.result(w, 202, v, e)
	}))
	m.HandleFunc("DELETE "+base+"/{documentId}", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		org, as, id := scope(r)
		e := k.Delete(r.Context(), u.ID, org, as, id)
		a.result(w, 200, map[string]bool{"ok": true}, e)
	}))
	m.HandleFunc("POST "+base+"/{documentId}/reindex", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		org, as, id := scope(r)
		v, e := k.Reindex(r.Context(), u.ID, org, as, id)
		a.result(w, 202, v, e)
	}))
	m.HandleFunc("POST "+base+"/import-url", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		var in struct {
			URL   string `json:"url"`
			Crawl bool   `json:"crawl"`
		}
		if !a.decode(w, r, &in) {
			return
		}
		org, as, _ := scope(r)
		v, e := k.Import(r.Context(), u.ID, org, as, in.URL, in.Crawl)
		a.result(w, 202, v, e)
	}))
	m.HandleFunc("POST "+base+"/upload", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		select {
		case a.uploadSlots <- struct{}{}:
			defer func() { <-a.uploadSlots }()
		default:
			a.err(w, core.Fail("upload_busy", 503))
			return
		}
		org, as, _ := scope(r)
		if e := k.Access(r.Context(), u.ID, org, as, true); e != nil {
			a.err(w, e)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, k.Config.MaxUpload+(64<<10))
		reader, e := r.MultipartReader()
		if e != nil {
			a.err(w, core.Fail("invalid_file", 422))
			return
		}
		part, e := reader.NextPart()
		if e != nil || part.FormName() != "file" || part.FileName() == "" {
			a.err(w, core.Fail("invalid_file", 422))
			return
		}
		b, e := io.ReadAll(io.LimitReader(part, k.Config.MaxUpload+1))
		if e != nil || int64(len(b)) > k.Config.MaxUpload {
			a.err(w, core.Fail("file_too_large", 413))
			return
		}
		name := part.FileName()
		if _, e = reader.NextPart(); e != io.EOF {
			a.err(w, core.Fail("invalid_file", 422))
			return
		}
		v, e := k.Upload(r.Context(), u.ID, org, as, name, b)
		a.result(w, 202, v, e)
	}))
	m.HandleFunc("POST "+base+"/search", a.protect(func(w http.ResponseWriter, r *http.Request, u core.User) {
		var in knowledge.SearchInput
		if !a.decode(w, r, &in) {
			return
		}
		org, as, _ := scope(r)
		v, e := k.Search(r.Context(), u.ID, org, as, in)
		a.result(w, 200, v, e)
	}))
}
func (a *Server) knowledgeDecode(w http.ResponseWriter, r *http.Request, v any) bool {
	return a.decodeLimit(w, r, v, 3<<20)
}
