package database

import (
	"context"
	"embed"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"sort"
	"strings"
)

// Migrate uses a transaction and advisory lock; a failed migration is never recorded.
func Migrate(ctx context.Context, p *pgxpool.Pool, fs embed.FS) error {
	tx, e := p.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(59113001)"); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())"); e != nil {
		return e
	}
	files, e := fs.ReadDir("migrations")
	if e != nil {
		return e
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".sql") {
			continue
		}
		var exists bool
		if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)", f.Name()).Scan(&exists); e != nil {
			return e
		}
		if exists {
			continue
		}
		b, e := fs.ReadFile("migrations/" + f.Name())
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, string(b)); e != nil {
			return fmt.Errorf("migration %s: %w", f.Name(), e)
		}
		if _, e = tx.Exec(ctx, "INSERT INTO schema_migrations(version) VALUES($1)", f.Name()); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
