package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/jackc/pgx/v5"
	"log/slog"
	"path/filepath"
	"site-assistant/internal/billing"
	"site-assistant/internal/core"
	"sort"
	"strings"
	"time"
)

type Service struct {
	Auth     *core.Service
	Store    *Store
	Files    FileStorage
	Provider EmbeddingProvider
	Importer *Importer
	Config   Config
}

func New(auth *core.Service, c Config) *Service {
	return &Service{Auth: auth, Store: &Store{DB: auth.Store.DB, Config: c}, Files: LocalStorage{Root: c.FileRoot}, Config: c, Importer: NewImporter(), Provider: &Ollama{BaseURL: c.BaseURL, Model: c.Model, Client: embeddingClient(c)}}
}
func (s *Service) Access(ctx context.Context, user, org, assistant string, write bool) error {
	if _, e := s.Auth.Access(ctx, user, org, write, false); e != nil {
		return e
	}
	_, e := s.Auth.Assistant(ctx, user, org, assistant)
	return e
}
func (s *Service) List(ctx context.Context, user, org, a string) ([]Source, error) {
	if e := s.Access(ctx, user, org, a, false); e != nil {
		return nil, e
	}
	return s.Store.List(ctx, org, a)
}
func (s *Service) Get(ctx context.Context, user, org, a, id string) (Source, error) {
	if e := s.Access(ctx, user, org, a, false); e != nil {
		return Source{}, e
	}
	return s.Store.Get(ctx, org, a, id)
}
func checksum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func validate(v Source) error {
	if strings.TrimSpace(v.Title) == "" || len(v.Title) > 240 || len(v.Content) > maxText || strings.ContainsRune(v.Content, 0) || (!validLang(v.Language)) {
		return core.Fail("validation", 422)
	}
	if v.Kind == "article" && strings.TrimSpace(v.Content) == "" {
		return core.Fail("empty_document", 422)
	}
	return nil
}
func validLang(v string) bool { return v == "" || v == "ru" || v == "en" || v == "cs" || v == "de" }
func (s *Service) Article(ctx context.Context, user, org, a string, in Input) (Source, error) {
	if e := s.Access(ctx, user, org, a, true); e != nil {
		return Source{}, e
	}
	v := Source{ID: core.ID(), OrganizationID: org, AssistantID: a, Kind: "article", Enabled: true}
	apply(&v, in)
	v.Checksum = checksum([]byte(v.Content))
	if e := validate(v); e != nil {
		return v, e
	}
	return s.Store.Put(ctx, v, true)
}
func apply(v *Source, in Input) {
	if in.Title != nil {
		v.Title = strings.TrimSpace(*in.Title)
	}
	if in.Content != nil {
		v.Content = *in.Content
	}
	if in.Language != nil {
		v.Language = *in.Language
	}
	if in.Enabled != nil {
		v.Enabled = *in.Enabled
	}
}
func (s *Service) Update(ctx context.Context, user, org, a, id string, in Input) (Source, error) {
	if e := s.Access(ctx, user, org, a, true); e != nil {
		return Source{}, e
	}
	v, e := s.Store.Get(ctx, org, a, id)
	if e != nil {
		return v, e
	}
	if in.Content != nil && v.Kind != "article" {
		return v, core.Fail("validation", 422)
	}
	apply(&v, in)
	if v.Kind == "article" {
		v.Checksum = checksum([]byte(v.Content))
	}
	if e = validate(v); e != nil {
		return v, e
	}
	return s.Store.Put(ctx, v, false)
}
func (s *Service) Upload(ctx context.Context, user, org, a, name string, b []byte) (Source, error) {
	if e := s.Access(ctx, user, org, a, true); e != nil {
		return Source{}, e
	}
	if int64(len(b)) > s.Config.MaxUpload {
		return Source{}, core.Fail("file_too_large", 413)
	}
	format := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	if format != "pdf" && format != "docx" && format != "txt" && format != "md" {
		return Source{}, core.Fail("unsupported_file", 422)
	}
	if len(b) == 0 {
		return Source{}, core.Fail("empty_document", 422)
	}
	// Cheap format validation here; full extraction always runs in the background.
	if (format == "pdf" && !strings.HasPrefix(string(b), "%PDF-")) || (format == "docx" && !strings.HasPrefix(string(b), "PK\x03\x04")) {
		return Source{}, core.Fail("invalid_file", 422)
	}
	id := core.ID()
	if e := billing.ReserveFile(ctx, s.Store.DB, org, id, int64(len(b))); e != nil {
		return Source{}, e
	}
	cleanup := func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		abandon, e := billing.AbandonFile(c, s.Store.DB, org, id)
		if e == nil && abandon && s.Files.Delete(c, id) == nil {
			billing.ConfirmDeleted(c, s.Store.DB, org, id)
		}
	}
	if e := s.Files.PutAt(ctx, id, b); e != nil {
		cleanup()
		return Source{}, e
	}
	v := Source{ID: id, OrganizationID: org, AssistantID: a, Kind: "document", Title: filepath.Base(strings.ReplaceAll(name, "\\", "/")), Enabled: true, Format: format, FileKey: id, Checksum: checksum(b)}
	e := validate(v)
	if e == nil {
		v, e = s.Store.Put(ctx, v, true)
	}
	if e != nil {
		cleanup()
	}
	return v, e
}
func (s *Service) Import(ctx context.Context, user, org, a, raw string, crawl bool) (Source, error) {
	if e := s.Access(ctx, user, org, a, true); e != nil {
		return Source{}, e
	}
	u, e := NormalizeURL(raw)
	if e != nil {
		return Source{}, e
	}
	v, e := s.Store.ByURL(ctx, org, a, u.String())
	if e == nil {
		v.Content = ""
		return s.Store.Put(ctx, v, false)
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return v, e
	}
	v = Source{ID: core.ID(), OrganizationID: org, AssistantID: a, Kind: "web", Title: u.Hostname(), URL: u.String(), Enabled: true, Crawl: crawl}
	return s.Store.Put(ctx, v, true)
}
func (s *Service) Reindex(ctx context.Context, user, org, a, id string) (Source, error) {
	if e := s.Access(ctx, user, org, a, true); e != nil {
		return Source{}, e
	}
	v, e := s.Store.Get(ctx, org, a, id)
	if e != nil {
		return v, e
	}
	if v.Kind == "web" {
		v.Content = ""
	}
	return s.Store.Put(ctx, v, false)
}
func (s *Service) Delete(ctx context.Context, user, org, a, id string) error {
	if e := s.Access(ctx, user, org, a, true); e != nil {
		return e
	}
	key, e := s.Store.Delete(ctx, org, a, id)
	if e != nil {
		return e
	}
	if key != "" {
		if e = s.Files.Delete(ctx, key); e != nil {
			return e
		}
		return billing.ConfirmDeleted(ctx, s.Store.DB, org, key)
	}
	return nil
}
func (s *Service) Search(ctx context.Context, user, org, a string, in SearchInput) ([]Hit, error) {
	if e := s.Access(ctx, user, org, a, false); e != nil {
		return nil, e
	}
	return s.search(ctx, org, a, in)
}

// Retrieve is for trusted internal channels after their own authorization; never bind it directly to HTTP IDs.
func (s *Service) Retrieve(ctx context.Context, org, a string, in SearchInput) ([]Hit, error) {
	return s.search(ctx, org, a, in)
}
func (s *Service) search(ctx context.Context, org, a string, in SearchInput) ([]Hit, error) {
	if strings.TrimSpace(in.Query) == "" || len(in.Query) > 2000 || (in.Mode != "" && in.Mode != "semantic" && in.Mode != "hybrid") {
		return nil, core.Fail("validation", 422)
	}
	ctx, cancel := context.WithTimeout(ctx, s.Config.Timeout)
	defer cancel()
	v, e := s.Provider.Embed(ctx, []string{in.Query})
	if e != nil {
		return nil, e
	}
	if e = ValidateEmbeddings(v, 1); e != nil {
		return nil, e
	}
	return s.Store.Candidates(ctx, org, a, in.Query, in.Mode, v, s.Config.MinSimilarity)
}
func Fuse(a, b []Hit) []Hit {
	m := map[string]Hit{}
	for _, list := range [][]Hit{a, b} {
		for i, h := range list {
			v, ok := m[h.ID]
			if !ok {
				v = h
				v.Score = 0
			}
			v.Score += 1 / float64(60+i+1)
			m[h.ID] = v
		}
	}
	out := []Hit{}
	for _, h := range m {
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].ID < out[j].ID
		}
		return out[i].Score > out[j].Score
	})
	return out[:min(5, len(out))]
}
func (s *Service) Run(ctx context.Context) {
	if s.ReconcileStorage(ctx) != nil {
		slog.Error("knowledge storage accounting incomplete")
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if e := s.cleanupStorage(ctx); e != nil && !errors.Is(e, pgx.ErrNoRows) && ctx.Err() == nil {
				slog.Error("knowledge storage cleanup failed")
			}
			if e := s.ProcessOne(ctx); e != nil && !errors.Is(e, pgx.ErrNoRows) && ctx.Err() == nil {
				slog.Error("knowledge worker operation failed")
			}
		}
	}
}
