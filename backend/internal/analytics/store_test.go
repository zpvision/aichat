package analytics

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/url"
	"os"
	"path/filepath"
	foundation "site-assistant"
	"site-assistant/internal/core"
	"site-assistant/internal/database"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAnalyticsLargeTenantAndReadConcurrency(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	u, e := url.Parse(dsn)
	if e != nil || !strings.HasSuffix(u.Path, "_test") {
		t.Fatal("test DB required")
	}
	ctx := context.Background()
	admin, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer admin.Close()
	schema := "analytics_" + strings.ReplaceAll(core.ID(), "-", "")
	if _, e = admin.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal(e)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	cfg, _ := pgxpool.ParseConfig(dsn)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	cfg.ConnConfig.RuntimeParams["jit"] = "off" // EXPLAIN must match Report's SET LOCAL.
	cfg.ConnConfig.RuntimeParams["enable_nestloop"] = "off"
	db, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if e = database.Migrate(ctx, db, foundation.Migrations); e != nil {
		t.Fatal(e)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := db.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	org, other, assistant, site, widget := core.ID(), core.ID(), core.ID(), core.ID(), core.ID()
	for _, id := range []string{org, other} {
		exec(`INSERT INTO organizations(id,name,country,timezone,default_locale) VALUES($1,'Synthetic analytics','CZ','UTC','en')`, id)
	}
	exec(`INSERT INTO assistants(id,organization_id,name,default_locale,language_policy) VALUES($1,$2,'Synthetic guide','en','visitor')`, assistant, org)
	exec(`INSERT INTO sites(id,organization_id,assistant_id,name,domain) VALUES($1,$2,$3,'Synthetic website','analytics.example')`, site, org, assistant)
	exec(`INSERT INTO widgets(id,organization_id,assistant_id,site_id,verification_token) VALUES($1,$2,$3,$4,'synthetic')`, widget, org, assistant, site)
	for i := 0; i < 2; i++ {
		a, s, w := core.ID(), core.ID(), core.ID()
		exec(`INSERT INTO assistants(id,organization_id,name,default_locale,language_policy) VALUES($1,$2,'Additional guide','en','visitor')`, a, org)
		exec(`INSERT INTO sites(id,organization_id,assistant_id,name,domain) VALUES($1,$2,$3,'Additional website',$4)`, s, org, a, s+".example")
		exec(`INSERT INTO widgets(id,organization_id,assistant_id,site_id,verification_token) VALUES($1,$2,$3,$4,'synthetic')`, w, org, a, s)
	}
	operator := core.ID()
	exec(`INSERT INTO users(id,email,password_hash,name,preferred_locale) VALUES($1,$2,'synthetic-not-login','Synthetic teammate','en')`, operator, operator+"@example.com")
	exec(`INSERT INTO organization_members(organization_id,user_id,role) VALUES($1,$2,'operator')`, org, operator)
	store := Store{DB: db}
	filter, _ := Parse(url.Values{}, time.Now())
	if _, e = store.Report(ctx, org, filter); e != nil {
		t.Fatal(e)
	}
	exec(`INSERT INTO widget_sessions(id,widget_id,organization_id,assistant_id,site_id,token_hash,origin,expires_at) SELECT gen_random_uuid(),w.id,w.organization_id,w.assistant_id,w.site_id,gen_random_uuid()::text,'http://localhost:4173',now()+interval '1 day' FROM widgets w CROSS JOIN generate_series(1,10000) WHERE w.organization_id=$1`, org)
	exec(`INSERT INTO conversations(id,organization_id,assistant_id,site_id,anonymous_session_id,kind,created_at) SELECT gen_random_uuid(),organization_id,assistant_id,site_id,id,'website',now()-(row_number() OVER()%45)*interval '1 day' FROM widget_sessions WHERE organization_id=$1`, org)
	exec(`INSERT INTO conversation_messages(id,organization_id,assistant_id,conversation_id,role,content,status,created_at) SELECT gen_random_uuid(),organization_id,assistant_id,id,'user','Synthetic question','completed',created_at FROM conversations WHERE organization_id=$1`, org)
	exec(`INSERT INTO ai_executions(id,organization_id,assistant_id,conversation_id,request_id,request_hash,provider,model,status,expires_at,created_at,provider_duration_ns) SELECT gen_random_uuid(),organization_id,assistant_id,id,gen_random_uuid(),'synthetic','mock','synthetic','completed',now()+interval '1 day',created_at,NULL FROM conversations WHERE organization_id=$1 LIMIT 6000`, org)
	exec(`INSERT INTO conversation_events(id,organization_id,conversation_id,kind,request_id,request_hash,created_at) SELECT gen_random_uuid(),organization_id,id,'human_requested',gen_random_uuid(),'synthetic',created_at+interval '4 seconds' FROM conversations WHERE organization_id=$1 LIMIT 1500`, org)
	exec(`INSERT INTO conversation_events(id,organization_id,conversation_id,kind,actor_id,request_id,request_hash,created_at) SELECT gen_random_uuid(),organization_id,conversation_id,'human_active',$2,gen_random_uuid(),'synthetic',created_at+interval '1 second' FROM conversation_events WHERE organization_id=$1 AND kind='human_requested'`, org, operator)
	exec(`INSERT INTO conversation_messages(id,organization_id,assistant_id,conversation_id,role,author_user_id,content,status,created_at) SELECT gen_random_uuid(),c.organization_id,c.assistant_id,c.id,'operator',$2,'Synthetic reply','completed',e.created_at+interval '2 seconds' FROM conversations c JOIN conversation_events e ON e.conversation_id=c.id AND e.organization_id=c.organization_id WHERE c.organization_id=$1 AND e.kind='human_active'`, org, operator)
	exec(`UPDATE conversations SET control_mode='human_active',assigned_user_id=$2 WHERE organization_id=$1 AND id IN(SELECT conversation_id FROM conversation_events WHERE organization_id=$1 AND kind='human_active')`, org, operator)
	measurements := []map[string]any{}
	for _, days := range []int{7, 30} {
		now := time.Now().UTC()
		filter, _ = Parse(url.Values{"from": {now.AddDate(0, 0, 1-days).Format(time.DateOnly)}, "to": {now.Format(time.DateOnly)}, "timezone": {"UTC"}}, now)
		started := time.Now()
		data, e := store.Report(ctx, org, filter)
		if e != nil {
			t.Fatal(e)
		}
		elapsed := time.Since(started).Milliseconds()
		var report map[string]any
		json.Unmarshal(data, &report)
		k := report["kpis"].(map[string]any)["current"].(map[string]any)
		if k["dialogs"].(float64) < 4000 {
			t.Fatal(k)
		}
		var plan []byte
		e = db.QueryRow(ctx, "EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON) "+reportSQL, org, filter.Start, filter.End, filter.Previous, filter.Zone, "", "", 0, "dialogs").Scan(&plan)
		if e != nil {
			t.Fatal(e)
		}
		measurements = append(measurements, map[string]any{"days": days, "report_ms": elapsed, "conversations": k["dialogs"], "explain": json.RawMessage(plan)})
		t.Logf("30000 conversations, %d days: %d ms", days, elapsed)
	}
	// Holding a writer row lock does not block analytics' ordinary MVCC reads.
	tx, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT id FROM conversations WHERE organization_id=$1 LIMIT 1 FOR UPDATE`, org); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	started := time.Now()
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := store.Report(ctx, org, filter); errs <- e }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	t.Logf("8 concurrent reports while writer holds row: %v", time.Since(started))
	measurements = append(measurements, map[string]any{"concurrent": 8, "wall_ms": time.Since(started).Milliseconds(), "writer_lock_held": true})
	otherData, e := store.Report(ctx, other, filter)
	if e != nil {
		t.Fatal(e)
	}
	var empty struct {
		Kpis map[string]struct{ Dialogs int }
	}
	json.Unmarshal(otherData, &empty)
	if empty.Kpis["current"].Dialogs != 0 {
		t.Fatal("tenant mixing")
	}
	if path := os.Getenv("ANALYTICS_REPORT_PATH"); path != "" {
		if !filepath.IsAbs(path) {
			t.Fatal("absolute report path required")
		}
		b, _ := json.MarshalIndent(measurements, "", "  ")
		if e = os.WriteFile(path, b, 0600); e != nil {
			t.Fatal(e)
		}
	}
}
