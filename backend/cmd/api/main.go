package main

import (
	"context"
	"errors"
	"flag"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	foundation "site-assistant"
	"site-assistant/internal/config"
	"site-assistant/internal/core"
	"site-assistant/internal/database"
	"site-assistant/internal/httpapi"
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
	pool, e := pgxpool.New(ctx, c.DatabaseURL)
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
		return database.Migrate(ctx, pool, foundation.Migrations)
	}
	store := &core.Store{DB: pool}
	srv := &http.Server{Addr: c.Addr, Handler: httpapi.New(core.NewService(store), c), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
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
			if store.Cleanup(clean) != nil {
				slog.Error("session cleanup failed")
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
