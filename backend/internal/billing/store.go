package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"strconv"
)

func TrialDays() (int, error) {
	v := os.Getenv("BILLING_TRIAL_DAYS")
	if v == "" {
		return 14, nil
	}
	n, e := strconv.Atoi(v)
	if e != nil || n < 1 || n > 90 {
		return 0, fmt.Errorf("invalid BILLING_TRIAL_DAYS")
	}
	return n, nil
}
func Configure(ctx context.Context, db *pgxpool.Pool, region string, days int) error {
	if e := CheckRegion(ctx, db, region); e != nil {
		return e
	}
	_, e := db.Exec(ctx, "UPDATE billing_settings SET trial_days=$1", days)
	return e
}
func CheckRegion(ctx context.Context, db *pgxpool.Pool, region string) error {
	var actual string
	if e := db.QueryRow(ctx, "SELECT region FROM billing_settings").Scan(&actual); e != nil {
		return e
	}
	if actual != region {
		return fmt.Errorf("billing region mismatch")
	}
	return nil
}
func Reserve(ctx context.Context, db *pgxpool.Pool, org, execution string) error {
	_, e := db.Exec(ctx, "SELECT billing_reserve($1,$2)", org, execution)
	return e
}

func Inspect(ctx context.Context, db *pgxpool.Pool, org, expectedName string) (json.RawMessage, error) {
	if !AuthorizedAdmin() {
		return nil, fmt.Errorf("billing administrative authorization required")
	}
	var actual string
	if e := db.QueryRow(ctx, "SELECT name FROM organizations WHERE id=$1", org).Scan(&actual); e != nil || actual != expectedName {
		return nil, fmt.Errorf("organization mismatch")
	}
	return Snapshot(ctx, db, org)
}
func ReleaseFallback(ctx context.Context, db *pgxpool.Pool, org, execution string) error {
	tx, e := db.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var conversation string
	if e = tx.QueryRow(ctx, "SELECT conversation_id FROM ai_executions WHERE id=$1 AND organization_id=$2", execution, org).Scan(&conversation); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "SELECT id FROM conversations WHERE id=$1 AND organization_id=$2 FOR UPDATE", conversation, org); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "SELECT id FROM ai_executions WHERE id=$1 AND organization_id=$2 FOR UPDATE", execution, org); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "SELECT organization_id FROM billing_subscriptions WHERE organization_id=$1 FOR UPDATE", org); e != nil {
		return e
	}
	var period, bucket string
	e = tx.QueryRow(ctx, "SELECT period_id,bucket FROM billing_reservations WHERE execution_id=$1 AND organization_id=$2 AND state='reserved' FOR UPDATE", execution, org).Scan(&period, &bucket)
	if e == nil {
		if _, e = tx.Exec(ctx, "UPDATE billing_periods SET ai_reserved=ai_reserved-CASE WHEN $2='ai' THEN 1 ELSE 0 END,demo_reserved=demo_reserved-CASE WHEN $2='demo' THEN 1 ELSE 0 END WHERE id=$1", period, bucket); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "UPDATE billing_reservations SET state='released',settled_at=now() WHERE execution_id=$1", execution); e != nil {
			return e
		}
	} else if e != pgx.ErrNoRows {
		return e
	}
	if _, e = tx.Exec(ctx, "UPDATE ai_executions SET provider_invoked=false WHERE id=$1 AND organization_id=$2 AND status='processing'", execution, org); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func Snapshot(ctx context.Context, db *pgxpool.Pool, org string) (json.RawMessage, error) {
	tx, e := db.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var period string
	if e = tx.QueryRow(ctx, "SELECT billing_ensure($1)", org).Scan(&period); e != nil {
		return nil, e
	}
	var b []byte
	e = tx.QueryRow(ctx, `SELECT jsonb_build_object('subscription',to_jsonb(s)-'resources','plan',to_jsonb(p)-'assistants'-'sites'-'members'-'knowledge_sources'-'knowledge_bytes'-'ai_responses'-'demo_responses'-'ai_concurrent'-'api_per_minute','period',to_jsonb(per),'resources',s.resources,'uncounted_files',(SELECT count(*) FROM knowledge_sources k WHERE k.organization_id=s.organization_id AND k.kind='document' AND NOT EXISTS(SELECT 1 FROM billing_files f WHERE f.file_key=k.file_key AND f.organization_id=k.organization_id)),'technical',(SELECT jsonb_build_object('executions',count(*),'input_tokens',sum(input_tokens),'output_tokens',sum(output_tokens),'input_token_reports',count(input_tokens),'output_token_reports',count(output_tokens)) FROM ai_executions e WHERE e.organization_id=s.organization_id AND e.created_at>=per.starts_at AND e.created_at<per.ends_at),'periods',COALESCE((SELECT jsonb_agg(to_jsonb(x) ORDER BY x.starts_at DESC) FROM(SELECT * FROM billing_periods WHERE organization_id=$1 ORDER BY starts_at DESC LIMIT 24)x),'[]')) FROM billing_subscriptions s JOIN billing_plans p ON p.id=s.plan_id JOIN billing_periods per ON per.id=$2 WHERE s.organization_id=$1`, org, period).Scan(&b)
	if e != nil {
		return nil, e
	}
	return json.RawMessage(b), tx.Commit(ctx)
}

// APICap participates in the existing key/company rate transaction. Abuse caps stay separate.
func APICap(ctx context.Context, tx pgx.Tx, org string) (*int64, error) {
	var period string
	if e := tx.QueryRow(ctx, "SELECT billing_ensure($1)", org).Scan(&period); e != nil {
		return nil, e
	}
	var cap *int64
	e := tx.QueryRow(ctx, "SELECT (limits->>'api_per_minute')::bigint FROM billing_periods WHERE id=$1", period).Scan(&cap)
	return cap, e
}
