package integrations

import (
	"bytes"
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"io"
	"log/slog"
	"net/http"
	"site-assistant/internal/core"
	"strconv"
	"time"
)

type job struct {
	ID, Org, Endpoint, Lease string
	Attempts, Max            int
	Event                    Event
}

func (s *Store) claim(ctx context.Context) (job, error) {
	j := job{}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return j, e
	}
	defer tx.Rollback(ctx)
	// Lease recovery is persistent, including a crash during the final attempt.
	if _, e = tx.Exec(ctx, "UPDATE webhook_attempts a SET code='lease_expired' FROM webhook_deliveries d WHERE a.id=d.lease_token AND a.code='processing' AND d.status='processing' AND d.lease_until<now() AND d.attempts>=d.max_attempts"); e != nil {
		return j, e
	}
	_, e = tx.Exec(ctx, "UPDATE webhook_deliveries SET status='failed',last_code='lease_expired',updated_at=now() WHERE status='processing' AND lease_until<now() AND attempts>=max_attempts")
	if e != nil {
		return j, e
	}
	e = tx.QueryRow(ctx, `SELECT id,organization_id,endpoint_id,attempts,max_attempts FROM webhook_deliveries WHERE (status='pending' AND next_attempt_at<=now() OR status='processing' AND lease_until<now()) AND attempts<max_attempts ORDER BY next_attempt_at,id FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&j.ID, &j.Org, &j.Endpoint, &j.Attempts, &j.Max)
	if errors.Is(e, pgx.ErrNoRows) {
		if err := tx.Commit(ctx); err != nil {
			return j, err
		}
		return j, pgx.ErrNoRows
	}
	if e != nil {
		return j, e
	}
	j.Attempts++
	j.Lease = core.ID()
	if _, e = tx.Exec(ctx, "UPDATE webhook_attempts SET code='lease_expired' WHERE delivery_id=$1 AND code='processing'", j.ID); e != nil {
		return j, e
	}
	if _, e = tx.Exec(ctx, "UPDATE webhook_deliveries SET status='processing',attempts=$2,lease_token=$3,lease_until=now()+interval '60 seconds',updated_at=now() WHERE id=$1", j.ID, j.Attempts, j.Lease); e != nil {
		return j, e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO webhook_attempts(id,organization_id,delivery_id,attempt,code,http_status,duration_ms) VALUES($1,$2,$3,$4,'processing',0,0)", j.Lease, j.Org, j.ID, j.Attempts); e != nil {
		return j, e
	}
	e = tx.QueryRow(ctx, "SELECT e.id,e.event_type,e.lead_id,e.created_at FROM integration_events e JOIN webhook_deliveries d ON d.event_id=e.id AND d.organization_id=e.organization_id WHERE d.id=$1", j.ID).Scan(&j.Event.ID, &j.Event.Type, &j.Event.LeadID, &j.Event.CreatedAt)
	if e != nil {
		return j, e
	}
	return j, tx.Commit(ctx)
}
func (s *Service) send(ctx context.Context, j job) (string, int) {
	tx, url, encrypted, enabled, e := s.Store.endpoint(ctx, j.Org, j.Endpoint)
	if e != nil {
		return "database_unavailable", 0
	}
	defer tx.Rollback(ctx)
	if !enabled {
		return "disabled", 0
	}
	if s.Config.ValidateURL(url) != nil {
		return "unsafe_url", 0
	}
	secret, e := open(s.Config.Key, j.Org, j.Endpoint, encrypted)
	if e != nil {
		return "secret_unavailable", 0
	}
	body := raw(j.Event)
	req, e := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if e != nil {
		return "unsafe_url", 0
	}
	stamp := strconv.FormatInt(time.Now().Unix(), 10)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "SiteAssistant-Webhooks/1.0")
	req.Header.Set("X-SiteAssistant-Timestamp", stamp)
	req.Header.Set("X-SiteAssistant-Signature", Signature(secret, stamp, body))
	req.Header.Set("X-SiteAssistant-Event-ID", j.Event.ID)
	resp, e := s.Config.Client(url).Do(req)
	if e != nil {
		return "network_error", 0
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return "delivered", resp.StatusCode
	}
	return "http_error", resp.StatusCode
}
func (s *Store) finish(ctx context.Context, j job, code string, status int, duration time.Duration) error {
	state := "pending"
	if code == "delivered" {
		state = "delivered"
	} else if code == "disabled" {
		state = "canceled"
	} else if j.Attempts >= j.Max {
		state = "failed"
	}
	backoff := []int{1, 5, 30, 120, 600, 1800, 3600, 7200}[min(j.Attempts-1, 7)]
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	tag, e := tx.Exec(ctx, "UPDATE webhook_deliveries SET status=$4,last_code=$5,http_status=$6,next_attempt_at=now()+$7*interval '1 second',lease_until=NULL,updated_at=now() WHERE organization_id=$1 AND id=$2 AND lease_token=$3 AND status='processing'", j.Org, j.ID, j.Lease, state, code, status, backoff)
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	if _, e = tx.Exec(ctx, "UPDATE webhook_attempts SET code=$2,http_status=$3,duration_ms=$4 WHERE id=$1", j.Lease, code, status, duration.Milliseconds()); e != nil {
		return e
	}
	return tx.Commit(ctx)
}

// Once is also used by integration tests with real PostgreSQL and an actual local receiver.
func (s *Service) Once(ctx context.Context) (bool, error) {
	if len(s.Config.Key) != 32 {
		return false, nil
	}
	j, e := s.Store.claim(ctx)
	if errors.Is(e, pgx.ErrNoRows) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	start := time.Now()
	call, cancel := context.WithTimeout(ctx, 10*time.Second)
	code, status := s.send(call, j)
	cancel()
	finish, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()
	return true, s.Store.finish(finish, j, code, status, time.Since(start))
}
func (s *Service) Run(ctx context.Context) {
	lastFailure := time.Time{}
	for {
		if ctx.Err() != nil {
			return
		}
		worked, e := s.Once(ctx)
		if e != nil && ctx.Err() == nil && time.Since(lastFailure) > time.Minute {
			slog.Error("webhook worker unavailable", "component", "integrations")
			lastFailure = time.Now()
		}
		if worked && e == nil {
			continue
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
