package billing_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	foundation "site-assistant"
	"site-assistant/internal/ai"
	"site-assistant/internal/billing"
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

func TestPeriodRolloverIsAtomicAndPreservesHistory(t *testing.T) {
	f := setup(t)
	if _, e := f.db.Exec(f.ctx, "UPDATE billing_subscriptions SET status='active',valid_until=now()+interval '60 days' WHERE organization_id=$1", f.org); e != nil {
		t.Fatal(e)
	}
	if _, e := f.db.Exec(f.ctx, "UPDATE billing_periods SET starts_at=now()-interval '30 days 1 hour',ends_at=now()-interval '1 hour',ai_used=7 WHERE organization_id=$1", f.org); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	errorsCh := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := billing.Snapshot(f.ctx, f.db, f.org); errorsCh <- e }()
	}
	wg.Wait()
	close(errorsCh)
	for e := range errorsCh {
		if e != nil {
			t.Fatal(e)
		}
	}
	var periods int
	var total, current int64
	if e := f.db.QueryRow(f.ctx, "SELECT count(*),sum(ai_used) FROM billing_periods WHERE organization_id=$1", f.org).Scan(&periods, &total); e != nil {
		t.Fatal(e)
	}
	if e := f.db.QueryRow(f.ctx, "SELECT ai_used FROM billing_periods WHERE organization_id=$1 ORDER BY starts_at DESC LIMIT 1", f.org).Scan(&current); e != nil {
		t.Fatal(e)
	}
	if periods != 2 || total != 7 || current != 0 {
		t.Fatal("period rollover lost history or duplicated periods", periods, total, current)
	}
	// Trial expiration cannot silently renew into a fresh allowance.
	if _, e := f.db.Exec(f.ctx, "UPDATE billing_subscriptions SET status='trialing',valid_until=now()-interval '1 minute' WHERE organization_id=$1", f.org); e != nil {
		t.Fatal(e)
	}
	if _, e := billing.Snapshot(f.ctx, f.db, f.org); e != nil {
		t.Fatal(e)
	}
	var status string
	if e := f.db.QueryRow(f.ctx, "SELECT status FROM billing_subscriptions WHERE organization_id=$1", f.org).Scan(&status); e != nil || status != "expired" {
		t.Fatal(status, e)
	}
}

func TestBillingHTTPAuthorizationAndPublicQuotaMask(t *testing.T) {
	f := setup(t)
	handler := httpapi.NewWithWidget(f.auth, config.Config{PublicURL: "http://localhost:5173"}, nil, f.ai, f.widgets)
	path := "/api/v1/organizations/" + f.org + "/billing"
	for _, tc := range []struct {
		user   string
		status int
	}{{"", 401}, {f.owner, 200}, {f.op1, 403}, {f.outsider, 404}} {
		r := httptest.NewRequest("GET", path, nil)
		if tc.user != "" {
			token := core.ID() + core.ID()
			if e := f.auth.Store.AddSession(f.ctx, core.TokenHash(token), tc.user); e != nil {
				t.Fatal(e)
			}
			r.AddCookie(&http.Cookie{Name: "session", Value: token})
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("billing authorization: want %d got %d: %s", tc.status, w.Code, w.Body.String())
		}
	}
	f.limit(t, "demo_responses", 0)
	body, _ := json.Marshal(ai.MessageInput{RequestID: core.ID(), Content: "Please help with services"})
	r := httptest.NewRequest("POST", "/api/v1/public/widgets/"+f.widget+"/sessions/"+f.session+"/messages", strings.NewReader(string(body)))
	r.Header.Set("Origin", "http://localhost:4173")
	r.Header.Set("Authorization", "Bearer "+f.token)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if !strings.Contains(w.Body.String(), "ai_unavailable") || strings.Contains(w.Body.String(), "plan_limit") || strings.Contains(w.Body.String(), "subscription") {
		t.Fatalf("public quota details: %s", w.Body.String())
	}
	var saved int
	if e := f.db.QueryRow(f.ctx, "SELECT count(*) FROM conversation_messages WHERE conversation_id=$1 AND role='user'", f.id).Scan(&saved); e != nil || saved != 1 {
		t.Fatal("visitor message must survive quota failure", saved, e)
	}
}

func (retriever) Search(context.Context, string, string, string, knowledge.SearchInput) ([]knowledge.Hit, error) {
	return nil, nil
}

func (f *fixture) limit(t *testing.T, key string, n any) {
	t.Helper()
	b, _ := json.Marshal(n)
	_, e := f.db.Exec(f.ctx, "UPDATE billing_periods SET limits=jsonb_set(limits,ARRAY[$2],$3::jsonb) WHERE organization_id=$1", f.org, key, b)
	if e != nil {
		t.Fatal(e)
	}
}
func (f *fixture) usage(t *testing.T) (int64, int64, int64, int64) {
	t.Helper()
	var a, r, d, dr int64
	if e := f.db.QueryRow(f.ctx, "SELECT ai_used,ai_reserved,demo_used,demo_reserved FROM billing_periods WHERE organization_id=$1 ORDER BY starts_at DESC LIMIT 1", f.org).Scan(&a, &r, &d, &dr); e != nil {
		t.Fatal(e)
	}
	return a, r, d, dr
}

type contextRetriever struct{}

func (contextRetriever) Search(context.Context, string, string, string, knowledge.SearchInput) ([]knowledge.Hit, error) {
	return []knowledge.Hit{{ID: core.ID(), SourceID: core.ID(), Title: "Terms", Content: "The Aurora accounting service costs 450 EUR monthly.", Revision: 1}}, nil
}
func TestPaidQuotaTenRequestsThreeReservations(t *testing.T) {
	f := setup(t)
	f.limit(t, "ai_responses", 3)
	f.limit(t, "ai_concurrent", 10)
	entered := make(chan struct{}, 10)
	release := make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		entered <- struct{}{}
		<-release
		json.NewEncoder(w).Encode(map[string]any{"model": "local-contract-test", "done": true, "message": map[string]string{"role": "assistant", "content": "Contract test response"}, "prompt_eval_count": 10, "eval_count": 20})
	}))
	defer server.Close()
	cfg := f.ai.Config
	cfg.Provider = "ollama"
	cfg.Model = "local-contract-test"
	cfg.BaseURL = server.URL
	cfg.Concurrency = 16
	cfg.Timeout = 15 * time.Second
	engine := ai.New(f.auth, contextRetriever{}, cfg)
	secondEngine := ai.New(f.auth, contextRetriever{}, cfg)
	ids := []string{}
	for i := 0; i < 10; i++ {
		c, e := engine.Create(f.ctx, f.owner, ai.Scope{OrganizationID: f.org, AssistantID: f.assistant})
		if e != nil {
			t.Fatal(e)
		}
		ids = append(ids, c.ID)
	}
	errorsCh := make(chan error, 10)
	for i, id := range ids {
		service := engine
		if i%2 == 1 {
			service = secondEngine
		}
		go func(id string) {
			_, e := service.Message(f.ctx, f.owner, ai.Scope{OrganizationID: f.org, AssistantID: f.assistant, ConversationID: id}, ai.MessageInput{RequestID: core.ID(), Content: "What does Aurora cost?"}, false, nil)
			errorsCh <- e
		}(id)
	}
	for i := 0; i < 3; i++ {
		select {
		case <-entered:
		case <-time.After(8 * time.Second):
			close(release)
			t.Fatal("not all three admitted")
		}
	}
	a, r, _, _ := f.usage(t)
	if a != 0 || r != 3 {
		close(release)
		t.Fatal("not reserved atomically", a, r)
	}
	close(release)
	ok := 0
	for i := 0; i < 10; i++ {
		if e := <-errorsCh; e == nil {
			ok++
		} else {
			fault(t, e, "plan_limit_exceeded")
		}
	}
	a, r, d, dr := f.usage(t)
	if ok != 3 || calls.Load() != 3 || a != 3 || r != 0 || d != 0 || dr != 0 {
		t.Fatal(ok, calls.Load(), a, r, d, dr)
	}
	var charged int
	f.db.QueryRow(f.ctx, "SELECT count(*) FROM ai_executions WHERE organization_id=$1 AND commercial_counted", f.org).Scan(&charged)
	if charged != 3 {
		t.Fatal(charged)
	}
}
func TestMockFallbackCancelAndRecovery(t *testing.T) {
	f := setup(t)
	f.limit(t, "ai_responses", 0)
	f.limit(t, "demo_responses", 2)
	scope := ai.Scope{OrganizationID: f.org, AssistantID: f.assistant}
	c, e := f.ai.Create(f.ctx, f.owner, scope)
	if e != nil {
		t.Fatal(e)
	}
	scope.ConversationID = c.ID
	input := ai.MessageInput{RequestID: core.ID(), Content: "Hello"}
	first, e := f.ai.Message(f.ctx, f.owner, scope, input, false, nil)
	if e != nil {
		t.Fatal(e)
	}
	again, e := f.ai.Message(f.ctx, f.owner, scope, input, false, nil)
	if e != nil || !again.Replay || again.Execution.ID != first.Execution.ID {
		t.Fatal(e)
	}
	a, _, d, _ := f.usage(t)
	if a != 0 || d != 1 {
		t.Fatal(a, d)
	}
	ctx, cancel := context.WithCancel(f.ctx)
	_, e = f.ai.Message(ctx, f.owner, scope, ai.MessageInput{RequestID: core.ID(), Content: "Hello again"}, true, func(ev ai.Event) error {
		if ev.Type == "delta" {
			cancel()
		}
		return nil
	})
	if e == nil {
		t.Fatal("cancel expected")
	}
	_, r, d, dr := f.usage(t)
	if r != 0 || dr != 0 || d != 1 {
		t.Fatal("cancel charged", r, d, dr)
	}
	begin, e := f.ai.Store.Begin(f.ctx, scope, ai.MessageInput{RequestID: core.ID(), Content: "crash"}, "en", "test-hash", "mock", "demo", time.Second)
	if e != nil {
		t.Fatal(e)
	}
	if e = billing.Reserve(f.ctx, f.db, f.org, begin.Execution.ID); e != nil {
		t.Fatal(e)
	}
	f.db.Exec(f.ctx, "UPDATE ai_executions SET expires_at=now()-interval '1 second' WHERE id=$1", begin.Execution.ID)
	if e = f.ai.Store.Recover(f.ctx); e != nil {
		t.Fatal(e)
	}
	_, _, d, dr = f.usage(t)
	if d != 1 || dr != 0 {
		t.Fatal("recovery charged", d, dr)
	}
	if e = f.ai.Store.Finish(f.ctx, scope, begin.Execution.ID, "completed", "stale", "en", "", 0, ai.Usage{}); e == nil {
		t.Fatal("stale execution finished")
	}
	// Server fallback: no provider request, even with a zero commercial limit.
	f.ai.Config.Provider = "ollama"
	f.ai.Provider = ai.DisabledProvider{}
	result, e := f.ai.Message(f.ctx, f.owner, scope, ai.MessageInput{RequestID: core.ID(), Content: "What is unknown?"}, false, nil)
	if e != nil || result.AssistantMessage.Status != "completed" {
		t.Fatal(result, e)
	}
	a, _, d, _ = f.usage(t)
	if a != 0 || d != 1 {
		t.Fatal("fallback charged")
	}
}
func TestResourceConcurrencyAndManualAfterExpiry(t *testing.T) {
	f := setup(t)
	f.limit(t, "assistants", 2)
	name := "Another"
	var wins atomic.Int32
	var id string
	var mu sync.Mutex
	wg := sync.WaitGroup{}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := f.auth.SaveAssistant(f.ctx, f.owner, f.org, "", core.AssistantInput{Name: &name})
			if e == nil {
				wins.Add(1)
				mu.Lock()
				id = v.ID
				mu.Unlock()
			} else {
				fault(t, core.TranslateError(e), "plan_limit_exceeded")
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatal(wins.Load())
	}
	if _, e := f.db.Exec(f.ctx, "DELETE FROM assistants WHERE organization_id=$1 AND id=$2", f.org, id); e != nil {
		t.Fatal(e)
	}
	if _, e := f.auth.SaveAssistant(f.ctx, f.owner, f.org, "", core.AssistantInput{Name: &name}); e != nil {
		t.Fatal("delete did not release", e)
	}
	f.db.Exec(f.ctx, "UPDATE billing_subscriptions SET valid_until=now()-interval '1 second' WHERE organization_id=$1", f.org)
	_, e := f.ai.WebsiteMessage(f.ctx, f.widget, f.session, f.token, "http://localhost:4173", ai.MessageInput{RequestID: core.ID(), Content: "Visitor message must remain"}, false, nil)
	fault(t, e, "subscription_inactive")
	c := f.c(t, f.op1)
	c, e = f.box.Action(f.ctx, f.op1, f.org, f.id, "take", action(c))
	if e != nil {
		t.Fatal(e)
	}
	in := action(c)
	in.Content = "Manual help is available"
	if _, e = f.box.Reply(f.ctx, f.op1, f.org, f.id, in); e != nil {
		t.Fatal(e)
	}
	page, e := f.box.VisitorPage(f.ctx, f.widget, f.session, f.token, "http://localhost:4173", 0, 0)
	if e != nil || len(page.Messages) != 3 || page.Messages[0].Content != "Visitor message must remain" {
		t.Fatal(page, e)
	}
	if _, e = billing.Snapshot(f.ctx, f.db, f.org); e != nil {
		t.Fatal(e)
	}
	var status string
	f.db.QueryRow(f.ctx, "SELECT status FROM billing_subscriptions WHERE organization_id=$1", f.org).Scan(&status)
	if status != "expired" {
		t.Fatal(status)
	}
}
func TestStorageAccountingAndAdministrativeGrant(t *testing.T) {
	f := setup(t)
	f.limit(t, "knowledge_bytes", 10)
	cfg, e := knowledge.LoadConfig()
	if e != nil {
		t.Fatal(e)
	}
	cfg.FileRoot = t.TempDir()
	ks := knowledge.New(f.auth, cfg)
	a, e := ks.Upload(f.ctx, f.owner, f.org, f.assistant, "one.txt", []byte("abcdef"))
	if e != nil {
		t.Fatal(e)
	}
	if abandoned, e := billing.AbandonFile(f.ctx, f.db, f.org, a.ID); e != nil || abandoned {
		t.Fatal("attached original must survive an uncertain response", e)
	}
	if _, e = ks.Upload(f.ctx, f.owner, f.org, f.assistant, "two.txt", []byte("ghijkl")); e == nil {
		t.Fatal("storage overflow")
	}
	var used, reserved int64
	f.db.QueryRow(f.ctx, "SELECT (resources->>'knowledge_bytes')::bigint,(resources->>'reserved_bytes')::bigint FROM billing_subscriptions WHERE organization_id=$1", f.org).Scan(&used, &reserved)
	if used != 6 || reserved != 0 {
		t.Fatal(used, reserved)
	}
	if _, e = ks.Reindex(f.ctx, f.owner, f.org, f.assistant, a.ID); e != nil {
		t.Fatal(e)
	}
	if e = ks.Delete(f.ctx, f.owner, f.org, f.assistant, a.ID); e != nil {
		t.Fatal(e)
	}
	f.db.QueryRow(f.ctx, "SELECT (resources->>'knowledge_bytes')::bigint FROM billing_subscriptions WHERE organization_id=$1", f.org).Scan(&used)
	if used != 0 {
		t.Fatal("delete bytes", used)
	}
	token := "test-administrator-token-at-least-32-characters"
	hash := sha256.Sum256([]byte(token))
	t.Setenv("BILLING_ADMIN_TOKEN_HASH", hex.EncodeToString(hash[:]))
	t.Setenv("BILLING_ADMIN_TOKEN", token)
	in := billing.AdminInput{OrganizationID: f.org, ExpectedName: "Inbox company", Plan: "start", Action: "grant", Days: 60, RequestID: core.ID(), Actor: "test-admin"}
	if _, e = billing.Admin(f.ctx, f.db, in); e != nil {
		t.Fatal(e)
	}
	if _, e = billing.Admin(f.ctx, f.db, in); e != nil {
		t.Fatal("grant replay", e)
	}
	var n int
	f.db.QueryRow(f.ctx, "SELECT count(*) FROM billing_audit WHERE organization_id=$1", f.org).Scan(&n)
	if n != 1 {
		t.Fatal("duplicate audit", n)
	}
	in.ExpectedName = "Wrong company"
	if _, e = billing.Admin(f.ctx, f.db, in); e == nil {
		t.Fatal("wrong company")
	}
	t.Setenv("BILLING_ADMIN_TOKEN", "wrong")
	if _, e = billing.Admin(f.ctx, f.db, in); e == nil {
		t.Fatal("missing admin credentials")
	}
}

func TestSitesMembersSourcesUnlimitedAndAPIQuota(t *testing.T) {
	f := setup(t)
	f.limit(t, "sites", 2)
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := f.auth.AddSite(f.ctx, f.owner, f.org, core.SiteInput{Name: "New", Domain: "quota.example", AssistantID: f.assistant})
			if e == nil {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatal("site quota race", wins.Load())
	}
	f.limit(t, "members", 4)
	other, e := f.auth.Register(f.ctx, "extra@billing.example", "billing-extra-password", "en")
	if e != nil {
		t.Fatal(e)
	}
	wins.Store(0)
	for _, id := range []string{f.outsider, other.ID} {
		invite, e := f.box.CreateInvitation(f.ctx, f.owner, f.org, "operator")
		if e != nil {
			t.Fatal(e)
		}
		token := strings.Split(invite.URL, "#invite=")[1]
		wg.Add(1)
		go func(id, token string) {
			defer wg.Done()
			if _, e := f.box.Accept(f.ctx, id, token); e == nil {
				wins.Add(1)
			}
		}(id, token)
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatal("member quota race", wins.Load())
	}
	f.limit(t, "knowledge_sources", 1)
	cfg, _ := knowledge.LoadConfig()
	ks := knowledge.New(f.auth, cfg)
	wins.Store(0)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			title, content := "A", "Text"
			if _, e := ks.Article(f.ctx, f.owner, f.org, f.assistant, knowledge.Input{Title: &title, Content: &content}); e == nil {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatal("source quota race", wins.Load())
	}
	f.limit(t, "assistants", 0)
	name := "Blocked"
	_, e = f.auth.SaveAssistant(f.ctx, f.owner, f.org, "", core.AssistantInput{Name: &name})
	fault(t, core.TranslateError(e), "plan_limit_exceeded")
	f.limit(t, "assistants", nil)
	if _, e = f.auth.SaveAssistant(f.ctx, f.owner, f.org, "", core.AssistantInput{Name: &name}); e != nil {
		t.Fatal("null is unlimited", e)
	}
	f.limit(t, "api_per_minute", 0)
	s := integrations.New(f.auth, integrations.Config{})
	key, e := s.CreateKey(f.ctx, f.owner, f.org, integrations.KeyInput{Name: "Quota", Days: 1, Scopes: []string{"leads:read"}})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Authenticate(f.ctx, key["token"].(string), "leads:read")
	fault(t, e, "plan_limit_exceeded")
	if e = billing.Configure(f.ctx, f.db, "ru", 14); e == nil {
		t.Fatal("cross-region billing")
	}
}

type unreliableFiles struct {
	knowledge.LocalStorage
	Fail bool
}

func (f *unreliableFiles) Delete(ctx context.Context, key string) error {
	if f.Fail {
		return errors.New("test storage unavailable")
	}
	return f.LocalStorage.Delete(ctx, key)
}
func TestStorageConcurrentAndDelayedDeletion(t *testing.T) {
	f := setup(t)
	f.limit(t, "knowledge_bytes", 10)
	cfg, _ := knowledge.LoadConfig()
	cfg.FileRoot = t.TempDir()
	ks := knowledge.New(f.auth, cfg)
	files := &unreliableFiles{LocalStorage: knowledge.LocalStorage{Root: cfg.FileRoot}}
	ks.Files = files
	type answer struct {
		s knowledge.Source
		e error
	}
	done := make(chan answer, 2)
	for _, body := range []string{"abcdef", "ghijkl"} {
		go func(body string) {
			v, e := ks.Upload(f.ctx, f.owner, f.org, f.assistant, "test.txt", []byte(body))
			done <- answer{v, e}
		}(body)
	}
	var source knowledge.Source
	wins := 0
	for i := 0; i < 2; i++ {
		a := <-done
		if a.e == nil {
			wins++
			source = a.s
		}
	}
	if wins != 1 {
		t.Fatal(wins)
	}
	files.Fail = true
	if e := ks.Delete(f.ctx, f.owner, f.org, f.assistant, source.ID); e == nil {
		t.Fatal("storage failure expected")
	}
	var used int64
	f.db.QueryRow(f.ctx, "SELECT (resources->>'knowledge_bytes')::bigint FROM billing_subscriptions WHERE organization_id=$1", f.org).Scan(&used)
	if used != 6 {
		t.Fatal("freed before actual deletion", used)
	}
	files.Fail = false
	ctx, cancel := context.WithTimeout(f.ctx, 1500*time.Millisecond)
	defer cancel()
	ks.Run(ctx)
	f.db.QueryRow(f.ctx, "SELECT (resources->>'knowledge_bytes')::bigint FROM billing_subscriptions WHERE organization_id=$1", f.org).Scan(&used)
	if used != 0 {
		t.Fatal("cleanup did not release", used)
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
