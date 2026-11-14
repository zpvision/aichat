package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/url"
	"os"
	"site-assistant/internal/billing"
	"site-assistant/internal/config"
	"site-assistant/internal/knowledge"
	"site-assistant/internal/objectstorage"
	"strings"
	"time"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "storage command failed; verify configuration and access")
		os.Exit(1)
	}
}
func run() error {
	apply := flag.Bool("apply", false, "copy and switch verified objects; default dry-run")
	after := flag.String("after", "", "resume after UUID")
	purpose := flag.String("purpose", "knowledge", "knowledge or logos")
	root := flag.String("root", "", "legacy local directory; defaults to the purpose-specific environment/root")
	action := flag.String("action", "migrate", "migrate, export, restore-local, cleanup-orphans")
	dir := flag.String("directory", "", "private export/restore directory for this purpose")
	flag.Parse()
	if !billing.AuthorizedAdmin() {
		return fmt.Errorf("admin required")
	}
	if *purpose != "knowledge" && *purpose != "logos" {
		return fmt.Errorf("purpose")
	}
	if *root == "" {
		key, fallback := "KNOWLEDGE_FILE_ROOT", "./data/knowledge"
		if *purpose == "logos" {
			key, fallback = "WIDGET_FILE_ROOT", "./data/widget"
		}
		*root = os.Getenv(key)
		if *root == "" {
			*root = fallback
		}
	}
	c, e := config.LoadAdministrative()
	if e != nil {
		return e
	}
	sc := objectstorage.Config{Provider: "local"}
	if *action != "restore-local" {
		sc, e = objectstorage.Load()
		if e != nil || sc.Provider != "s3" {
			return fmt.Errorf("S3 required")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	db, e := pgxpool.New(ctx, c.DatabaseURL)
	if e != nil {
		return e
	}
	defer db.Close()
	s := knowledge.NewObjectStore(db, sc, *root, *purpose)
	if *action != "migrate" && *action != "export" && *action != "restore-local" && *action != "cleanup-orphans" {
		return fmt.Errorf("invalid action")
	}
	if *action == "cleanup-orphans" && *apply && os.Getenv("BACKUP_WRITERS_STOPPED") != "yes" {
		return fmt.Errorf("stop writers before orphan cleanup")
	}
	if *action == "export" || *action == "restore-local" {
		if *dir == "" || os.Getenv("BACKUP_WRITERS_STOPPED") != "yes" {
			return fmt.Errorf("offline directory required")
		}
	}
	if *action == "restore-local" {
		u, _ := url.Parse(c.DatabaseURL)
		if !strings.HasSuffix(u.Path, "_test") {
			return fmt.Errorf("restore requires isolated test DB")
		}
	}
	ids, e := s.Candidates(ctx, *after)
	if *action == "cleanup-orphans" {
		ids, e = s.Orphans(ctx)
	}
	if e != nil {
		return e
	}
	failed := false
	for _, id := range ids {
		v := knowledge.MigrationResult{ID: id, State: "verified"}
		var e error
		switch *action {
		case "migrate":
			v, e = s.MigrateOne(ctx, id, *apply)
		case "export":
			e = s.ExportOne(ctx, id, *dir)
		case "restore-local":
			e = s.RestoreLocalOne(ctx, id, *dir, *apply)
		case "cleanup-orphans":
			v.State = "would_delete_unreferenced"
			if *apply {
				e = s.Delete(ctx, id)
				v.State = "deleted_unreferenced"
			}
		}
		if e != nil {
			v.State = "failed"
			failed = true
		}
		json.NewEncoder(os.Stdout).Encode(v)
	}
	if failed {
		return fmt.Errorf("one or more objects failed; inspect safe per-object report")
	}
	return nil
}
