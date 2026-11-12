package knowledge

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	foundation "site-assistant"
	"site-assistant/internal/core"
	"site-assistant/internal/database"
	"strings"
	"testing"
	"time"
)

func TestChunking(t *testing.T) {
	text := strings.Repeat("Надёжные služby für international companies.\n\n", 200)
	chunks := Split([]Page{{Text: text, Number: 7}}, 2200, 260)
	if len(chunks) < 3 {
		t.Fatal("not split")
	}
	for i, c := range chunks {
		if c.Page != 7 || c.Position != i || len([]rune(c.Text)) > 2250 {
			t.Fatal("chunk metadata/size")
		}
	}
	if strings.Join(strings.Fields(chunks[0].Text), " ") == "" {
		t.Fatal("empty")
	}
}
func docx(content string) []byte {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for name, data := range map[string]string{"[Content_Types].xml": "<Types/>", "word/document.xml": content} {
		f, _ := w.Create(name)
		f.Write([]byte(data))
	}
	w.Close()
	return b.Bytes()
}
func TestExtraction(t *testing.T) {
	ctx := context.Background()
	for _, format := range []string{"txt", "md"} {
		p, e := Extract(ctx, []byte("Привет. Čeština. Grüße. English."), format)
		if e != nil || !strings.Contains(p[0].Text, "Čeština") {
			t.Fatal(format, e)
		}
	}
	b := docx(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>First</w:t></w:r></w:p><w:tbl><w:tr><w:tc><w:p><w:r><w:t>Table</w:t></w:r></w:p></w:tc></w:tr></w:tbl></w:body></w:document>`)
	p, e := Extract(ctx, b, "docx")
	if e != nil || strings.Index(p[0].Text, "First") > strings.Index(p[0].Text, "Table") {
		t.Fatal(e)
	}
	for _, test := range []struct {
		data   []byte
		format string
	}{{nil, "txt"}, {[]byte{255}, "txt"}, {[]byte("%PDF-bad"), "txt"}, {[]byte("bad"), "docx"}, {docx("<bad>"), "docx"}, {[]byte("text"), "pdf"}, {[]byte(""), "md"}} {
		if _, e = Extract(ctx, test.data, test.format); e == nil {
			t.Fatal("accepted invalid file", test.format)
		}
	}
}
func TestSSRF(t *testing.T) {
	for _, raw := range []string{"http://localhost/a", "http://127.0.0.1", "http://169.254.169.254/latest", "http://10.1.2.3", "http://172.17.0.1", "http://[::1]", "http://[::ffff:127.0.0.1]", "http://100.100.100.200", "file:///etc/passwd", "http://example.com:8080", "http://user:pass@example.com"} {
		if _, e := NormalizeURL(raw); e == nil {
			t.Fatal("unsafe URL", raw)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, e := NewImporter().Page(ctx, "http://localtest.me")
	if e == nil {
		t.Fatal("private DNS allowed")
	}
}
func TestImporter(t *testing.T) {
	u, _ := url.Parse("https://example.com/a")
	p, e := ParseHTML([]byte(`<html lang="de"><head><title>Example</title></head><body><nav>Menu</nav><main><h1>Heading</h1><p>Useful text</p><script>alert(1)</script><a href="/b">Next</a></main><footer>Footer</footer></body></html>`), u)
	if e != nil || strings.Contains(p.Text, "alert") || strings.Contains(p.Text, "Menu") || p.Language != "de" || len(p.Links) != 1 {
		t.Fatal(p, e)
	}
}
func TestEmbeddingValidation(t *testing.T) {
	v := Embeddings{Vectors: [][]float32{{1, 0}}, Dimensions: 2, Model: "mock", Version: "test"}
	if e := ValidateEmbeddings(v, 1); e != nil {
		t.Fatal(e)
	}
	v.Vectors[0][0] = float32(math.NaN())
	if ValidateEmbeddings(v, 1) == nil {
		t.Fatal("NaN accepted")
	}
}
func TestArchiveExpansionLimit(t *testing.T) {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	f, _ := w.Create("word/document.xml")
	f.Write(bytes.Repeat([]byte("x"), 21<<20))
	f, _ = w.Create("[Content_Types].xml")
	f.Write([]byte("<Types/>"))
	w.Close()
	if _, e := Extract(context.Background(), b.Bytes(), "docx"); e == nil {
		t.Fatal("oversize DOCX expansion allowed")
	}
}

type mockEmbedding struct {
	fail    bool
	version string
}

func (m *mockEmbedding) Embed(ctx context.Context, in []string) (Embeddings, error) {
	if m.fail {
		return Embeddings{}, core.Fail("embedding_unavailable", 503)
	}
	out := Embeddings{Model: "mock", Version: m.version, Dimensions: 3}
	for range in {
		out.Vectors = append(out.Vectors, []float32{1, 0, 0})
	}
	return out, nil
}
func setup(t *testing.T) (*Service, string, string, string) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	u, e := url.Parse(dsn)
	if e != nil || !strings.HasSuffix(u.Path, "_test") {
		t.Fatal("test DB must end _test")
	}
	ctx := context.Background()
	admin, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	schema := "knowledge_" + strings.ReplaceAll(core.ID(), "-", "")
	if _, e = admin.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal(e)
	}
	cfg, _ := pgxpool.ParseConfig(dsn)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { pool.Close(); admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); admin.Close() })
	for i := 0; i < 2; i++ {
		if e = database.Migrate(ctx, pool, foundation.Migrations); e != nil {
			t.Fatal(e)
		}
	}
	auth := core.NewService(&core.Store{DB: pool})
	user, e := auth.Register(ctx, "knowledge@example.com", "test-password-12345", "en")
	if e != nil {
		t.Fatal(e)
	}
	name, country := "Company", "CZ"
	org, e := auth.SaveOrganization(ctx, user.ID, "", core.OrganizationInput{Name: &name, Country: &country})
	if e != nil {
		t.Fatal(e)
	}
	name = "Helper"
	assistant, e := auth.SaveAssistant(ctx, user.ID, org.ID, "", core.AssistantInput{Name: &name})
	if e != nil {
		t.Fatal(e)
	}
	c, e := LoadConfig()
	if e != nil {
		t.Fatal(e)
	}
	c.FileRoot = t.TempDir()
	s := New(auth, c)
	s.Provider = &mockEmbedding{version: "v1"}
	return s, user.ID, org.ID, assistant.ID
}
func str(s string) *string { return &s }
func TestIntegrationKnowledge(t *testing.T) {
	s, user, org, a := setup(t)
	ctx := context.Background()
	v, e := s.Article(ctx, user, org, a, Input{Title: str("Knowledge"), Content: str(strings.Repeat("Tariff Aurora costs 450 EUR. ", 150)), Language: str("en")})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.ProcessOne(ctx); e != nil {
		t.Fatal(e)
	}
	v, e = s.Get(ctx, user, org, a, v.ID)
	if e != nil || v.Status != "ready" || v.ActiveRevision == nil {
		t.Fatal(v, e)
	}
	hits, e := s.Search(ctx, user, org, a, SearchInput{Query: "Aurora"})
	if e != nil || len(hits) == 0 || hits[0].SourceID != v.ID {
		t.Fatal(hits, e)
	}
	old := *v.ActiveRevision
	v, e = s.Update(ctx, user, org, a, v.ID, Input{Content: str("Aurora costs 500 EUR now.")})
	if e != nil {
		t.Fatal(e)
	}
	m := s.Provider.(*mockEmbedding)
	m.fail = true
	s.ProcessOne(ctx)
	v, _ = s.Store.Get(ctx, org, a, v.ID)
	if v.ActiveRevision == nil || *v.ActiveRevision != old || v.ErrorCode != "embedding_unavailable" {
		t.Fatal("old index lost", v)
	}
	m.fail = false
	s.Store.DB.Exec(ctx, "UPDATE knowledge_jobs SET available_at=now() WHERE source_id=$1", v.ID)
	s.ProcessOne(ctx)
	v, _ = s.Store.Get(ctx, org, a, v.ID)
	if *v.ActiveRevision != v.Revision {
		t.Fatal("new index not activated")
	}
	var count int
	s.Store.DB.QueryRow(ctx, "SELECT count(DISTINCT revision) FROM knowledge_chunks WHERE source_id=$1", v.ID).Scan(&count)
	if count != 1 {
		t.Fatal("mixed revisions")
	}
	other, e := s.Auth.SaveAssistant(ctx, user, org, "", core.AssistantInput{Name: str("Other")})
	if e != nil {
		t.Fatal(e)
	}
	hits, e = s.Search(ctx, user, org, other.ID, SearchInput{Query: "Aurora"})
	if e != nil || len(hits) != 0 {
		t.Fatal("assistant leak", hits, e)
	}
	if _, e = s.Get(ctx, user, org, other.ID, v.ID); e == nil {
		t.Fatal("cross assistant source")
	}
	if _, e = s.Article(ctx, user, core.ID(), a, Input{Title: str("Bad"), Content: str("Bad")}); e == nil {
		t.Fatal("cross tenant source")
	}
	operator, e := s.Auth.Register(ctx, "operator@example.com", "test-password-12345", "en")
	if e != nil {
		t.Fatal(e)
	}
	s.Store.DB.Exec(ctx, "INSERT INTO organization_members(organization_id,user_id,role) VALUES($1,$2,'operator')", org, operator.ID)
	if _, e = s.Article(ctx, operator.ID, org, a, Input{Title: str("Bad"), Content: str("Bad")}); e == nil {
		t.Fatal("operator wrote")
	}
	if _, e = s.List(ctx, operator.ID, org, a); e != nil {
		t.Fatal("operator read", e)
	}
	data := []byte("Unicode: Česká účetní služba. Русский текст.")
	for _, file := range []struct {
		name string
		data []byte
	}{{"terms.md", []byte("# Terms\n\nČeský a русский текст.")}, {"table.docx", docx(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>Český a русский текст</w:t></w:r></w:p></w:body></w:document>`)}} {
		f, err := s.Upload(ctx, user, org, a, file.name, file.data)
		if err != nil {
			t.Fatal(err)
		}
		s.ProcessOne(ctx)
		f, _ = s.Get(ctx, user, org, a, f.ID)
		if f.Status != "ready" || !strings.Contains(f.Content, "русский") {
			t.Fatal(f)
		}
	}
	if _, err := s.Upload(ctx, user, org, a, "fake.pdf", []byte("not PDF")); err == nil {
		t.Fatal("format spoofing accepted")
	}
	if _, err := s.Upload(ctx, user, org, a, "empty.txt", nil); err == nil {
		t.Fatal("empty upload accepted")
	}
	upload, e := s.Upload(ctx, user, org, a, "../../note.txt", data)
	if e != nil {
		t.Fatal(e)
	}
	if upload.Title != "note.txt" {
		t.Fatal("filename")
	}
	if _, e = s.Upload(ctx, user, org, a, "copy.txt", data); e == nil {
		t.Fatal("duplicate upload allowed")
	}
	s.ProcessOne(ctx)
	upload, _ = s.Get(ctx, user, org, a, upload.ID)
	if upload.Status != "ready" {
		t.Fatal(upload)
	}
	m.version = "v2"
	hits, e = s.Search(ctx, user, org, a, SearchInput{Query: "Aurora"})
	if e != nil || len(hits) != 0 {
		t.Fatal("mixed model versions")
	}
	m.version = "v1"
	disabled := false
	s.Update(ctx, user, org, a, v.ID, Input{Enabled: &disabled})
	hits, e = s.Search(ctx, user, org, a, SearchInput{Query: "Aurora", Mode: "hybrid"})
	if e != nil {
		t.Fatal(e)
	}
	for _, h := range hits {
		if h.SourceID == v.ID {
			t.Fatal("disabled in search")
		}
	}
	if e = s.Delete(ctx, user, org, a, v.ID); e != nil {
		t.Fatal(e)
	}
	s.Store.DB.QueryRow(ctx, "SELECT count(*) FROM knowledge_chunks WHERE source_id=$1", v.ID).Scan(&count)
	if count != 0 {
		t.Fatal("chunks survived deletion")
	}
	// Recover a lease abandoned by an interrupted process.
	fresh, e := s.Article(ctx, user, org, a, Input{Title: str("Recover"), Content: str("Recovery content")})
	if e != nil {
		t.Fatal(e)
	}
	j, e := s.Store.Claim(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if j.SourceID != fresh.ID {
		t.Fatal("unexpected job")
	}
	s.Store.DB.Exec(ctx, "UPDATE knowledge_jobs SET lease_until=now()-interval '1 minute' WHERE id=$1", j.ID)
	s.ProcessOne(ctx)
	fresh, _ = s.Get(ctx, user, org, a, fresh.ID)
	if fresh.Status != "ready" {
		t.Fatal("lease not recovered", fresh)
	}
}
func TestOllamaValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			fmt.Fprint(w, `{"models":[{"name":"test","digest":"v1"}]}`)
			return
		}
		fmt.Fprint(w, `{"embeddings":[[1,0],[1]]}`)
	}))
	defer server.Close()
	o := Ollama{BaseURL: server.URL, Model: "test", Client: server.Client()}
	if _, e := o.Embed(context.Background(), []string{"a", "b"}); e == nil {
		t.Fatal("inconsistent dimensions accepted")
	}
}
func TestPDF(t *testing.T) {
	if os.Getenv("PDF_TESTS") != "1" {
		t.Skip("PDF_TESTS=1 requires built extractor image")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	b := []byte("%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]/Resources<</Font<</F1 4 0 R>>>>/Contents 5 0 R>>endobj\n4 0 obj<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>endobj\n5 0 obj<</Length 49>>stream\nBT /F1 12 Tf 72 720 Td (Hello PDF knowledge) Tj ET\nendstream\nendobj\ntrailer<</Root 1 0 R>>\n%%EOF")
	p, e := Extract(ctx, b, "pdf")
	if e != nil || len(p) != 1 || p[0].Number != 1 || !strings.Contains(p[0].Text, "Hello PDF") {
		t.Fatal(p, e)
	}
}
func TestPublicImport(t *testing.T) {
	if os.Getenv("PUBLIC_IMPORT_TESTS") != "1" {
		t.Skip("PUBLIC_IMPORT_TESTS=1 requires internet")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	p, e := NewImporter().Page(ctx, "https://example.com/")
	if e != nil || p.Title != "Example Domain" || len(p.Text) < 20 {
		t.Fatal(p, e)
	}
}
