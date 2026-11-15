package billing_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"site-assistant/internal/ai"
	"site-assistant/internal/analytics"
	"site-assistant/internal/config"
	"site-assistant/internal/core"
	"site-assistant/internal/httpapi"
	"testing"
	"time"
)

func TestAnalyticsActualActivityAndSources(t *testing.T) {
	f := setup(t)
	filter, e := analytics.Parse(url.Values{}, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	s := analytics.Service{Auth: f.auth, Store: &analytics.Store{DB: f.db}}
	// Real generation is deterministic Mock; no provider-reported time is invented.
	_, e = f.ai.WebsiteMessage(f.ctx, f.widget, f.session, f.token, "http://localhost:4173", ai.MessageInput{RequestID: core.ID(), Content: "Please help"}, false, func(ai.Event) error { return nil })
	if e != nil {
		t.Fatal(e)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := f.db.Exec(f.ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	exec(`INSERT INTO lead_collections(id,organization_id,assistant_id,conversation_id,state,snapshot,privacy_url,submitted_at) VALUES($1,$2,$3,$4,'submitted','{}','',now())`, core.ID(), f.org, f.assistant, f.id)
	// An API lead must not raise website leads or the conversation numerator.
	exec(`INSERT INTO leads(id,organization_id,assistant_id,source,snapshot,privacy_url) VALUES($1,$2,$3,'api','{}','')`, core.ID(), f.org, f.assistant)
	for i := 0; i < 2; i++ {
		exec(`INSERT INTO conversation_events(id,organization_id,conversation_id,kind,actor_id,request_id,request_hash) VALUES($1,$2,$3,'human_active',$4,$5,'test')`, core.ID(), f.org, f.id, f.op1, core.ID())
	}
	exec(`INSERT INTO conversation_messages(id,organization_id,assistant_id,conversation_id,role,status,author_user_id,content) VALUES($1,$2,$3,$4,'operator','completed',$5,'synthetic')`, core.ID(), f.org, f.assistant, f.id, f.op2)
	report := func(fil analytics.Filter) map[string]any {
		t.Helper()
		b, e := s.Report(f.ctx, f.owner, f.org, fil)
		if e != nil {
			t.Fatal(e)
		}
		var v map[string]any
		if e = json.Unmarshal(b, &v); e != nil {
			t.Fatal(e)
		}
		return v
	}
	v := report(filter)
	k := v["kpis"].(map[string]any)["current"].(map[string]any)
	for key, want := range map[string]float64{"dialogs": 1, "leads": 0, "eligible": 1, "converted": 1, "without_operator": 0, "handoffs": 1} {
		if k[key] != want {
			t.Fatalf("%s: got %v want %v", key, k[key], want)
		}
	}
	if k["generation_ms"] != nil {
		t.Fatal("Mock duration must not be real generation time")
	}
	team := v["operators"].([]any)
	for _, item := range team {
		x := item.(map[string]any)
		if x["user_id"] == f.op1 && x["taken"] != float64(1) {
			t.Fatal(x)
		}
		if x["user_id"] == f.op2 && x["replies"] != float64(1) {
			t.Fatal(x)
		}
	}
	// Filter IDs do not grant cross-tenant access; foreign or absent IDs yield empty data.
	filter.Assistant = core.ID()
	empty := report(filter)["kpis"].(map[string]any)["current"].(map[string]any)
	if empty["dialogs"] != float64(0) {
		t.Fatal(empty)
	}
	if _, e = s.Report(f.ctx, f.outsider, f.org, filter); e == nil {
		t.Fatal("foreign member accessed analytics")
	}
}

func TestAnalyticsHTTPRolesAndValidation(t *testing.T) {
	f := setup(t)
	handler := httpapi.New(f.auth, config.Config{PublicURL: "http://localhost:5173"})
	for _, tc := range []struct {
		user, query string
		status      int
	}{{f.owner, "", 200}, {f.op1, "", 403}, {f.outsider, "", 404}, {"", "", 401}, {f.owner, "?from=2026-02-30&to=2026-03-01", 422}, {f.owner, "?page=-1", 422}} {
		r := httptest.NewRequest("GET", "/api/v1/organizations/"+f.org+"/analytics"+tc.query, nil)
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
			t.Fatalf("want %d got %d %s", tc.status, w.Code, w.Body.String())
		}
	}
	if _, e := f.db.Exec(f.ctx, `UPDATE organization_members SET role='admin' WHERE organization_id=$1 AND user_id=$2`, f.org, f.op1); e != nil {
		t.Fatal(e)
	}
	filter, _ := analytics.Parse(url.Values{}, time.Now())
	s := analytics.Service{Auth: f.auth, Store: &analytics.Store{DB: f.db}}
	if _, e := s.Report(f.ctx, f.op1, f.org, filter); e != nil {
		t.Fatal("admin denied", e)
	}
}

func TestAnalyticsSnapshot(t *testing.T) {
	f := setup(t)
	filter, e := analytics.Parse(url.Values{}, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	s := analytics.Service{Auth: f.auth, Store: &analytics.Store{DB: f.db}}
	data, e := s.Report(f.ctx, f.owner, f.org, filter)
	if e != nil {
		t.Fatal(e)
	}
	var v struct {
		Kpis map[string]struct {
			Dialogs    int
			Eligible   int
			Generation *float64 `json:"generation_ms"`
		}
		Timeseries []any
	}
	if e = json.Unmarshal(data, &v); e != nil {
		t.Fatal(e)
	}
	if v.Kpis["current"].Dialogs != 1 || v.Kpis["current"].Eligible != 0 || v.Kpis["current"].Generation != nil || len(v.Timeseries) != 7 {
		t.Fatal(string(data))
	}
	if _, e = s.Report(f.ctx, f.op1, f.org, filter); e == nil {
		t.Fatal("operator accessed company analytics")
	}
}

func TestAnalyticsPreviousBoundaryAndExecutionSemantics(t *testing.T) {
	f := setup(t)
	filter, _ := analytics.Parse(url.Values{}, time.Now())
	s := analytics.Service{Auth: f.auth, Store: &analytics.Store{DB: f.db}}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := f.db.Exec(f.ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	exec(`UPDATE conversations SET created_at=$2 WHERE id=$1`, f.id, filter.Start.Add(-time.Hour))
	exec(`INSERT INTO conversation_messages(id,organization_id,assistant_id,conversation_id,role,status,created_at) VALUES($1,$2,$3,$4,'user','completed',$5)`, core.ID(), f.org, f.assistant, f.id, filter.Start)
	for i, status := range []string{"completed", "failed", "canceled"} {
		exec(`INSERT INTO ai_executions(id,organization_id,assistant_id,conversation_id,request_id,request_hash,provider,model,status,provider_duration_ns,input_tokens,expires_at) VALUES($1,$2,$3,$4,$5,'synthetic','ollama','local-contract-fixture',$6,$7,$8,now()+interval '1 day')`, core.ID(), f.org, f.assistant, f.id, core.ID(), status, int64(i+1)*1500000000, 10)
	}
	b, e := s.Report(f.ctx, f.owner, f.org, filter)
	if e != nil {
		t.Fatal(e)
	}
	var v struct {
		Kpis map[string]struct {
			Dialogs, Eligible int
			Time              *float64 `json:"generation_ms"`
		}
		Quality    struct{ Failed, Canceled int }
		Executions []struct {
			Status string
			Output *int `json:"output_tokens"`
		}
	}
	if e = json.Unmarshal(b, &v); e != nil {
		t.Fatal(e)
	}
	if v.Kpis["previous"].Dialogs != 1 || v.Kpis["previous"].Eligible != 0 || v.Kpis["current"].Dialogs != 0 || v.Kpis["current"].Time == nil || *v.Kpis["current"].Time != 1500 || v.Quality.Failed != 1 || v.Quality.Canceled != 1 {
		t.Fatal(string(b))
	}
	for _, x := range v.Executions {
		if x.Output != nil {
			t.Fatal("unknown tokens became zero")
		}
	}
	exec(`UPDATE conversations SET created_at=$2 WHERE id=$1`, f.id, filter.End)
	b, e = s.Report(f.ctx, f.owner, f.org, filter)
	if e != nil {
		t.Fatal(e)
	}
	json.Unmarshal(b, &v)
	if v.Kpis["current"].Dialogs != 0 || v.Kpis["previous"].Dialogs != 0 {
		t.Fatal("exclusive end", string(b))
	}
}

func TestAnalyticsStaffConfirmationAndDeletedLead(t *testing.T) {
	f := setup(t)
	collection, lead := core.ID(), core.ID()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := f.db.Exec(f.ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	exec(`INSERT INTO conversation_messages(id,organization_id,assistant_id,conversation_id,role,status) VALUES($1,$2,$3,$4,'user','completed')`, core.ID(), f.org, f.assistant, f.id)
	exec(`INSERT INTO lead_collections(id,organization_id,assistant_id,conversation_id,state,snapshot,privacy_url,submitted_at) VALUES($1,$2,$3,$4,'submitted','{}','',now())`, collection, f.org, f.assistant, f.id)
	exec(`INSERT INTO leads(id,organization_id,assistant_id,site_id,conversation_id,collection_id,source,snapshot,privacy_url) SELECT $1,organization_id,assistant_id,site_id,id,$2,'staff','{}','' FROM conversations WHERE organization_id=$3 AND id=$4`, lead, collection, f.org, f.id)
	exec(`UPDATE lead_collections SET lead_id=$1 WHERE organization_id=$2 AND id=$3`, lead, f.org, collection)
	filter, _ := analytics.Parse(url.Values{}, time.Now())
	s := analytics.Service{Auth: f.auth, Store: &analytics.Store{DB: f.db}}
	check := func(want int) {
		t.Helper()
		b, e := s.Report(f.ctx, f.owner, f.org, filter)
		if e != nil {
			t.Fatal(e)
		}
		var v struct {
			Kpis map[string]struct{ Leads, Converted, Eligible int }
		}
		json.Unmarshal(b, &v)
		k := v.Kpis["current"]
		if k.Leads != want || k.Converted != 1 || k.Eligible != 1 {
			t.Fatal(string(b))
		}
	}
	check(1)
	exec(`DELETE FROM leads WHERE organization_id=$1 AND id=$2`, f.org, lead)
	check(0)
}
