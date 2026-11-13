package knowledge

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"site-assistant/internal/core"
	"time"
)

func embeddingClient(c Config) *http.Client {
	return &http.Client{Timeout: c.Timeout, CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
}
func (s *Service) ProcessOne(parent context.Context) error {
	j, e := s.Store.Claim(parent)
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(parent, 3*time.Minute)
	defer cancel()
	e = s.process(ctx, j)
	if e == nil {
		return nil
	}
	if parent.Err() != nil {
		return parent.Err()
	}
	code := "processing_failed"
	if f, ok := core.TranslateError(e).(*core.Fault); ok && f.Code == "plan_limit_exceeded" {
		e = f
	}
	retry := true
	var f *core.Fault
	if errors.As(e, &f) {
		code = f.Code
		retry = f.Status >= 500
	}
	if errors.Is(e, pgx.ErrNoRows) {
		code = "processing_interrupted"
		retry = false
	}
	finish, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.Store.Failure(finish, j, code, retry)
}
func (s *Service) process(ctx context.Context, j Job) error {
	v, e := s.Store.Get(ctx, j.OrganizationID, j.AssistantID, j.SourceID)
	if e != nil {
		return e
	}
	if v.Revision != j.Revision {
		return s.Store.Failure(ctx, j, "superseded", false)
	}
	pages := []Page{{Text: v.Content}}
	if v.Kind == "document" {
		b, e := s.Files.Read(ctx, v.FileKey, s.Config.MaxUpload)
		if e != nil {
			return e
		}
		extract, cancel := context.WithTimeout(ctx, 30*time.Second)
		pages, e = Extract(extract, b, v.Format)
		cancel()
		if e != nil {
			return e
		}
	}
	if v.Kind == "web" && v.Content == "" {
		p, e := s.Importer.Page(ctx, v.URL)
		if e != nil {
			return e
		}
		v.Title = p.Title
		if validLang(p.Language) {
			v.Language = p.Language
		}
		pages = []Page{{Text: p.Text}}
		if v.Crawl {
			if e = s.crawl(ctx, j, &v, p); e != nil {
				return e
			}
		}
	}
	chunks := Split(pages, s.Config.ChunkChars, s.Config.Overlap)
	if e = s.Store.Extracted(ctx, j, v, pages); e != nil {
		return e
	}
	if len(chunks) == 0 {
		return core.Fail("empty_document", 422)
	}
	if len(chunks) > 1500 {
		return core.Fail("file_too_large", 413)
	}
	var combined Embeddings
	for start := 0; start < len(chunks); start += s.Config.Batch {
		end := min(start+s.Config.Batch, len(chunks))
		input := []string{}
		for _, c := range chunks[start:end] {
			input = append(input, c.Text)
		}
		emb, e := s.Provider.Embed(ctx, input)
		if e != nil {
			return e
		}
		if e = ValidateEmbeddings(emb, len(input)); e != nil {
			return e
		}
		if start == 0 {
			combined = emb
		} else {
			if combined.Model != emb.Model || combined.Version != emb.Version || combined.Dimensions != emb.Dimensions {
				return core.Fail("embedding_changed", 503)
			}
			combined.Vectors = append(combined.Vectors, emb.Vectors...)
		}
	}
	return s.Store.Complete(ctx, j, v, pages, chunks, combined)
}
func (s *Service) crawl(ctx context.Context, j Job, root *Source, first WebPage) error {
	type item struct {
		url   string
		depth int
	}
	queue := []item{}
	for _, l := range first.Links {
		if len(queue) >= 1000 {
			break
		}
		queue = append(queue, item{l, 1})
	}
	seen := map[string]bool{root.URL: true}
	count := 1
	attempts := 1
	s.Store.ImportProgress(ctx, j, count)
	for len(queue) > 0 && attempts < s.Config.MaxPages {
		it := queue[0]
		queue = queue[1:]
		if it.depth > s.Config.Depth {
			continue
		}
		u, e := NormalizeURL(it.url)
		if e != nil || seen[u.String()] {
			continue
		}
		seen[u.String()] = true
		attempts++
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
		p, e := s.Importer.Page(ctx, u.String())
		if e != nil {
			root.ErrorCode = "import_partial"
			continue
		}
		count++
		v, e := s.Store.ByURL(ctx, root.OrganizationID, root.AssistantID, u.String())
		create := errors.Is(e, pgx.ErrNoRows)
		if e != nil && !create {
			return e
		}
		if create {
			v = Source{ID: core.ID(), OrganizationID: root.OrganizationID, AssistantID: root.AssistantID, Kind: "web", URL: u.String(), Enabled: true}
		}
		h := checksum([]byte(p.Text))
		if create || v.Checksum != h {
			v.Title = p.Title
			v.Content = p.Text
			v.Checksum = h
			if validLang(p.Language) {
				v.Language = p.Language
			}
			if _, e = s.Store.Put(ctx, v, create); e != nil {
				return e
			}
		}
		s.Store.ImportProgress(ctx, j, count)
		for _, l := range p.Links {
			if len(queue) < 1000 {
				queue = append(queue, item{l, it.depth + 1})
			}
		}
	}
	return nil
}
