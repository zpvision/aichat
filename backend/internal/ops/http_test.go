package ops

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMetricsPrivateAndRequestID(t *testing.T) {
	t.Setenv("METRICS_TOKEN", "")
	r := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	Store{}.Handler()(w, r)
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
	w = httptest.NewRecorder()
	Observe(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })).ServeHTTP(w, r)
	if len(w.Header().Get("X-Request-ID")) != 36 {
		t.Fatal("request ID")
	}
	if _, ok := interface{}(&response{ResponseWriter: w}).(interface{ Unwrap() http.ResponseWriter }); !ok {
		t.Fatal("SSE controller must unwrap")
	}
}
