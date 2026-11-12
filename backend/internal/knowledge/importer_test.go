package knowledge

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(r *http.Request, status int, body, ctype string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {ctype}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}
}
func TestRedirectAndRobots(t *testing.T) {
	pageRequests := 0
	i := &Importer{Client: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/robots.txt" {
			return response(r, 404, "", "text/plain"), nil
		}
		pageRequests++
		v := response(r, 302, "", "text/html")
		v.Header.Set("Location", "http://127.0.0.1/secret")
		return v, nil
	})}}
	if _, e := i.Page(context.Background(), "https://public.example/"); e == nil || pageRequests != 1 {
		t.Fatal("redirect SSRF bypass", e, pageRequests)
	}
	pageRequests = 0
	i.Client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/robots.txt" {
			return response(r, 200, "User-agent: *\nDisallow: /private\n", "text/plain"), nil
		}
		pageRequests++
		return response(r, 200, "<p>secret</p>", "text/html"), nil
	})
	if _, e := i.Page(context.Background(), "https://public.example/private"); e == nil || pageRequests != 0 {
		t.Fatal("robots ignored")
	}
}
func TestCrawlLimitsAndRepeat(t *testing.T) {
	s, user, org, a := setup(t)
	s.Config.MaxPages = 3
	s.Config.Depth = 1
	requests := 0
	s.Importer = &Importer{Client: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/robots.txt" {
			return response(r, 404, "", "text/plain"), nil
		}
		requests++
		var b strings.Builder
		fmt.Fprintf(&b, "<html lang='en'><head><title>Page %s</title></head><body><main><p>Accounting services page %s</p>", r.URL.Path, r.URL.Path)
		for n := 0; n < 25; n++ {
			fmt.Fprintf(&b, "<a href='/page%d#anchor'>Next</a>", n)
		}
		b.WriteString("</main></body></html>")
		return response(r, 200, b.String(), "text/html"), nil
	})}}
	v, e := s.Import(context.Background(), user, org, a, "https://public.example/", true)
	if e != nil {
		t.Fatal(e)
	}
	s.ProcessOne(context.Background())
	list, e := s.List(context.Background(), user, org, a)
	if e != nil || len(list) != 3 || requests != 3 {
		t.Fatal("crawl cap", len(list), requests, e)
	}
	v, _ = s.Get(context.Background(), user, org, a, v.ID)
	if v.ImportedPages != 3 {
		t.Fatal("progress", v.ImportedPages)
	}
	s.ProcessOne(context.Background())
	s.ProcessOne(context.Background())
	if _, e = s.Import(context.Background(), user, org, a, "https://public.example/", true); e != nil {
		t.Fatal(e)
	}
	s.ProcessOne(context.Background())
	list, e = s.List(context.Background(), user, org, a)
	if e != nil || len(list) != 3 {
		t.Fatal("repeat duplicated sources", len(list), e)
	}
	s.Importer.Client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/robots.txt" {
			return response(r, 404, "", "text/plain"), nil
		}
		if r.URL.Path == "/partial" {
			return response(r, 200, "<title>Partial</title><main><p>Valid root page</p><a href='/failed'>Link</a></main>", "text/html"), nil
		}
		return response(r, 503, "unavailable", "text/html"), nil
	})
	partial, e := s.Import(context.Background(), user, org, a, "https://public.example/partial", true)
	if e != nil {
		t.Fatal(e)
	}
	s.ProcessOne(context.Background())
	partial, _ = s.Get(context.Background(), user, org, a, partial.ID)
	if partial.Status != "ready" || partial.ErrorCode != "import_partial" || partial.ImportedPages != 1 {
		t.Fatal("partial import not reported", partial.Status, partial.ErrorCode, partial.ImportedPages)
	}
}
