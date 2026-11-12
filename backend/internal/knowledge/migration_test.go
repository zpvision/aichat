package knowledge

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/url"
	"os"
	foundation "site-assistant"
	"site-assistant/internal/core"
	"site-assistant/internal/database"
	"strings"
	"testing"
)

func TestUpgradePreservesFoundation(t *testing.T) {
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
	schema := "upgrade_" + strings.ReplaceAll(core.ID(), "-", "")
	if _, e = admin.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal(e)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	cfg, _ := pgxpool.ParseConfig(dsn)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	p, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	sql, _ := foundation.Migrations.ReadFile("migrations/001_foundation.sql")
	if _, e = p.Exec(ctx, string(sql)); e != nil {
		t.Fatal(e)
	}
	if _, e = p.Exec(ctx, "CREATE TABLE schema_migrations(version text PRIMARY KEY,applied_at timestamptz NOT NULL DEFAULT now()); INSERT INTO schema_migrations(version) VALUES('001_foundation.sql')"); e != nil {
		t.Fatal(e)
	}
	id := core.ID()
	if _, e = p.Exec(ctx, "INSERT INTO users(id,email,password_hash,preferred_locale) VALUES($1,'existing@example.com','existing-hash','cs')", id); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		if e = database.Migrate(ctx, p, foundation.Migrations); e != nil {
			t.Fatal(e)
		}
	}
	var hash, language string
	if e = p.QueryRow(ctx, "SELECT password_hash,preferred_locale FROM users WHERE id=$1", id).Scan(&hash, &language); e != nil || hash != "existing-hash" || language != "cs" {
		t.Fatal("foundation data changed", e)
	}
}
