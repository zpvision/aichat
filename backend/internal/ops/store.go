package ops

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type Store struct{ DB *pgxpool.Pool }

func(s Store) HasObjectRegistry(ctx context.Context)(bool,error){var v bool;e:=s.DB.QueryRow(ctx,"SELECT to_regclass('object_files') IS NOT NULL").Scan(&v);return v,e}

func RetentionDays() (int, error) {
	v := os.Getenv("RETENTION_SESSION_DAYS")
	if v == "" {
		return 7, nil
	}
	n, e := strconv.Atoi(v)
	if e != nil || n < 1 || n > 3650 {
		return 0, fmt.Errorf("invalid RETENTION_SESSION_DAYS")
	}
	return n, nil
}

func Writable(root string) error {
	if root == "" {
		return fmt.Errorf("storage root required")
	}
	if e := os.MkdirAll(root, 0700); e != nil {
		return fmt.Errorf("storage unavailable")
	}
	f, e := os.CreateTemp(root, ".probe-")
	if e != nil {
		return fmt.Errorf("storage not writable")
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

type Original struct {
	Organization string `json:"organization_id"`
	ID           string `json:"source_id"`
	State        string `json:"state"`
}

func (s Store) OriginalsWith(ctx context.Context, size func(context.Context, string) (int64, error)) ([]Original, error) {
	rows, e := s.DB.Query(ctx, "SELECT organization_id,id,file_key FROM knowledge_sources WHERE kind='document' ORDER BY organization_id,id")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Original{}
	for rows.Next() {
		var v Original
		var key string
		if e = rows.Scan(&v.Organization, &v.ID, &key); e != nil {
			return nil, e
		}
		v.State = "present"
		if _, e = size(ctx, key); e != nil {
			v.State = "missing_or_unavailable"
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s Store) Originals(ctx context.Context, root string) ([]Original, error) {
	rows, e := s.DB.Query(ctx, "SELECT organization_id,id,file_key FROM knowledge_sources WHERE kind='document' ORDER BY organization_id,id")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Original{}
	for rows.Next() {
		var v Original
		var key string
		if e = rows.Scan(&v.Organization, &v.ID, &key); e != nil {
			return nil, e
		}
		v.State = "present"
		if filepath.Base(key) != key || len(key) != 36 {
			v.State = "invalid_key"
		} else {
			info, err := os.Lstat(filepath.Join(root, key))
			if err != nil {
				v.State = "missing_or_unreadable"
			} else if !info.Mode().IsRegular() {
				v.State = "not_regular"
			}
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Cleanup never deletes visitor sessions: their FK cascades into customer conversations.
func (s Store) Cleanup(ctx context.Context, days int, apply bool) (int64, error) {
	if days < 1 || days > 3650 {
		return 0, fmt.Errorf("invalid retention days")
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return 0, e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SET LOCAL statement_timeout='3s'"); e != nil {
		return 0, e
	}
	var n int64
	if !apply {
		e = tx.QueryRow(ctx, "SELECT count(*) FROM (SELECT token_hash FROM sessions WHERE expires_at<now()-$1*interval '1 day' LIMIT 500)x", days).Scan(&n)
	} else {
		tag, err := tx.Exec(ctx, "DELETE FROM sessions WHERE token_hash IN (SELECT token_hash FROM sessions WHERE expires_at<now()-$1*interval '1 day' LIMIT 500 FOR UPDATE SKIP LOCKED)", days)
		e = err
		n = tag.RowsAffected()
	}
	if e != nil {
		return 0, e
	}
	return n, tx.Commit(ctx)
}
func (s Store) Metrics(ctx context.Context) (map[string]int64, error) {
	out := map[string]int64{}
	for name, q := range map[string]string{
		"ai_active":                  "SELECT count(*) FROM ai_executions WHERE status='processing'",
		"ai_failures":                "SELECT count(*) FROM ai_executions WHERE status='failed'",
		"knowledge_pending":          "SELECT count(*) FROM knowledge_jobs WHERE state='pending'",
		"knowledge_failed":           "SELECT count(*) FROM knowledge_jobs WHERE state='failed'",
		"knowledge_stale":            "SELECT count(*) FROM knowledge_jobs WHERE state='processing' AND lease_until<now()",
		"webhook_pending":            "SELECT count(*) FROM webhook_deliveries WHERE status IN ('pending','processing')",
		"webhook_failed":             "SELECT count(*) FROM webhook_deliveries WHERE status='failed'",
		"webhook_stale":              "SELECT count(*) FROM webhook_deliveries WHERE status='processing' AND lease_until<now()",
		"quota_reserved":             "SELECT count(*) FROM billing_reservations WHERE state='reserved'",
		"postgres_connections":       "SELECT count(*) FROM pg_stat_activity WHERE datname=current_database()",
		"postgres_long_transactions": "SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND xact_start<now()-interval '10 seconds'",
	} {
		var n int64
		if e := s.DB.QueryRow(ctx, q).Scan(&n); e != nil {
			return nil, fmt.Errorf("metrics unavailable")
		}
		out[name] = n
	}
	return out, nil
}
func Context() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}
