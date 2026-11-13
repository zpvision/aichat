package billing

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"site-assistant/internal/core"
)

func ReserveFile(ctx context.Context, db *pgxpool.Pool, org, id string, size int64) error {
	if size < 0 {
		return fmt.Errorf("invalid storage size")
	}
	tx, e := db.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var period string
	if e = tx.QueryRow(ctx, "SELECT billing_ensure($1)", org).Scan(&period); e != nil {
		return e
	}
	var cap *int64
	var used, reserved int64
	var complete bool
	e = tx.QueryRow(ctx, `SELECT (p.limits->>'knowledge_bytes')::bigint,(s.resources->>'knowledge_bytes')::bigint,(s.resources->>'reserved_bytes')::bigint,NOT EXISTS(SELECT 1 FROM knowledge_sources k WHERE k.organization_id=$1 AND k.kind='document' AND NOT EXISTS(SELECT 1 FROM billing_files f WHERE f.organization_id=$1 AND f.file_key=k.file_key)) FROM billing_subscriptions s JOIN billing_periods p ON p.id=$2 WHERE s.organization_id=$1`, org, period).Scan(&cap, &used, &reserved, &complete)
	if e != nil {
		return e
	}
	if !complete {
		return core.Fail("storage_accounting_incomplete", 409)
	}
	if cap != nil && (size > *cap || used+reserved > *cap-size) {
		return core.Fail("plan_limit_exceeded", 409)
	}
	if _, e = tx.Exec(ctx, "INSERT INTO billing_files(id,organization_id,file_key,bytes,state) VALUES($1::uuid,$2,($1::uuid)::text,$3,'reserved')", id, org, size); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "UPDATE billing_subscriptions SET resources=jsonb_set(resources,'{reserved_bytes}',to_jsonb((resources->>'reserved_bytes')::bigint+$2)) WHERE organization_id=$1", org, size); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func ConfirmDeleted(ctx context.Context, db *pgxpool.Pool, org, key string) error {
	tx, e := db.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SELECT organization_id FROM billing_subscriptions WHERE organization_id=$1 FOR UPDATE", org); e != nil {
		return e
	}
	var used, reserved int64
	e = tx.QueryRow(ctx, "SELECT COALESCE(sum(bytes) FILTER(WHERE state='deleting' AND accounting_bucket='used'),0),COALESCE(sum(bytes) FILTER(WHERE state IN ('deleting','reserved') AND accounting_bucket='reserved'),0) FROM billing_files WHERE organization_id=$1 AND file_key=$2", org, key).Scan(&used, &reserved)
	if e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "UPDATE billing_subscriptions SET resources=jsonb_set(jsonb_set(resources,'{reserved_bytes}',to_jsonb((resources->>'reserved_bytes')::bigint-$3)),'{knowledge_bytes}',to_jsonb((resources->>'knowledge_bytes')::bigint-$2)) WHERE organization_id=$1", org, used, reserved); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "UPDATE billing_files SET state='deleted' WHERE organization_id=$1 AND file_key=$2 AND state IN ('reserved','deleting')", org, key); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func RegisterExisting(ctx context.Context, db *pgxpool.Pool, org, id, key string, size int64) error {
	tx, e := db.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SELECT organization_id FROM billing_subscriptions WHERE organization_id=$1 FOR UPDATE", org); e != nil {
		return e
	}
	tag, e := tx.Exec(ctx, "INSERT INTO billing_files(id,organization_id,file_key,bytes,state,accounting_bucket) SELECT id,organization_id,file_key,$4,'attached','used' FROM knowledge_sources WHERE organization_id=$1 AND id=$2 AND file_key=$3 ON CONFLICT DO NOTHING", org, id, key, size)
	if e != nil {
		return e
	}
	if tag.RowsAffected() > 0 {
		if _, e = tx.Exec(ctx, "UPDATE billing_subscriptions SET resources=jsonb_set(resources,'{knowledge_bytes}',to_jsonb((resources->>'knowledge_bytes')::bigint+$2)) WHERE organization_id=$1", org, size); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}

type FileRef struct{ Org, ID, Key string }

func UnregisteredFiles(ctx context.Context, db *pgxpool.Pool, after string) ([]FileRef, error) {
	rows, e := db.Query(ctx, "SELECT k.organization_id,k.id,k.file_key FROM knowledge_sources k WHERE k.kind='document' AND ($1='' OR k.id>NULLIF($1,'')::uuid) AND NOT EXISTS(SELECT 1 FROM billing_files f WHERE f.organization_id=k.organization_id AND f.file_key=k.file_key) ORDER BY k.id LIMIT 1000", after)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []FileRef{}
	for rows.Next() {
		var f FileRef
		if e = rows.Scan(&f.Org, &f.ID, &f.Key); e != nil {
			return nil, e
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
func AbandonFile(ctx context.Context, db *pgxpool.Pool, org, key string) (bool, error) {
	tag, e := db.Exec(ctx, "UPDATE billing_files SET state='deleting',lease_until=now() WHERE organization_id=$1 AND file_key=$2 AND state='reserved'", org, key)
	return tag.RowsAffected() > 0, e
}
func ClaimCleanupFile(ctx context.Context, db *pgxpool.Pool) (FileRef, error) {
	f := FileRef{}
	e := db.QueryRow(ctx, "WITH next AS(SELECT id FROM billing_files WHERE state IN ('reserved','deleting') AND lease_until<now() ORDER BY lease_until FOR UPDATE SKIP LOCKED LIMIT 1) UPDATE billing_files f SET state='deleting',lease_until=now()+interval '1 minute' FROM next WHERE f.id=next.id RETURNING f.organization_id,f.id,f.file_key").Scan(&f.Org, &f.ID, &f.Key)
	return f, e
}
