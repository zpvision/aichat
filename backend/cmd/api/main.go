package main

import (
	"context"
	"errors"
	"flag"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	foundation "site-assistant"
	"site-assistant/internal/ai"
	"site-assistant/internal/billing"
	"site-assistant/internal/config"
	"site-assistant/internal/core"
	"site-assistant/internal/database"
	"site-assistant/internal/httpapi"
	"site-assistant/internal/integrations"
	"site-assistant/internal/knowledge"
	"site-assistant/internal/ops"
	"site-assistant/internal/widget"
	"syscall"
	"time"
	_ "time/tzdata"
)

func main() {
	if e := run(); e != nil {
		slog.Error("application stopped", "reason", e.Error())
		os.Exit(1)
	}
}
func run() error {
	migrate := flag.Bool("migrate", false, "apply database migrations and exit")
	flag.Parse()
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	c, e := config.Load()
	if e != nil {
		return e
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pc, e := pgxpool.ParseConfig(c.DatabaseURL)
	if e != nil {
		return errors.New("invalid database connection configuration")
	}
	pc.ConnConfig.RuntimeParams["app.region"] = c.Region
	pc.MaxConns = 20
	pc.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "30s"
	pool, e := pgxpool.NewWithConfig(ctx, pc)
	if e != nil {
		return errors.New("invalid database connection configuration")
	}
	defer pool.Close()
	ping, cancel := context.WithTimeout(ctx, 5*time.Second)
	e = pool.Ping(ping)
	cancel()
	if e != nil {
		return errors.New("database connection failed")
	}
	if *migrate {
		if e = database.Migrate(ctx, pool, foundation.Migrations); e != nil {
			return e
		}
		days, e := billing.TrialDays()
		if e != nil {
			return e
		}
		return billing.Configure(ctx, pool, c.Region, days)
	}
	days, e := billing.TrialDays()
	if e != nil {
		return e
	}
	if e = billing.Configure(ctx, pool, c.Region, days); e != nil {
		return e
	}
	store := &core.Store{DB: pool}
	kc, e := knowledge.LoadConfig()
	if e != nil {
		return e
	}
	auth := core.NewService(store)
	ks := knowledge.New(auth, kc)
	ac, e := ai.LoadConfig(c.Production)
	if e != nil {
		return e
	}
	engine := ai.New(auth, ks, ac)
	wc, e := widget.Load(c)
	if e != nil {
		return e
	}
	widgets := widget.New(auth, engine, wc)
	ic, e := integrations.Load(c.Production)
	if e != nil {
		return e
	}
	widgets.Integrations = integrations.New(auth, ic)
	retentionDays, e := ops.RetentionDays()
	if e != nil {
		return e
	}
	if e = widgets.Integrations.ValidateSecrets(ctx); e != nil {
		return e
	}
	for _, root := range []string{kc.FileRoot, wc.FileRoot} {
		if e = ops.Writable(root); e != nil {
			return e
		}
	}
	workerCtx, workerCancel := context.WithCancel(ctx)
	workerDone := make(chan struct{})
	go func() { defer close(workerDone); ks.Run(workerCtx) }()
	defer func() { workerCancel(); <-workerDone }()
	aiDone := make(chan struct{})
	go func() { defer close(aiDone); engine.Run(workerCtx) }()
	defer func() { workerCancel(); <-aiDone }()
	integrationDone := make(chan struct{})
	go func() { defer close(integrationDone); widgets.Integrations.Run(workerCtx) }()
	defer func() { workerCancel(); <-integrationDone }()
	srv := &http.Server{Addr: c.Addr, Handler: httpapi.NewWithWidget(auth, c, ks, engine, widgets), BaseContext: func(net.Listener) context.Context { return ctx }, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 150 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	done := make(chan error, 1)
	go func() {
		slog.Info("server listening", "addr", c.Addr, "region", c.Region)
		done <- srv.ListenAndServe()
	}()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			clean, cancel := context.WithTimeout(ctx, 5*time.Second)
			if n, e := (ops.Store{DB: pool}).Cleanup(clean, retentionDays, true); e != nil {
				slog.Error("session cleanup failed")
			} else if n > 0 {
				slog.Info("technical cleanup", "kind", "expired_user_sessions", "deleted", n)
			}
			cancel()
		case e := <-done:
			if e != http.ErrServerClosed {
				return e
			}
			return nil
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			return srv.Shutdown(shutdown)
		}
	}
}
