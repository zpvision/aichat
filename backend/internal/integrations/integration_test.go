package integrations_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	foundation "site-assistant"
	"site-assistant/internal/ai"
	"site-assistant/internal/config"
	"site-assistant/internal/core"
	"site-assistant/internal/database"
	"site-assistant/internal/httpapi"
	"site-assistant/internal/inbox"
	"site-assistant/internal/integrations"
	"site-assistant/internal/knowledge"
	"site-assistant/internal/widget"
	"strings"
	"sync"
	"sync/atomic"

	"testing"
	"time"
)

type retriever struct{}

func TestStartupEncryptionContinuity(t *testing.T) {
	f := setup(t)
	key := bytes.Repeat([]byte{7}, 32)
	s := integrations.New(f.auth, integrations.Config{Key: key})
	if _, e := s.CreateEndpoint(f.ctx, f.owner, f.org, integrations.EndpointInput{URL: "https://hooks.company.cz/webhook"}); e != nil {
		t.Fatal(e)
	}
	if e := integrations.New(f.auth, integrations.Config{Key: key}).ValidateSecrets(f.ctx); e != nil {
		t.Fatal("restart with original key", e)
	}
	for _, bad := range [][]byte{nil, bytes.Repeat([]byte{8}, 32)} {
		if e := integrations.New(f.auth, integrations.Config{Key: bad}).ValidateSecrets(f.ctx); e == nil {
			t.Fatal("missing/wrong key accepted")
		}
	}
}

func (retriever) Search(context.Context, string, string, string, knowledge.SearchInput) ([]knowledge.Hit, error) {
	return nil, nil
}

func makeService(f *fixture, c integrations.Config) *integrations.Service {
	return integrations.New(f.auth, c)
}
func TestKeysExternalAPIAndRecovery(t *testing.T) {
	f := setup(t)
	s := makeService(f, integrations.Config{})
	ctx := f.ctx
	created, e := s.CreateKey(ctx, f.owner, f.org, integrations.KeyInput{Name: "CRM", Scopes: []string{"leads:read", "leads:write"}, Days: 30})
	if e != nil {
		t.Fatal(e)
	}
	secret := created["token"].(string)
	if _, e = s.CreateKey(ctx, f.op1, f.org, integrations.KeyInput{Name: "Forbidden", Scopes: []string{"leads:read"}, Days: 1}); e == nil {
		t.Fatal("operator key creation")
	}
	if _, e = s.CreateKey(ctx, f.outsider, f.org, integrations.KeyInput{Name: "Foreign", Scopes: []string{"leads:read"}, Days: 1}); e == nil {
		t.Fatal("tenant key creation")
	}
	p, e := s.Authenticate(ctx, secret, "leads:write")
	if e != nil || p.OrganizationID != f.org {
		t.Fatal(e, p)
	}
	f.widgets.Integrations = s
	handler := httpapi.NewWithWidget(f.auth, config.Config{PublicURL: "http://localhost:5173"}, nil, f.ai, f.widgets)
	body := []byte(`{"assistant_id":"` + f.assistant + `","values":{"full_name":"Anna","email":"anna@example.com"}}`)
	var lead integrations.ExternalLead
	for i := 0; i < 2; i++ {
		r := httptest.NewRequest("POST", "http://localhost/api/v1/external/leads", bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+secret)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", "request-one")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 201 {
			t.Fatal(w.Code, w.Body.String())
		}
		v := integrations.ExternalLead{}
		json.Unmarshal(w.Body.Bytes(), &v)
		if i == 1 && v.ID != lead.ID {
			t.Fatal("duplicate lead")
		}
		lead = v
	}
	if lead.ConversationID != nil || lead.Email != "anna@example.com" {
		t.Fatal(lead)
	}
	detail, e := f.widgets.Leads.Detail(ctx, f.owner, f.org, lead.ID)
	if e != nil || detail.Lead.CanOpenConversation || detail.Lead.Source != "api" {
		t.Fatal("CRM standalone", detail, e)
	}
	feed, e := s.Store.Feed(ctx, f.org, "", 1)
	if e != nil || len(feed["items"].([]map[string]any)) != 1 {
		t.Fatal(feed, e)
	}
	cursor := feed["next_cursor"].(string)
	event := feed["items"].([]map[string]any)[0]["event_id"]
	empty, e := s.Store.Feed(ctx, f.org, cursor, 1)
	if e != nil || len(empty["items"].([]map[string]any)) != 0 {
		t.Fatal(empty, e)
	}
	if _, e = s.Store.Lead(ctx, core.ID(), lead.ID); e == nil {
		t.Fatal("cross tenant lead")
	}
	if _, e = s.CreateLead(ctx, p, "request-one", integrations.CreateInput{AssistantID: f.assistant, Values: map[string]json.RawMessage{"email": json.RawMessage(`"other@example.com"`)}}); e == nil {
		t.Fatal("idempotency conflict")
	}
	ro, e := s.CreateKey(ctx, f.owner, f.org, integrations.KeyInput{Name: "Read only", Scopes: []string{"leads:read"}, Days: 1})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Authenticate(ctx, ro["token"].(string), "leads:write")
	fault(t, e, "api_scope")
	if e = s.Store.Revoke(ctx, f.org, created["id"].(string)); e != nil {
		t.Fatal(e)
	}
	_, e = s.Authenticate(ctx, secret, "leads:read")
	fault(t, e, "api_unauthorized")
	keys, e := s.Store.Rows(ctx, f.org, "keys")
	if e != nil || bytes.Contains(mustJSON(keys), []byte(secret)) || bytes.Contains(mustJSON(keys), []byte("token_hash")) {
		t.Fatal("key leaked")
	}
	if e = f.widgets.Leads.Delete(ctx, f.owner, f.org, lead.ID); e != nil {
		t.Fatal(e)
	}
	deleted, e := s.Store.Feed(ctx, f.org, "", 1)
	if e != nil || deleted["items"].([]map[string]any)[0]["deleted"] != true || deleted["items"].([]map[string]any)[0]["event_id"] != event {
		t.Fatal("tombstone event stability", deleted, e)
	}
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
func TestWebhooksRealReceiverRetriesSignatureAndIsolation(t *testing.T) {
	f := setup(t)
	ctx := f.ctx
	var calls atomic.Int32
	var bad atomic.Bool
	var secret string
	var eventID string
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if !integrations.Verify(secret, r.Header.Get("X-SiteAssistant-Timestamp"), r.Header.Get("X-SiteAssistant-Signature"), b, time.Now()) {
			bad.Store(true)
		}
		var payload map[string]any
		json.Unmarshal(b, &payload)
		if len(payload) != 4 || payload["event_type"] != "lead.created" || bytes.Contains(b, []byte("anna@")) {
			bad.Store(true)
		}
		id, _ := payload["event_id"].(string)
		if eventID != "" && id != eventID {
			bad.Store(true)
		}
		eventID = id
		if calls.Add(1) == 1 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(204)
	}))
	defer receiver.Close()
	c := integrations.Config{Key: bytes.Repeat([]byte{7}, 32), DevReceiver: receiver.URL}
	s := makeService(f, c)
	endpoint, e := s.CreateEndpoint(ctx, f.owner, f.org, integrations.EndpointInput{URL: receiver.URL})
	if e != nil {
		t.Fatal(e)
	}
	secret = endpoint["secret"].(string)
	key, e := s.CreateKey(ctx, f.owner, f.org, integrations.KeyInput{Name: "Write", Scopes: []string{"leads:write"}, Days: 1})
	if e != nil {
		t.Fatal(e)
	}
	p, e := s.Authenticate(ctx, key["token"].(string), "leads:write")
	if e != nil {
		t.Fatal(e)
	}
	in := integrations.CreateInput{AssistantID: f.assistant, Values: map[string]json.RawMessage{"email": json.RawMessage(`"anna@example.com"`)}}
	errch := make(chan error, 12)
	for i := 0; i < 12; i++ {
		go func() { _, e := s.CreateLead(ctx, p, "concurrent-one", in); errch <- e }()
	}
	for i := 0; i < 12; i++ {
		if e = <-errch; e != nil {
			t.Fatal(e)
		}
	}
	var n int
	f.db.QueryRow(ctx, "SELECT count(*) FROM leads WHERE organization_id=$1", f.org).Scan(&n)
	if n != 1 {
		t.Fatal("duplicate lead", n)
	}
	f.db.QueryRow(ctx, "SELECT count(*) FROM webhook_deliveries WHERE organization_id=$1", f.org).Scan(&n)
	if n != 1 {
		t.Fatal("duplicate job", n)
	}
	if _, e = s.Once(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = f.db.Exec(ctx, "UPDATE webhook_deliveries SET next_attempt_at=now() WHERE organization_id=$1", f.org); e != nil {
		t.Fatal(e)
	}
	// A fresh service simulates restart; delivery/event identity and signing key survive.
	restarted := makeService(f, c)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := restarted.Once(ctx); errch <- err }()
	}
	wg.Wait()
	for i := 0; i < 4; i++ {
		if e = <-errch; e != nil {
			t.Fatal(e)
		}
	}
	if calls.Load() != 2 || bad.Load() {
		t.Fatal("signature/privacy/duplicate send", calls.Load(), bad.Load())
	}
	rows, e := s.Store.Rows(ctx, f.org, "deliveries")
	if e != nil {
		t.Fatal(e)
	}
	var d struct{ ID, Status string }
	json.Unmarshal(rows[0], &d)
	if d.Status != "delivered" {
		t.Fatal(string(rows[0]))
	}
	attempts, e := s.Store.Attempts(ctx, f.org, d.ID)
	if e != nil || len(attempts) != 2 {
		t.Fatal(attempts, e)
	}
	if v, e := s.Store.Rows(ctx, core.ID(), "deliveries"); e != nil || len(v) != 0 {
		t.Fatal("foreign journal", v, e)
	}
	rot, e := s.Rotate(ctx, f.owner, f.org, endpoint["id"].(string))
	if e != nil || rot["secret"] == secret {
		t.Fatal(e)
	}
	if e = s.Store.Toggle(ctx, f.org, endpoint["id"].(string), false); e != nil {
		t.Fatal(e)
	}
	_, e = s.Store.Test(ctx, f.org, endpoint["id"].(string))
	fault(t, e, "integration_disabled")
}
func TestCursorCannotSkipUncommittedLead(t *testing.T) {
	f := setup(t)
	ctx := f.ctx
	s := makeService(f, integrations.Config{})
	tx, e := f.db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	first := core.ID()
	_, e = tx.Exec(ctx, `INSERT INTO leads(id,organization_id,assistant_id,source,snapshot,privacy_url) VALUES($1,$2,$3,'api','{}','')`, first, f.org, f.assistant)
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() {
		_, e := f.db.Exec(ctx, `INSERT INTO leads(id,organization_id,assistant_id,source,snapshot,privacy_url) VALUES($1,$2,$3,'api','{}','')`, core.ID(), f.org, f.assistant)
		done <- e
	}()
	select {
	case e = <-done:
		t.Fatal("later tenant event committed ahead of earlier event", e)
	case <-time.After(100 * time.Millisecond):
	}
	feed, e := s.Store.Feed(ctx, f.org, "", 100)
	if e != nil || len(feed["items"].([]map[string]any)) != 0 {
		t.Fatal(feed, e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	feed, e = s.Store.Feed(ctx, f.org, "", 1)
	if e != nil || feed["has_more"] != true {
		t.Fatal(feed, e)
	}
	last, e := s.Store.Feed(ctx, f.org, feed["next_cursor"].(string), 1)
	if e != nil || len(last["items"].([]map[string]any)) != 1 {
		t.Fatal(last, e)
	}
}

func TestLeaseFencingExhaustionAndDisable(t *testing.T) {
	f := setup(t)
	ctx := f.ctx
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(204)
	}))
	defer receiver.Close()
	s := makeService(f, integrations.Config{Key: bytes.Repeat([]byte{5}, 32), DevReceiver: receiver.URL})
	ep, e := s.CreateEndpoint(ctx, f.owner, f.org, integrations.EndpointInput{URL: receiver.URL})
	if e != nil {
		t.Fatal(e)
	}
	id := ep["id"].(string)
	delivery, e := s.Store.Test(ctx, f.org, id)
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { _, e := s.Once(ctx); done <- e }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("worker did not send")
	}
	if _, e = f.db.Exec(ctx, "UPDATE webhook_deliveries SET lease_until=now()-interval '1 second' WHERE id=$1", delivery); e != nil {
		close(release)
		t.Fatal(e)
	}
	if _, e = s.Once(ctx); e != nil {
		close(release)
		t.Fatal(e)
	}
	close(release)
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	var state string
	if e = f.db.QueryRow(ctx, "SELECT status FROM webhook_deliveries WHERE id=$1", delivery).Scan(&state); e != nil || state != "delivered" {
		t.Fatal("stale worker overwrote success", state, e)
	}
	attempts, e := s.Store.Attempts(ctx, f.org, delivery)
	if e != nil || len(attempts) != 2 || !bytes.Contains(mustJSON(attempts), []byte("lease_expired")) {
		t.Fatal("crash attempt journal", attempts, e)
	}
	next, e := s.Store.Test(ctx, f.org, id)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Store.Toggle(ctx, f.org, id, false); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Once(ctx); e != nil {
		t.Fatal(e)
	}
	f.db.QueryRow(ctx, "SELECT status FROM webhook_deliveries WHERE id=$1", next).Scan(&state)
	if state != "canceled" {
		t.Fatal(state)
	}
	if e = s.Store.Toggle(ctx, f.org, id, true); e != nil {
		t.Fatal(e)
	}
	if e = s.Store.Retry(ctx, f.org, next); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Once(ctx); e != nil {
		t.Fatal(e)
	}
	f.db.QueryRow(ctx, "SELECT status FROM webhook_deliveries WHERE id=$1", next).Scan(&state)
	if state != "delivered" {
		t.Fatal("retry failed", state)
	}
	expired, e := s.CreateKey(ctx, f.owner, f.org, integrations.KeyInput{Name: "Expiring", Days: 1, Scopes: []string{"leads:read"}})
	if e != nil {
		t.Fatal(e)
	}
	f.db.Exec(ctx, "UPDATE integration_keys SET expires_at=now()-interval '1 second' WHERE id=$1", expired["id"])
	_, e = s.Authenticate(ctx, expired["token"].(string), "leads:read")
	fault(t, e, "api_unauthorized")
	// Final-attempt crash is moved to failed instead of being stuck forever.
	last, e := s.Store.Test(ctx, f.org, id)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.db.Exec(ctx, "UPDATE webhook_deliveries SET status='processing',attempts=max_attempts,lease_until=now()-interval '1 second' WHERE id=$1", last)
	if e != nil {
		t.Fatal(e)
	}
	s.Once(ctx)
	f.db.QueryRow(ctx, "SELECT status FROM webhook_deliveries WHERE id=$1", last).Scan(&state)
	if state != "failed" {
		t.Fatal("final crash recovery", state)
	}
}
func (retriever) Retrieve(context.Context, string, string, knowledge.SearchInput) ([]knowledge.Hit, error) {
	return nil, nil
}

type fixture struct {
	ctx                                                                   context.Context
	db                                                                    *pgxpool.Pool
	auth                                                                  *core.Service
	ai                                                                    *ai.Service
	box                                                                   *inbox.Service
	widgets                                                               *widget.Service
	owner, op1, op2, outsider, org, assistant, widget, session, token, id string
}

func setup(t *testing.T) *fixture {
	t.Helper()
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
	schema := "inbox_" + strings.ReplaceAll(core.ID(), "-", "")
	if _, e = admin.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); admin.Close() })
	pc, _ := pgxpool.ParseConfig(dsn)
	pc.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	db, e := pgxpool.NewWithConfig(ctx, pc)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(db.Close)
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
	engine := ai.New(auth, retriever{}, ac)
	engine.Provider = &ai.MockProvider{}
	wc, e := widget.Load(config.Config{PublicURL: "http://localhost:5173"})
	if e != nil {
		t.Fatal(e)
	}
	wc.AllowLocalhost = true
	ws := widget.New(auth, engine, wc)
	f := &fixture{ctx: ctx, db: db, auth: auth, ai: engine, box: ws.Inbox, widgets: ws}
	users := []*string{&f.owner, &f.op1, &f.op2, &f.outsider}
	for i, p := range users {
		v, e := auth.Register(ctx, []string{"owner@inbox.example", "one@inbox.example", "two@inbox.example", "other@inbox.example"}[i], "inbox-test-password", "en")
		if e != nil {
			t.Fatal(e)
		}
		*p = v.ID
	}
	name, country := "Inbox company", "CZ"
	org, e := auth.SaveOrganization(ctx, f.owner, "", core.OrganizationInput{Name: &name, Country: &country})
	if e != nil {
		t.Fatal(e)
	}
	f.org = org.ID
	asst, e := auth.SaveAssistant(ctx, f.owner, f.org, "", core.AssistantInput{Name: &name})
	if e != nil {
		t.Fatal(e)
	}
	f.assistant = asst.ID
	site, e := auth.AddSite(ctx, f.owner, f.org, core.SiteInput{Name: "Inbox website", Domain: "inbox.example", AssistantID: f.assistant})
	if e != nil {
		t.Fatal(e)
	}
	w, e := ws.Create(ctx, f.owner, f.org, site.ID)
	if e != nil {
		t.Fatal(e)
	}
	on := true
	origins := []string{"http://localhost:4173"}
	theme := widget.Defaults()
	theme.PrivacyURL = "https://inbox.example/privacy"
	w, e = ws.Update(ctx, f.owner, f.org, site.ID, widget.Input{Enabled: &on, Origins: &origins, Settings: &theme})
	if e != nil {
		t.Fatal(e)
	}
	f.widget = w.Widget.ID
	s, e := ws.Session(ctx, w.Widget, origins[0])
	if e != nil {
		t.Fatal(e)
	}
	f.session = s.ID
	f.token = s.Token
	scope, e := engine.Store.WebsiteScope(ctx, f.widget, f.session, f.token, origins[0])
	if e != nil {
		t.Fatal(e)
	}
	f.id = scope.ConversationID
	for _, id := range []string{f.op1, f.op2} {
		v, e := f.box.CreateInvitation(ctx, f.owner, f.org, "operator")
		if e != nil {
			t.Fatal(e)
		}
		token := strings.Split(v.URL, "#invite=")[1]
		if _, e = f.box.Accept(ctx, id, token); e != nil {
			t.Fatal(e)
		}
		if _, e = f.box.Accept(ctx, id, token); e != nil {
			t.Fatal("idempotent invite", e)
		}
	}
	return f
}
func action(c inbox.Conversation) inbox.Action {
	return inbox.Action{RequestID: core.ID(), Version: c.Version}
}
func (f *fixture) c(t *testing.T, user string) inbox.Conversation {
	t.Helper()
	_, c, e := f.box.Access(f.ctx, user, f.org, f.id)
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func fault(t *testing.T, e error, code string) {
	t.Helper()
	var f *core.Fault
	if !errors.As(e, &f) || f.Code != code {
		t.Fatalf("expected %s, got %v", code, e)
	}
}
