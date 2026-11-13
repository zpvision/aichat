package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"image"
	"image/png"
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
	"site-assistant/internal/widget"
	"strings"
	"testing"
	"time"
)

type widgetRetriever struct{}

func (widgetRetriever) Search(context.Context, string, string, string, knowledge.SearchInput) ([]knowledge.Hit, error) {
	return []knowledge.Hit{}, nil
}
func (widgetRetriever) Retrieve(context.Context, string, string, knowledge.SearchInput) ([]knowledge.Hit, error) {
	return []knowledge.Hit{}, nil
}
func TestWidgetIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	u, e := url.Parse(dsn)
	if e != nil || !strings.HasSuffix(u.Path, "_test") {
		t.Fatal("test database required")
	}
	ctx := context.Background()
	admin, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer admin.Close()
	schema := "widget_" + strings.ReplaceAll(core.ID(), "-", "")
	if _, e = admin.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal(e)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	pc, _ := pgxpool.ParseConfig(dsn)
	pc.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	db, e := pgxpool.NewWithConfig(ctx, pc)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	for i := 0; i < 2; i++ {
		if e = database.Migrate(ctx, db, foundation.Migrations); e != nil {
			t.Fatal(e)
		}
	}
	auth := core.NewService(&core.Store{DB: db})
	t.Setenv("AI_PROVIDER", "mock")
	ac, e := ai.LoadConfig(false)
	if e != nil {
		t.Fatal(e)
	}
	engine := ai.New(auth, widgetRetriever{}, ac)
	engine.Provider = &ai.MockProvider{}
	kc, _ := knowledge.LoadConfig()
	k := knowledge.New(auth, kc)
	c := config.Config{PublicURL: "http://localhost:5173"}
	wc, e := widget.Load(c)
	if e != nil {
		t.Fatal(e)
	}
	wc.AllowLocalhost = true
	wc.FileRoot = t.TempDir()
	widgets := widget.New(auth, engine, wc)
	widgets.AI.WebsiteLimits.SessionMessages = 2
	h := NewWithWidget(auth, c, k, engine, widgets)
	call := func(method, path, body, cookie, origin, parent, token string, want int) map[string]any {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Protection", "1")
		r.Header.Set("Origin", origin)
		if parent != "" {
			r.Header.Set("X-Widget-Origin", parent)
		}
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: "session", Value: cookie})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d != %d, %s", method, path, w.Code, want, w.Body.String())
		}
		var out map[string]any
		json.Unmarshal(w.Body.Bytes(), &out)
		if out == nil {
			out = map[string]any{}
		}
		out["raw"] = w.Body.String()
		out["cors"] = w.Header().Get("Access-Control-Allow-Origin")
		if cookies := w.Result().Cookies(); len(cookies) > 0 {
			out["cookie"] = cookies[0].Value
		}
		return out
	}
	call("POST", "/api/v1/auth/register", `{"email":"widget@example.com","password":"widget-password-123","preferred_locale":"en"}`, "", c.PublicURL, "", "", 202)
	login := call("POST", "/api/v1/auth/login", `{"email":"widget@example.com","password":"widget-password-123"}`, "", c.PublicURL, "", "", 200)
	cookie := login["cookie"].(string)
	user := login["id"].(string)
	name, country := "Company", "CZ"
	org, e := auth.SaveOrganization(ctx, user, "", core.OrganizationInput{Name: &name, Country: &country})
	if e != nil {
		t.Fatal(e)
	}
	assistant, e := auth.SaveAssistant(ctx, user, org.ID, "", core.AssistantInput{Name: &name})
	if e != nil {
		t.Fatal(e)
	}
	site, e := auth.AddSite(ctx, user, org.ID, core.SiteInput{Name: "Site", Domain: "example.com", AssistantID: assistant.ID})
	if e != nil {
		t.Fatal(e)
	}
	path := "/api/v1/organizations/" + org.ID + "/sites/" + site.ID + "/widget"
	created := call("POST", path, `{}`, cookie, c.PublicURL, "", "", 201)
	wid := created["widget"].(map[string]any)["id"].(string)
	updated := call("PATCH", path, `{"enabled":true,"origins":["http://localhost:4173"]}`, cookie, c.PublicURL, "", "", 200)
	if !strings.Contains(updated["snippet"].(string), wid) {
		t.Fatal("installation code")
	}
	var logo bytes.Buffer
	if e = png.Encode(&logo, image.NewRGBA(image.Rect(0, 0, 24, 24))); e != nil {
		t.Fatal(e)
	}
	if _, e = widgets.UploadLogo(ctx, user, org.ID, site.ID, logo.Bytes()); e != nil {
		t.Fatal(e)
	}
	if _, e = widgets.UploadLogo(ctx, user, org.ID, site.ID, []byte(`<svg onload="alert(1)"/>`)); e == nil {
		t.Fatal("SVG logo accepted")
	}
	if _, e = widgets.UploadLogo(ctx, user, org.ID, site.ID, make([]byte, 257<<10)); e == nil {
		t.Fatal("oversized logo accepted")
	}
	pub := "/api/v1/public/widgets/" + wid
	response := call("GET", pub+"/config", "", "", "http://localhost:4173", "", "", 200)
	if response["cors"] != "http://localhost:4173" || strings.Contains(response["raw"].(string), org.ID) || response["demo"] != true {
		t.Fatal("public config leaked authority or CORS")
	}
	call("GET", pub+"/config", "", "", "https://evil.example", "", "", 403)
	call("GET", pub+"/config", "", "", wc.PublicURL, "https://evil.example", "", 403)
	call("GET", "/api/v1/public/widgets/"+core.ID()+"/config", "", "", "http://localhost:4173", "", "", 404)
	session := call("POST", pub+"/sessions", `{}`, "", wc.PublicURL, "http://localhost:4173", "", 201)
	sid, token := session["id"].(string), session["token"].(string)
	messages := pub + "/sessions/" + sid + "/messages"
	call("GET", messages, "", "", wc.PublicURL, "http://localhost:4173", token, 200)
	call("GET", messages, "", "", wc.PublicURL, "http://localhost:4173", strings.Repeat("x", 43), 401)
	call("GET", pub+"/sessions/"+core.ID()+"/messages", "", "", wc.PublicURL, "http://localhost:4173", token, 401)
	body := fmt.Sprintf(`{"request_id":%q,"content":"<script>alert(1)</script> Hello"}`, core.ID())
	first := call("POST", messages, body, "", wc.PublicURL, "http://localhost:4173", token, 200)
	if !strings.Contains(first["raw"].(string), "event: delta") || !strings.Contains(first["raw"].(string), "event: complete") || strings.Contains(first["raw"].(string), "organization_id") {
		t.Fatal("public SSE")
	}
	call("POST", messages, body, "", wc.PublicURL, "http://localhost:4173", token, 200)
	call("POST", messages, fmt.Sprintf(`{"request_id":%q,"content":"Second"}`, core.ID()), "", wc.PublicURL, "http://localhost:4173", token, 200)
	call("POST", messages, fmt.Sprintf(`{"request_id":%q,"content":"Third"}`, core.ID()), "", wc.PublicURL, "http://localhost:4173", token, 429)
	var count int
	db.QueryRow(ctx, "SELECT message_count FROM widget_sessions WHERE id=$1", sid).Scan(&count)
	if count != 2 {
		t.Fatal("replay consumed budget", count)
	}
	other := call("POST", pub+"/sessions", `{}`, "", wc.PublicURL, "http://localhost:4173", "", 201)
	call("GET", pub+"/sessions/"+other["id"].(string)+"/messages", "", "", wc.PublicURL, "http://localhost:4173", token, 401)
	history := call("GET", messages, "", "", wc.PublicURL, "http://localhost:4173", token, 200)
	if len(history["messages"].([]any)) != 4 {
		t.Fatal("history")
	}
	// A public ID never grants membership, and a visitor token cannot move between widgets/tenants.
	outsider, e := auth.Register(ctx, "outsider@example.com", "outsider-password-123", "en")
	if e != nil {
		t.Fatal(e)
	}
	outsiderToken, e := auth.NewSession(ctx, outsider.ID)
	if e != nil {
		t.Fatal(e)
	}
	call("GET", path, "", outsiderToken, c.PublicURL, "", "", 404)
	otherOrg, e := auth.SaveOrganization(ctx, outsider.ID, "", core.OrganizationInput{Name: &name, Country: &country})
	if e != nil {
		t.Fatal(e)
	}
	otherAssistant, e := auth.SaveAssistant(ctx, outsider.ID, otherOrg.ID, "", core.AssistantInput{Name: &name})
	if e != nil {
		t.Fatal(e)
	}
	otherSite, e := auth.AddSite(ctx, outsider.ID, otherOrg.ID, core.SiteInput{Name: "Other site", Domain: "other.example", AssistantID: otherAssistant.ID})
	if e != nil {
		t.Fatal(e)
	}
	otherWidget, e := widgets.Create(ctx, outsider.ID, otherOrg.ID, otherSite.ID)
	if e != nil {
		t.Fatal(e)
	}
	origins := []string{"http://localhost:4173"}
	if _, e = widgets.Update(ctx, outsider.ID, otherOrg.ID, otherSite.ID, widget.Input{Enabled: boolPtr(true), Origins: &origins}); e != nil {
		t.Fatal(e)
	}
	call("GET", "/api/v1/public/widgets/"+otherWidget.Widget.ID+"/sessions/"+sid+"/messages", "", "", wc.PublicURL, origins[0], token, 401)
	if _, e = db.Exec(ctx, "INSERT INTO organization_members(organization_id,user_id,role) VALUES($1,$2,'operator')", org.ID, outsider.ID); e != nil {
		t.Fatal(e)
	}
	call("GET", path, "", outsiderToken, c.PublicURL, "", "", 200)
	call("PATCH", path, `{"enabled":false}`, outsiderToken, c.PublicURL, "", "", 403)
	// Disabled inference is an explicit failure before consuming the visitor's budget.
	engine.Config.Provider = "disabled"
	call("POST", pub+"/sessions/"+other["id"].(string)+"/messages", fmt.Sprintf(`{"request_id":%q,"content":"Hello"}`, core.ID()), "", wc.PublicURL, origins[0], other["token"].(string), 503)
	engine.Config.Provider = "mock"
	engine.WebsiteLimits.SiteHourly = 2
	call("POST", pub+"/sessions/"+other["id"].(string)+"/messages", fmt.Sprintf(`{"request_id":%q,"content":"Budget"}`, core.ID()), "", wc.PublicURL, origins[0], other["token"].(string), 429)
	engine.WebsiteLimits.SiteHourly = 300
	engine.WebsiteLimits.SiteConcurrent = 1
	third := call("POST", pub+"/sessions", `{}`, "", wc.PublicURL, origins[0], "", 201)
	engine.Provider = &ai.MockProvider{Delay: 100 * time.Millisecond}
	started, finished := make(chan struct{}), make(chan error, 1)
	go func() {
		_, err := engine.WebsiteMessage(ctx, wid, other["id"].(string), other["token"].(string), origins[0], ai.MessageInput{RequestID: core.ID(), Content: "Concurrent"}, true, func(ev ai.Event) error {
			if ev.Type == "start" {
				close(started)
			}
			return nil
		})
		finished <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("generation did not start")
	}
	call("POST", pub+"/sessions/"+third["id"].(string)+"/messages", fmt.Sprintf(`{"request_id":%q,"content":"Concurrent other visitor"}`, core.ID()), "", wc.PublicURL, origins[0], third["token"].(string), 429)
	call("POST", pub+"/sessions/"+other["id"].(string)+"/messages", fmt.Sprintf(`{"request_id":%q,"content":"Same conversation"}`, core.ID()), "", wc.PublicURL, origins[0], other["token"].(string), 409)
	call("POST", pub+"/sessions/"+other["id"].(string)+"/cancel", `{}`, "", wc.PublicURL, origins[0], other["token"].(string), 200)
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("cancel succeeded as completed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancel hung")
	}
	conv := ""
	db.QueryRow(ctx, "SELECT id FROM conversations WHERE anonymous_session_id=$1", sid).Scan(&conv)
	call("GET", "/api/v1/organizations/"+org.ID+"/assistants/"+assistant.ID+"/preview/conversations/"+conv, "", cookie, c.PublicURL, "", "", 404)
	db.Exec(ctx, "UPDATE widget_sessions SET expires_at=now()-interval '1 second' WHERE id=$1", other["id"])
	call("GET", pub+"/sessions/"+other["id"].(string)+"/messages", "", "", wc.PublicURL, "http://localhost:4173", other["token"].(string), 401)
	call("PATCH", path, `{"enabled":false}`, cookie, c.PublicURL, "", "", 200)
	call("POST", messages, body, "", wc.PublicURL, "http://localhost:4173", token, 404)
	widgets.Config.Production = true
	widgets.Config.AllowLocalhost = false
	if _, e = widgets.Update(ctx, user, org.ID, site.ID, widget.Input{Enabled: boolPtr(true)}); e == nil {
		t.Fatal("production unverified enabled")
	}
	for n := 0; n < 6; n++ {
		widgets.Limiter.Allow("test", 5, 1000000000)
	}
	if widgets.Limiter.Allow("test", 5, 1000000000) {
		t.Fatal("rate cap")
	}
}
func boolPtr(v bool) *bool { return &v }
