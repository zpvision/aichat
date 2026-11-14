package ops

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"site-assistant/internal/core"
	"sync/atomic"
	"time"
)

var requests, errors4, errors5, quota, dbErrors atomic.Int64
var micros atomic.Int64

type response struct {
	http.ResponseWriter
	status int
}

func (w *response) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *response) WriteHeader(n int) {
	if w.status == 0 {
		w.status = n
		w.ResponseWriter.WriteHeader(n)
	}
}
func (w *response) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(b)
}
func Error(code string) {
	if code == "plan_limit_exceeded" || code == "subscription_inactive" {
		quota.Add(1)
	}
}
func Observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := core.ID()
		w.Header().Set("X-Request-ID", id)
		start := time.Now()
		out := &response{ResponseWriter: w}
		next.ServeHTTP(out, r)
		requests.Add(1)
		micros.Add(time.Since(start).Microseconds())
		if out.status >= 500 {
			errors5.Add(1)
		} else if out.status >= 400 {
			errors4.Add(1)
		}
		if out.status >= 500 {
			slog.Error("http request failed", "request_id", id, "status", out.status, "code", out.Header().Get("X-Error-Code"), "duration_ms", time.Since(start).Milliseconds())
		}
	})
}
func (s Store) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := os.Getenv("METRICS_TOKEN")
		if len(key) < 32 || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+key)) != 1 {
			http.NotFound(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		values, e := s.Metrics(ctx)
		if e != nil {
			dbErrors.Add(1)
			http.Error(w, "metrics unavailable", 503)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		for k, v := range values {
			fmt.Fprintf(w, "site_assistant_%s %d\n", k, v)
		}
		for k, v := range map[string]int64{"http_requests_total": requests.Load(), "http_4xx_total": errors4.Load(), "http_5xx_total": errors5.Load(), "http_duration_microseconds_sum": micros.Load(), "http_duration_microseconds_count": requests.Load(), "quota_rejections_total": quota.Load(), "metrics_database_errors_total": dbErrors.Load()} {
			fmt.Fprintf(w, "site_assistant_%s %d\n", k, v)
		}
	}
}
