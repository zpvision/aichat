package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	foundation "site-assistant"
	"site-assistant/internal/ai"
	"site-assistant/internal/config"
	"site-assistant/internal/core"
	"site-assistant/internal/database"
	"site-assistant/internal/knowledge"
	"strings"
	"testing"
)

func TestIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	u, e := url.Parse(dsn)
	if e != nil || !strings.HasSuffix(u.Path, "_test") {
		t.Fatal("test database name MUST end with _test")
	}
	ctx := context.Background()
	p, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	// Isolated schema in a dedicated test database; never truncates shared tables.
	schema := "test_" + strings.ReplaceAll(core.ID(), "-", "")
	if _, e = p.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal(e)
	}
	defer p.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil {
		t.Fatal(e)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	for i := 0; i < 2; i++ {
		if e = database.Migrate(ctx, db, foundation.Migrations); e != nil {
			t.Fatal(e)
		}
	}
	store := &core.Store{DB: db}
	auth := core.NewService(store)
	kc, err := knowledge.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	kc.FileRoot = t.TempDir()
	k := knowledge.New(auth, kc)
	t.Setenv("AI_PROVIDER", "mock")
	ac, err := ai.LoadConfig(false)
	if err != nil {
		t.Fatal(err)
	}
	engine := ai.New(auth, previewTestRetriever{}, ac)
	engine.Provider = &ai.MockProvider{}
	h := NewWithAI(auth, config.Config{PublicURL: "http://localhost:5173"}, k, engine)
	call := func(method, path, body, token string, want int) map[string]any {
		t.Helper()
		r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		r.RemoteAddr = "127.0.0.1:9000"
		r.Header.Set("Origin", "http://localhost:5173")
		r.Header.Set("X-CSRF-Protection", "1")
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.AddCookie(&http.Cookie{Name: "session", Value: token})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		var out map[string]any
		json.Unmarshal(w.Body.Bytes(), &out)
		if cookies := w.Result().Cookies(); len(cookies) > 0 {
			out["token"] = cookies[0].Value
			if !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
				t.Fatal("cookie flags")
			}
		}
		return out
	}
	reg := func(email string) map[string]any {
		call("POST", "/api/v1/auth/register", fmt.Sprintf(`{"email":%q,"password":"test-password-12345","preferred_locale":"en"}`, email), "", 202)
		return call("POST", "/api/v1/auth/login", fmt.Sprintf(`{"email":%q,"password":"test-password-12345"}`, email), "", 200)
	}
	a := reg("a@example.com")
	duplicate := call("POST", "/api/v1/auth/register", `{"email":"a@example.com","password":"another-password-12345","preferred_locale":"de"}`, "", 202)
	if duplicate["ok"] != true || duplicate["token"] != nil {
		t.Fatal("registration leaked account or issued session")
	}
	token := a["token"].(string)
	b := reg("b@example.com")
	bt := b["token"].(string)
	call("GET", "/api/v1/profile", "", "", 401)
	call("GET", "/api/v1/auth/me", "", token, 200)
	call("POST", "/api/v1/auth/login", `{"email":"a@example.com","password":"wrong"}`, "", 401)
	call("POST", "/api/v1/auth/login", `{"email":"absent@example.com","password":"wrong"}`, "", 401)
	login := call("POST", "/api/v1/auth/login", `{"email":"a@example.com","password":"test-password-12345"}`, "", 200)
	lt := login["token"].(string)
	o := call("POST", "/api/v1/organizations", `{"name":"Tenant A","country":"CZ","timezone":"Europe/Prague","default_locale":"cs"}`, token, 201)
	id := o["id"].(string)
	base := "/api/v1/organizations/" + id
	call("PATCH", base, `{"name":"Tenant A updated","timezone":"Europe/Berlin"}`, token, 200)
	call("PATCH", base, `{"timezone":"Not/AZone"}`, token, 422)
	call("GET", base, "", bt, 404)
	call("PATCH", base, `{"name":"stolen"}`, bt, 404)
	call("GET", base+"/assistants", "", bt, 404)
	call("GET", base+"/sites", "", bt, 404)
	assistant := call("POST", base+"/assistants", `{"name":"Helper","default_locale":"de","language_policy":"visitor"}`, token, 201)
	aid := assistant["id"].(string)
	aiBase := base + "/assistants/" + aid
	call("GET", aiBase+"/ai/settings", "", token, 200)
	call("PATCH", aiBase+"/ai/settings", `{"role":"Support","language_policy":"fixed","response_locale":"de"}`, token, 200)
	conversation := call("POST", aiBase+"/preview/conversations", `{}`, token, 201)
	chatPath := aiBase + "/preview/conversations/" + conversation["id"].(string)
	call("GET", chatPath, "", bt, 404)
	messageBody := fmt.Sprintf(`{"request_id":%q,"content":"What is the price?"}`, core.ID())
	call("POST", chatPath+"/messages", messageBody, token, 200)
	replayed := call("POST", chatPath+"/messages", messageBody, token, 200)
	if replayed["replay"] != true {
		t.Fatal("HTTP replay")
	}
	streamReq := httptest.NewRequest("POST", chatPath+"/messages/stream", strings.NewReader(fmt.Sprintf(`{"request_id":%q,"content":"Hello"}`, core.ID())))
	streamReq.Header.Set("Origin", "http://localhost:5173")
	streamReq.Header.Set("X-CSRF-Protection", "1")
	streamReq.Header.Set("Content-Type", "application/json")
	streamReq.AddCookie(&http.Cookie{Name: "session", Value: token})
	streamResponse := httptest.NewRecorder()
	h.ServeHTTP(streamResponse, streamReq)
	if streamResponse.Code != 200 || !strings.Contains(streamResponse.Body.String(), "event: delta") || !strings.Contains(streamResponse.Body.String(), "event: complete") {
		t.Fatal("HTTP SSE", streamResponse.Code, streamResponse.Body.String())
	}
	call("GET", chatPath, "", token, 200)
	kbase := base + "/assistants/" + aid + "/knowledge"
	article := call("POST", kbase+"/articles", `{"title":"Article","content":"Knowledge content","language":"en"}`, token, 202)
	kid := article["id"].(string)
	call("GET", kbase+"/"+kid, "", token, 200)
	call("GET", kbase+"/"+kid, "", bt, 404)
	call("PATCH", kbase+"/"+kid, `{"content":"Updated content"}`, token, 202)
	call("POST", kbase+"/import-url", `{"url":"http://127.0.0.1"}`, token, 422)
	call("PATCH", base+"/assistants/"+aid, `{"status":"active"}`, token, 422)
	call("POST", base+"/assistants", `{"name":""}`, token, 422)
	site := call("POST", base+"/sites", fmt.Sprintf(`{"name":"Website","domain":"EXAMPLE.com.","assistant_id":%q}`, aid), token, 201)
	if site["domain"] != "example.com" {
		t.Fatal("normalization")
	}
	call("POST", base+"/sites", fmt.Sprintf(`{"name":"Bad","domain":"https://example.com","assistant_id":%q}`, aid), token, 422)
	ob := call("POST", "/api/v1/organizations", `{"name":"Tenant B","country":"DE","timezone":"UTC"}`, bt, 201)
	bb := "/api/v1/organizations/" + ob["id"].(string)
	call("GET", bb+"/assistants/"+aid, "", bt, 404)
	if _, err := db.Exec(ctx, "INSERT INTO sites(id,organization_id,assistant_id,name,domain) VALUES($1,$2,$3,'Invalid','invalid.example')", core.ID(), ob["id"], aid); err == nil {
		t.Fatal("database allowed cross-tenant assistant FK")
	}
	call("PATCH", bb+"/assistants/"+aid, `{"name":"stolen"}`, bt, 404)
	call("POST", bb+"/sites", fmt.Sprintf(`{"name":"Cross tenant","domain":"other.com","assistant_id":%q}`, aid), bt, 404)
	if _, e = db.Exec(ctx, "INSERT INTO organization_members(organization_id,user_id,role) VALUES($1,$2,'operator')", id, b["id"]); e != nil {
		t.Fatal(e)
	}
	call("GET", base, "", bt, 200)
	call("POST", base+"/assistants", `{"name":"blocked"}`, bt, 403)
	call("GET", kbase+"/"+kid, "", bt, 200)
	call("GET", chatPath, "", bt, 403)
	call("POST", chatPath+"/messages", fmt.Sprintf(`{"request_id":%q,"content":"Hi"}`, core.ID()), bt, 403)
	call("PATCH", aiBase+"/ai/settings", `{"role":"Forbidden"}`, bt, 403)
	call("POST", kbase+"/articles", `{"title":"Forbidden","content":"No"}`, bt, 403)
	call("DELETE", kbase+"/"+kid, `{}`, bt, 403)
	call("DELETE", base+"/sites/"+site["id"].(string), `{}`, bt, 403)
	db.Exec(ctx, "UPDATE organization_members SET role='admin' WHERE organization_id=$1 AND user_id=$2", id, b["id"])
	call("PATCH", base, `{"name":"blocked"}`, bt, 403)
	call("PATCH", base+"/assistants/"+aid, `{"description":"updated"}`, bt, 200)
	call("POST", kbase+"/"+kid+"/reindex", `{}`, bt, 202)
	call("DELETE", kbase+"/"+kid, `{}`, token, 200)
	call("GET", kbase+"/"+kid, "", token, 404)
	call("POST", chatPath+"/cancel", `{}`, token, 200)
	call("DELETE", chatPath, `{}`, token, 200)
	for _, locale := range []string{"ru", "en", "cs", "de"} {
		call("PATCH", "/api/v1/profile", fmt.Sprintf(`{"preferred_locale":%q}`, locale), token, 200)
	}
	call("PATCH", "/api/v1/profile", `{"preferred_locale":"xx"}`, token, 422)
	call("DELETE", base+"/sites/"+site["id"].(string), `{}`, token, 200)
	call("POST", "/api/v1/auth/logout", `{}`, lt, 200)
	call("GET", "/api/v1/auth/me", "", lt, 401)
	if _, err := db.Exec(ctx, "UPDATE sessions SET expires_at=now()-interval '1 second' WHERE token_hash=$1", core.TokenHash(token)); err != nil {
		t.Fatal(err)
	}
	call("GET", "/api/v1/auth/me", "", token, 401)
	r := httptest.NewRequest("POST", "/api/v1/auth/logout", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("CSRF bypass")
	}
	for i := 0; i < 12; i++ {
		r := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{}`))
		r.Header.Set("Origin", "http://localhost:5173")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Protection", "1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if i == 11 && w.Code != 429 {
			t.Fatal("rate limit missing")
		}
	}
}
