package leads_test

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
	"site-assistant/internal/leads"
	"site-assistant/internal/widget"
	"strings"

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

func (f *fixture) settings(t *testing.T) leads.Settings {
	t.Helper()
	s, e := f.widgets.Leads.Settings(f.ctx, f.owner, f.org, f.assistant)
	if e != nil {
		t.Fatal(e)
	}
	s.Enabled = true
	s.Fields[0].Required = true
	v, e := f.widgets.Leads.SaveSettings(f.ctx, f.owner, f.org, f.assistant, s)
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func (f *fixture) view(t *testing.T) leads.View {
	t.Helper()
	v, e := f.widgets.Leads.VisitorView(f.ctx, f.widget, f.session, f.token, "http://localhost:4173")
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func input(v leads.View) leads.Input {
	return leads.Input{Version: v.Version, ConversationVersion: v.ConversationVersion}
}
func (f *fixture) mutate(t *testing.T, kind string, v leads.View, values map[string]json.RawMessage) leads.View {
	t.Helper()
	in := input(v)
	in.Values = values
	out, e := f.widgets.Leads.VisitorAction(f.ctx, f.widget, f.session, f.token, "http://localhost:4173", kind, in)
	if e != nil {
		t.Fatal(kind, e)
	}
	return out
}
func (f *fixture) message(t *testing.T, body string) ai.Result {
	t.Helper()
	r, e := f.ai.WebsiteMessage(f.ctx, f.widget, f.session, f.token, "http://localhost:4173", ai.MessageInput{RequestID: core.ID(), Content: body}, false, nil)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func TestCollectionWorkflowSnapshotAndConcurrentConfirmation(t *testing.T) {
	f := setup(t)
	s := f.settings(t)
	f.widgets.Leads.Extractor = &leads.MockStructuredExtractor{Error: errors.New("must not run in consultation")}
	f.message(t, "What are your services?")
	if v := f.view(t); v.State != "idle" {
		t.Fatal(v.State)
	}
	f.widgets.Leads.Extractor = &leads.MockStructuredExtractor{}
	v := f.mutate(t, "start", f.view(t), nil)
	f.message(t, `{"full_name":"Anna"}`)
	v = f.view(t)
	if v.State != "collecting" || !v.Values["full_name"].Valid {
		t.Fatal(v)
	}
	if _, e := f.widgets.Leads.VisitorAction(f.ctx, f.widget, f.session, f.token, "http://localhost:4173", "submit", input(v)); e == nil {
		t.Fatal("incomplete submission")
	}
	s.Fields[0].Required = false
	s.Fields[3].Required = true
	if _, e := f.widgets.Leads.SaveSettings(f.ctx, f.owner, f.org, f.assistant, s); e != nil {
		t.Fatal(e)
	}
	r := f.message(t, `{"email":"anna@example.com"}`)
	v = f.view(t)
	if v.State != "confirmation" || len(v.Missing) != 0 {
		t.Fatal("snapshot changed", v)
	}
	if _, e := f.ai.WebsiteMessage(f.ctx, f.widget, f.session, f.token, "http://localhost:4173", ai.MessageInput{RequestID: r.UserMessage.RequestID, Content: r.UserMessage.Content}, false, nil); e != nil {
		t.Fatal(e)
	}
	if f.view(t).Version != v.Version {
		t.Fatal("replayed extraction changed version")
	}
	errch := make(chan error, 16)
	for i := 0; i < 16; i++ {
		go func() {
			_, e := f.widgets.Leads.VisitorAction(f.ctx, f.widget, f.session, f.token, "http://localhost:4173", "submit", input(v))
			errch <- e
		}()
	}
	for i := 0; i < 16; i++ {
		if e := <-errch; e != nil {
			t.Fatal(e)
		}
	}
	var count int
	f.db.QueryRow(f.ctx, "SELECT count(*) FROM leads WHERE organization_id=$1", f.org).Scan(&count)
	if count != 1 {
		t.Fatal("duplicate leads", count)
	}
	public := f.view(t)
	if public.State != "submitted" || public.LeadID != nil || len(public.Values) != 0 {
		t.Fatal("public submission/PII duplication", public)
	}
	staff, e := f.widgets.Leads.StaffView(f.ctx, f.owner, f.org, f.id)
	if e != nil || staff.LeadID == nil {
		t.Fatal(staff, e)
	}
	d, e := f.widgets.Leads.Detail(f.ctx, f.owner, f.org, *staff.LeadID)
	if e != nil || d.Lead.Email != "anna@example.com" || d.Lead.Name != "Anna" || len(d.Events) != 1 {
		t.Fatal(d, e)
	}
	if len(d.Lead.Data) != 0 || d.Lead.Provenance["email"].MessageID == "" || string(d.Lead.Provenance["email"].Value) != "null" {
		t.Fatal("contact provenance/duplicate", d)
	}
	if _, e = f.widgets.Leads.Detail(f.ctx, f.outsider, f.org, d.Lead.ID); e == nil {
		t.Fatal("foreign tenant read")
	}
	if _, e = f.widgets.Leads.VisitorView(f.ctx, f.widget, f.session, "wrong-token", "http://localhost:4173"); e == nil {
		t.Fatal("foreign visitor read")
	}
	if _, e = f.widgets.Leads.Settings(f.ctx, f.op1, f.org, f.assistant); e == nil {
		t.Fatal("operator settings")
	}
	me := f.op1
	d, e = f.widgets.Leads.Patch(f.ctx, f.op1, f.org, d.Lead.ID, leads.LeadPatch{Version: d.Lead.Version, Assigned: &me})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.widgets.Leads.Detail(f.ctx, f.op2, f.org, d.Lead.ID); e == nil {
		t.Fatal("other operator access")
	}
	st, note := "in_progress", "Private CRM note"
	d, e = f.widgets.Leads.Patch(f.ctx, f.op1, f.org, d.Lead.ID, leads.LeadPatch{Version: d.Lead.Version, Status: &st, Note: &note})
	if e != nil || len(d.Events) != 4 {
		t.Fatal("audit each action", d, e)
	}
	if _, e = f.widgets.Leads.Patch(f.ctx, f.op1, f.org, d.Lead.ID, leads.LeadPatch{Version: d.Lead.Version, Values: map[string]json.RawMessage{"email": json.RawMessage(`"other@example.com"`)}}); e == nil {
		t.Fatal("operator contact edit")
	}
	list, e := f.widgets.Leads.List(f.ctx, f.owner, f.org, leads.Filter{Status: "in_progress", Limit: 1})
	if e != nil || len(list["items"].([]leads.Lead)) != 1 {
		t.Fatal(list, e)
	}
	if e = f.widgets.Leads.Delete(f.ctx, f.op1, f.org, d.Lead.ID); e == nil {
		t.Fatal("operator deletion")
	}
	if e = f.widgets.Leads.Delete(f.ctx, f.owner, f.org, d.Lead.ID); e != nil {
		t.Fatal(e)
	}
	_, e = f.widgets.Leads.VisitorAction(f.ctx, f.widget, f.session, f.token, "http://localhost:4173", "submit", input(v))
	fault(t, e, "lead_deleted")
}
func TestConfirmationRollbackAndStaleData(t *testing.T) {
	f := setup(t)
	f.settings(t)
	v := f.mutate(t, "start", f.view(t), nil)
	v = f.mutate(t, "edit", v, map[string]json.RawMessage{"full_name": json.RawMessage(`"Ivan"`), "email": json.RawMessage(`"a@example.com"`)})
	old := v
	v = f.mutate(t, "edit", v, map[string]json.RawMessage{"email": json.RawMessage(`"b@example.com"`)})
	_, e := f.widgets.Leads.VisitorAction(f.ctx, f.widget, f.session, f.token, "http://localhost:4173", "submit", input(old))
	fault(t, e, "state_conflict")
	_, e = f.db.Exec(f.ctx, `CREATE FUNCTION fail_lead_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test database failure'; END $$; CREATE TRIGGER fail_lead_test BEFORE INSERT ON leads FOR EACH ROW EXECUTE FUNCTION fail_lead_test()`)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.widgets.Leads.VisitorAction(f.ctx, f.widget, f.session, f.token, "http://localhost:4173", "submit", input(v)); e == nil {
		t.Fatal("expected DB failure")
	}
	after := f.view(t)
	if after.Version != v.Version || after.State != "confirmation" || string(after.Values["email"].Value) != `"b@example.com"` {
		t.Fatal("data lost on failure", after)
	}
	if _, e = f.db.Exec(f.ctx, "DROP TRIGGER fail_lead_test ON leads; DROP FUNCTION fail_lead_test()"); e != nil {
		t.Fatal(e)
	}
	f.mutate(t, "submit", v, nil)
}

type heldExtractor struct{ entered, release chan struct{} }

func (h *heldExtractor) Extract(_ context.Context, in leads.ExtractInput) ([]leads.Candidate, error) {
	close(h.entered)
	<-h.release
	return (&leads.MockStructuredExtractor{}).Extract(context.Background(), in)
}
func TestHandoffFencesExtractionAndManualCompletion(t *testing.T) {
	f := setup(t)
	f.settings(t)
	f.mutate(t, "start", f.view(t), nil)
	h := &heldExtractor{make(chan struct{}), make(chan struct{})}
	f.widgets.Leads.Extractor = h
	done := make(chan error, 1)
	go func() {
		_, e := f.ai.WebsiteMessage(f.ctx, f.widget, f.session, f.token, "http://localhost:4173", ai.MessageInput{RequestID: core.ID(), Content: `{"full_name":"Anna","email":"anna@example.com"}`}, true, func(ai.Event) error { return nil })
		done <- e
	}()
	select {
	case <-h.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("extract not started")
	}
	c := f.c(t, f.op1)
	c, e := f.box.Action(f.ctx, f.op1, f.org, f.id, "take", action(c))
	if e != nil {
		t.Fatal(e)
	}
	close(h.release)
	if e = <-done; e == nil {
		t.Fatal("stale generation succeeded")
	}
	v := f.view(t)
	if !v.Paused || len(v.Values) != 0 {
		t.Fatal("canceled extraction published", v)
	}
	if _, e = f.widgets.Leads.VisitorAction(f.ctx, f.widget, f.session, f.token, "http://localhost:4173", "edit", input(v)); e == nil {
		t.Fatal("visitor edit while human active")
	}
	f.widgets.Leads.Extractor = &leads.MockStructuredExtractor{Error: errors.New("must not run with human")}
	f.message(t, "anna@example.com")
	if len(f.view(t).Values) != 0 {
		t.Fatal("automatic collection during handoff")
	}
	in := input(v)
	in.Values = map[string]json.RawMessage{"full_name": json.RawMessage(`"Anna"`), "email": json.RawMessage(`"anna@example.com"`)}
	v, e = f.widgets.Leads.StaffAction(f.ctx, f.op1, f.org, f.id, "edit", in)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.widgets.Leads.StaffAction(f.ctx, f.op2, f.org, f.id, "submit", input(v)); e == nil {
		t.Fatal("foreign operator submit")
	}
	v, e = f.widgets.Leads.StaffAction(f.ctx, f.op1, f.org, f.id, "submit", input(v))
	if e != nil || v.LeadID == nil {
		t.Fatal(e, v)
	}
	d, e := f.widgets.Leads.Detail(f.ctx, f.op1, f.org, *v.LeadID)
	if e != nil || d.Lead.Source != "staff" || d.Lead.Assigned == nil || *d.Lead.Assigned != f.op1 {
		t.Fatal(d, e)
	}
	if _, e = f.box.Action(f.ctx, f.op1, f.org, f.id, "resume-ai", action(c)); e != nil {
		t.Fatal(e)
	}
}
func TestExtractionErrorsAndCorrections(t *testing.T) {
	f := setup(t)
	f.settings(t)
	f.mutate(t, "start", f.view(t), nil)
	f.message(t, `{"full_name":"Anna","email":"bad@","phone":"123","inn":"123"}`)
	v := f.view(t)
	if v.State != "collecting" || v.Values["email"].Valid {
		t.Fatal(v)
	}
	f.message(t, `{"email":"anna@example.com","phone":"+420777123456"}`)
	v = f.view(t)
	if v.State != "confirmation" {
		t.Fatal(v)
	}
	f.widgets.Leads.Extractor = &leads.MockStructuredExtractor{Error: context.DeadlineExceeded}
	f.message(t, "update my details")
	v = f.view(t)
	if v.Error != "lead_extraction_timeout" || !v.Values["email"].Valid {
		t.Fatal("timeout data lost", v)
	}
	f.mutate(t, "edit", v, map[string]json.RawMessage{"email": json.RawMessage(`"new@example.com"`)})
	f.widgets.Leads.Extractor = &leads.MockStructuredExtractor{}
	f.message(t, "repeat earlier information")
	v = f.view(t)
	if string(v.Values["email"].Value) != `"new@example.com"` {
		t.Fatal("old value overwrote correction")
	}
	v = f.mutate(t, "cancel", v, nil)
	if len(v.Values) != 0 || v.State != "canceled" {
		t.Fatal(v)
	}
	v = f.mutate(t, "start", v, nil)
	if len(v.Values) != 0 || v.State != "collecting" {
		t.Fatal("restart", v)
	}
}
