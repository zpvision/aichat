package inbox_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/url"
	"os"
	foundation "site-assistant"
	"site-assistant/internal/ai"
	"site-assistant/internal/config"
	"site-assistant/internal/core"
	"site-assistant/internal/database"
	"site-assistant/internal/inbox"
	"site-assistant/internal/knowledge"
	"site-assistant/internal/widget"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type retriever struct{}

func (retriever) Search(context.Context, string, string, string, knowledge.SearchInput) ([]knowledge.Hit, error) {
	return nil, nil
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
	w, e = ws.Update(ctx, f.owner, f.org, site.ID, widget.Input{Enabled: &on, Origins: &origins})
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
func TestInboxWorkflowAndPrivacy(t *testing.T) {
	f := setup(t)
	ctx := f.ctx
	scope := ai.Scope{OrganizationID: f.org, AssistantID: f.assistant, ConversationID: f.id}
	_, e := f.ai.WebsiteMessage(ctx, f.widget, f.session, f.token, "http://localhost:4173", ai.MessageInput{RequestID: core.ID(), Content: "Hello"}, false, nil)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.ai.Create(ctx, f.owner, ai.Scope{OrganizationID: f.org, AssistantID: f.assistant}); e != nil {
		t.Fatal(e)
	}
	list, e := f.box.List(ctx, f.owner, f.org, inbox.Filter{Query: "Hello", Limit: 1})
	if e != nil || len(list.Items) != 1 || list.Items[0].Unread != 1 {
		t.Fatal("list/search/preview exclusion", list, e)
	}
	if _, _, e = f.box.Access(ctx, f.outsider, f.org, f.id); e == nil {
		t.Fatal("cross tenant access")
	}
	if _, e = f.ai.Detail(ctx, f.op1, scope, 0); e == nil {
		t.Fatal("operator preview access")
	}
	c, e := f.box.RequestHuman(ctx, f.widget, f.session, f.token, "http://localhost:4173", inbox.Action{RequestID: core.ID()})
	if e != nil || c.Mode != "human_requested" {
		t.Fatal(e, c)
	}
	c, e = f.box.Action(ctx, f.op1, f.org, f.id, "take", action(c))
	if e != nil || c.Assigned == nil || *c.Assigned != f.op1 {
		t.Fatal(e, c)
	}
	today := time.Now().UTC().Format("2006-01-02")
	filtered, e := f.box.List(ctx, f.op1, f.org, inbox.Filter{Mode: "human_active", Assigned: "me", Site: c.SiteID, Assistant: f.assistant, From: today, To: today})
	if e != nil || len(filtered.Items) != 1 {
		t.Fatal("combined mode/member/site/assistant/date filters", filtered, e)
	}
	emptyFilter, e := f.box.List(ctx, f.owner, f.org, inbox.Filter{Site: core.ID()})
	if e != nil || len(emptyFilter.Items) != 0 {
		t.Fatal("site scope filter", emptyFilter, e)
	}
	foreign := action(c)
	foreign.Assignee = f.outsider
	_, e = f.box.Action(ctx, f.owner, f.org, f.id, "assign", foreign)
	fault(t, e, "validation")
	_, e = f.box.Action(ctx, f.op1, f.org, f.id, "assign", foreign)
	fault(t, e, "forbidden")
	_, _, e = f.box.Access(ctx, f.op2, f.org, f.id)
	fault(t, e, "forbidden")
	a := action(c)
	a.Content = "Private secret note"
	if _, e = f.box.Action(ctx, f.op1, f.org, f.id, "note", a); e != nil {
		t.Fatal(e)
	}
	a = action(c)
	a.Content = "Human answer"
	m, e := f.box.Reply(ctx, f.op1, f.org, f.id, a)
	if e != nil {
		t.Fatal(e)
	}
	again, e := f.box.Reply(ctx, f.op1, f.org, f.id, a)
	if e != nil || again.ID != m.ID {
		t.Fatal("reply duplicate", e)
	}
	visitor, e := f.box.VisitorPage(ctx, f.widget, f.session, f.token, "http://localhost:4173", 0, 0)
	if e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(visitor)
	if strings.Contains(string(b), "Private secret note") {
		t.Fatal("public note leak")
	}
	if len(visitor.Messages) != 3 || visitor.Messages[len(visitor.Messages)-1].Role != "operator" {
		t.Fatal("human history", visitor.Messages)
	}
	history, e := f.ai.Store.History(ctx, scope, 20)
	if e != nil {
		t.Fatal(e)
	}
	b, _ = json.Marshal(history)
	if !strings.Contains(string(b), "Human answer") || strings.Contains(string(b), "Private secret note") {
		t.Fatal("prompt history", string(b))
	}
	if e = f.box.Read(ctx, f.op1, f.org, f.id, m.Sequence); e != nil {
		t.Fatal(e)
	}
	summary, e := f.box.Store.Summary(ctx, f.org, f.op1, "operator")
	if e != nil || summary["unread"] != 0 {
		t.Fatal(summary, e)
	}
	c = f.c(t, f.op1)
	resume := action(c)
	c, e = f.box.Action(ctx, f.op1, f.org, f.id, "resume-ai", resume)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.box.Action(ctx, f.op1, f.org, f.id, "resume-ai", resume); e != nil {
		t.Fatal("resume idempotency", e)
	}
	_, e = f.ai.WebsiteMessage(ctx, f.widget, f.session, f.token, "http://localhost:4173", ai.MessageInput{RequestID: core.ID(), Content: "Another question"}, false, nil)
	if e != nil {
		t.Fatal(e)
	}
	c = f.c(t, f.owner)
	assign := action(c)
	assign.Assignee = f.op2
	c, e = f.box.Action(ctx, f.owner, f.org, f.id, "assign", assign)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.box.Reply(ctx, f.op1, f.org, f.id, inbox.Action{RequestID: core.ID(), Version: c.Version, Content: "Intrusion"})
	fault(t, e, "forbidden")
	closeAction := action(c)
	c, e = f.box.Action(ctx, f.op2, f.org, f.id, "close", closeAction)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.box.Action(ctx, f.op2, f.org, f.id, "close", closeAction); e != nil {
		t.Fatal("close replay", e)
	}
	_, e = f.ai.WebsiteMessage(ctx, f.widget, f.session, f.token, "http://localhost:4173", ai.MessageInput{RequestID: core.ID(), Content: "Reopen?"}, false, nil)
	fault(t, e, "conversation_closed")
}

type heldProvider struct{ entered, release chan struct{} }

func (p heldProvider) Generate(ctx context.Context, r ai.GenerateRequest, stream bool, emit func(string) error) (ai.GenerateResult, error) {
	close(p.entered)
	<-p.release
	_ = emit("STALE")
	return ai.GenerateResult{Content: "STALE"}, nil
}
func TestPostgresFencingAndRaces(t *testing.T) {
	f := setup(t)
	ctx := f.ctx
	c := f.c(t, f.owner)
	p := heldProvider{make(chan struct{}), make(chan struct{})}
	f.ai.Provider = p
	done := make(chan error, 1)
	var leaked atomic.Int32
	go func() {
		_, e := f.ai.WebsiteMessage(ctx, f.widget, f.session, f.token, "http://localhost:4173", ai.MessageInput{RequestID: core.ID(), Content: "Racing question"}, true, func(ev ai.Event) error {
			if ev.Type == "delta" {
				leaked.Add(1)
			}
			return nil
		})
		done <- e
	}()
	<-p.entered
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, user := range []string{f.op1, f.op2} {
		wg.Add(1)
		go func(u string) {
			defer wg.Done()
			_, e := f.box.Action(ctx, u, f.org, f.id, "take", action(c))
			results <- e
		}(user)
	}
	wg.Wait()
	close(results)
	wins := 0
	for e := range results {
		if e == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatal("two operators claimed", wins)
	}
	close(p.release)
	if e := <-done; e == nil {
		t.Fatal("stale provider succeeded")
	}
	if leaked.Load() != 0 {
		t.Fatal("delta after handoff")
	}
	var successful int
	f.db.QueryRow(ctx, "SELECT count(*) FROM ai_executions WHERE conversation_id=$1 AND status='completed'", f.id).Scan(&successful)
	if successful != 0 {
		t.Fatal("stale Finish passed")
	}
	c = f.c(t, f.owner)
	a := action(c)
	a.Content = "Manual after handoff"
	if _, e := f.box.Reply(ctx, *c.Assigned, f.org, f.id, a); e != nil {
		t.Fatal(e)
	}
	// Submit while ownership changes: either AI is fenced or the visitor is saved in manual mode, never lost.
	f.ai.Provider = &ai.MockProvider{}
	c, e := f.box.Action(ctx, f.owner, f.org, f.id, "resume-ai", action(c))
	if e != nil {
		t.Fatal(e)
	}
	gate := make(chan struct{})
	errCh := make(chan error, 2)
	go func() {
		<-gate
		_, e := f.ai.WebsiteMessage(ctx, f.widget, f.session, f.token, "http://localhost:4173", ai.MessageInput{RequestID: core.ID(), Content: "During switch"}, true, func(ai.Event) error { return nil })
		errCh <- e
	}()
	go func() { <-gate; _, e := f.box.Action(ctx, f.op1, f.org, f.id, "take", action(c)); errCh <- e }()
	close(gate)
	<-errCh
	<-errCh
	var count int
	f.db.QueryRow(ctx, "SELECT count(*) FROM conversation_messages WHERE conversation_id=$1 AND content='During switch'", f.id).Scan(&count)
	if count != 1 {
		t.Fatal("visitor lost or duplicated", count)
	}
	// Human reply vs resume is serialized on the same row/version.
	c = f.c(t, f.owner)
	old := action(c)
	old.Content = "Racing staff"
	ch := make(chan error, 2)
	go func() { _, e := f.box.Reply(ctx, *c.Assigned, f.org, f.id, old); ch <- e }()
	go func() { _, e := f.box.Action(ctx, f.owner, f.org, f.id, "resume-ai", action(c)); ch <- e }()
	<-ch
	<-ch
	c = f.c(t, f.owner)
	if c.Mode != "ai_active" {
		t.Fatal("resume failed")
	}
	f.ai.Config.Provider = "disabled"
	r, e := f.ai.WebsiteMessage(ctx, f.widget, f.session, f.token, "http://localhost:4173", ai.MessageInput{RequestID: core.ID(), Content: "Without GPU"}, false, nil)
	if e != nil || !r.Manual || r.UserMessage.ID == "" {
		t.Fatal("disabled manual path", e)
	}
}
func TestInvitationsAndCursor(t *testing.T) {
	f := setup(t)
	ctx := f.ctx
	v, e := f.box.CreateInvitation(ctx, f.owner, f.org, "admin")
	if e != nil {
		t.Fatal(e)
	}
	token := strings.Split(v.URL, "#invite=")[1]
	if _, e = f.box.Accept(ctx, f.outsider, token); e != nil {
		t.Fatal(e)
	}
	if _, e = f.box.Accept(ctx, f.op1, token); e == nil {
		t.Fatal("invite reused")
	}
	if _, e = f.box.CreateInvitation(ctx, f.owner, f.org, "owner"); e == nil {
		t.Fatal("owner invitation")
	}
	if _, e = f.box.CreateInvitation(ctx, f.op1, f.org, "admin"); e == nil {
		t.Fatal("operator invitation")
	}
	for _, expired := range []bool{false, true} {
		v, e = f.box.CreateInvitation(ctx, f.owner, f.org, "operator")
		if e != nil {
			t.Fatal(e)
		}
		token = strings.Split(v.URL, "#invite=")[1]
		if expired {
			f.db.Exec(ctx, "UPDATE organization_invitations SET expires_at=now()-interval '1 second' WHERE id=$1", v.ID)
		} else {
			f.box.Revoke(ctx, f.owner, f.org, v.ID)
		}
		if _, e = f.box.Accept(ctx, f.op1, token); e == nil {
			t.Fatal("revoked/expired invitation")
		}
	}
	c := f.c(t, f.owner)
	c, e = f.box.Action(ctx, f.op1, f.org, f.id, "take", action(c))
	if e != nil {
		t.Fatal(e)
	}
	first, e := f.box.Page(ctx, f.owner, f.org, f.id, 0, 0)
	if e != nil {
		t.Fatal(e)
	}
	a := action(c)
	a.Content = "Cursor reply"
	m, e := f.box.Reply(ctx, f.op1, f.org, f.id, a)
	if e != nil {
		t.Fatal(e)
	}
	p, e := f.box.Page(ctx, f.owner, f.org, f.id, first.Cursor, 0)
	if e != nil || len(p.Messages) != 1 || p.Messages[0].ID != m.ID {
		t.Fatal("incremental", e, p)
	}
	empty, e := f.box.Page(ctx, f.owner, f.org, f.id, p.Cursor, 0)
	if e != nil || len(empty.Messages) != 0 {
		t.Fatal("duplicate poll", e)
	}
	// Simulated restart/recovery uses the existing persisted lease, not process memory.
	c, e = f.box.Action(ctx, f.owner, f.org, f.id, "resume-ai", action(c))
	if e != nil {
		t.Fatal(e)
	}
	scope := ai.Scope{OrganizationID: f.org, AssistantID: f.assistant, ConversationID: f.id}
	r, e := f.ai.Store.Begin(ctx, scope, ai.MessageInput{RequestID: core.ID(), Content: "Interrupted preview-style call"}, "en", "hash", "mock", "test", time.Second)
	if e == nil {
		t.Fatal("preview Begin accepted website", r)
	}
	pending := heldProvider{make(chan struct{}), make(chan struct{})}
	f.ai.Provider = pending
	done := make(chan error, 1)
	go func() {
		_, e := f.ai.WebsiteMessage(ctx, f.widget, f.session, f.token, "http://localhost:4173", ai.MessageInput{RequestID: core.ID(), Content: "Restart interrupted"}, true, func(ai.Event) error { return nil })
		done <- e
	}()
	select {
	case <-pending.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("generation not started")
	}
	if _, e = f.db.Exec(ctx, "UPDATE ai_executions SET expires_at=now()-interval '1 second' WHERE conversation_id=$1 AND status='processing'", f.id); e != nil {
		t.Fatal(e)
	}
	if e = (&ai.Store{DB: f.db}).Recover(ctx); e != nil {
		t.Fatal(e)
	}
	close(pending.release)
	if e = <-done; e == nil {
		t.Fatal("restarted stale execution completed")
	}
	var state string
	if e = f.db.QueryRow(ctx, "SELECT status FROM ai_executions WHERE conversation_id=$1 ORDER BY created_at DESC LIMIT 1", f.id).Scan(&state); e != nil || state != "failed" {
		t.Fatal("recovery", state, e)
	}
	w, e := f.widgets.Store.Public(ctx, f.widget)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		if _, e = f.widgets.Session(ctx, w, "http://localhost:4173"); e != nil {
			t.Fatal(e)
		}
	}
	seen := map[string]bool{}
	cursor := ""
	for {
		p, e := f.box.List(ctx, f.owner, f.org, inbox.Filter{Limit: 1, Cursor: cursor})
		if e != nil {
			t.Fatal(e)
		}
		for _, item := range p.Items {
			if seen[item.ID] {
				t.Fatal("duplicate keyset item")
			}
			seen[item.ID] = true
		}
		cursor = p.NextCursor
		if cursor == "" {
			break
		}
	}
	if len(seen) != 3 {
		t.Fatal("keyset pagination", len(seen))
	}
}

func TestDeliveryLockPrecedesConfirmedHandoff(t *testing.T) {
	f := setup(t)
	ctx := f.ctx
	c := f.c(t, f.owner)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	var switched atomic.Bool
	var leaks atomic.Int32
	done := make(chan error, 1)
	go func() {
		_, e := f.ai.WebsiteMessage(ctx, f.widget, f.session, f.token, "http://localhost:4173", ai.MessageInput{RequestID: core.ID(), Content: "Streaming race"}, true, func(ev ai.Event) error {
			if ev.Type == "delta" {
				if switched.Load() {
					leaks.Add(1)
				}
				once.Do(func() { close(entered); <-release })
			}
			return nil
		})
		done <- e
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("no delta")
	}
	took := make(chan error, 1)
	go func() {
		_, e := f.box.Action(ctx, f.op1, f.org, f.id, "take", action(c))
		if e == nil {
			switched.Store(true)
		}
		took <- e
	}()
	select {
	case e := <-took:
		t.Fatal("handoff confirmed while old flush held row lock", e)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	select {
	case e := <-took:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("handoff hung")
	}
	<-done
	if leaks.Load() != 0 {
		t.Fatal("post-confirmation deltas", leaks.Load())
	}
}
