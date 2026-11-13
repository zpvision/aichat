package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"os/user"
	"site-assistant/internal/billing"
	"site-assistant/internal/config"
	"time"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "subscription command failed:", e)
		os.Exit(1)
	}
}
func run() error {
	action := flag.String("action", "inspect", "inspect, grant, change-plan, extend, suspend")
	org := flag.String("organization", "", "explicit organization UUID")
	name := flag.String("expect-name", "", "exact organization name")
	plan := flag.String("plan", "", "trial, start or business")
	days := flag.Int("days", 30, "activation/extension days")
	request := flag.String("request-id", "", "unique operation UUID, reuse for identical retries")
	flag.Parse()
	if !billing.AuthorizedAdmin() {
		return fmt.Errorf("billing administrative authorization required")
	}
	if *org == "" || *name == "" {
		return fmt.Errorf("organization and expect-name are required")
	}
	c, e := config.Load()
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, e := pgxpool.New(ctx, c.DatabaseURL)
	if e != nil {
		return fmt.Errorf("invalid database configuration")
	}
	defer db.Close()
	if e = billing.CheckRegion(ctx, db, c.Region); e != nil {
		return fmt.Errorf("billing configuration or region mismatch")
	}
	actor, e := user.Current()
	if e != nil {
		return fmt.Errorf("OS user unavailable")
	}
	var out json.RawMessage
	if *action == "inspect" {
		out, e = billing.Inspect(ctx, db, *org, *name)
	} else {
		out, e = billing.Admin(ctx, db, billing.AdminInput{OrganizationID: *org, ExpectedName: *name, Plan: *plan, Days: *days, Action: *action, RequestID: *request, Actor: actor.Username})
	}
	if e != nil {
		return fmt.Errorf("operation rejected; verify organization, plan and request ID")
	}
	return json.NewEncoder(os.Stdout).Encode(out)
}
