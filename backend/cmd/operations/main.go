package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"site-assistant/internal/billing"
	"site-assistant/internal/config"
	"site-assistant/internal/ops"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "operation failed; verify configuration and permissions")
		os.Exit(1)
	}
}
func run() error {
	action := flag.String("action", "originals", "originals, metrics, cleanup")
	apply := flag.Bool("apply", false, "apply bounded technical cleanup; default dry run")
	days := flag.Int("days", 7, "days after session expiry")
	root := flag.String("root", "./data/knowledge", "knowledge originals directory")
	flag.Parse()
	if !billing.AuthorizedAdmin() {
		return fmt.Errorf("admin required")
	}
	c, e := config.Load()
	if e != nil {
		return e
	}
	ctx, cancel := ops.Context()
	defer cancel()
	db, e := pgxpool.New(ctx, c.DatabaseURL)
	if e != nil {
		return e
	}
	defer db.Close()
	s := ops.Store{DB: db}
	var out any
	switch *action {
	case "originals":
		out, e = s.Originals(ctx, *root)
	case "metrics":
		out, e = s.Metrics(ctx)
	case "cleanup":
		var n int64
		n, e = s.Cleanup(ctx, *days, *apply)
		out = map[string]any{"dry_run": !*apply, "sessions": n}
	default:
		return fmt.Errorf("invalid action")
	}
	if e != nil {
		return e
	}
	return json.NewEncoder(os.Stdout).Encode(out)
}
