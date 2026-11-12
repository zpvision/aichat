package knowledge

import (
	"context"
	"crypto/tls"
	"errors"
	"github.com/temoto/robotstxt"
	"golang.org/x/net/html"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"site-assistant/internal/core"
	"strings"
	"time"
)

type Importer struct{ Client *http.Client }

func PublicIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsMulticast() {
		return false
	}
	for _, s := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32", "2001::/23", "64:ff9b::/96", "64:ff9b:1::/48", "2002::/16"} {
		if netip.MustParsePrefix(s).Contains(ip) {
			return false
		}
	}
	return true
}
func NormalizeURL(raw string) (*url.URL, error) {
	if len(raw) > 2000 {
		return nil, core.Fail("unsafe_url", 422)
	}
	u, e := url.Parse(strings.TrimSpace(raw))
	if e != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || (u.Port() != "" && u.Port() != "80" && u.Port() != "443") {
		return nil, core.Fail("unsafe_url", 422)
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") || !strings.Contains(host, ".") {
		return nil, core.Fail("unsafe_url", 422)
	}
	if ip, e := netip.ParseAddr(host); e == nil && !PublicIP(ip) {
		return nil, core.Fail("unsafe_url", 422)
	}
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	if u.Path == "" {
		u.Path = "/"
	}
	return u, nil
}
func NewImporter() *Importer {
	dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(addr)
		if e != nil {
			return nil, core.Fail("unsafe_url", 422)
		}
		ips, e := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if e != nil || len(ips) == 0 {
			return nil, core.Fail("import_failed", 503)
		}
		for _, ip := range ips {
			if !PublicIP(ip) {
				return nil, core.Fail("unsafe_url", 422)
			}
		}
		d := net.Dialer{Timeout: 8 * time.Second}
		return d.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
	}
	tr := &http.Transport{Proxy: nil, DialContext: dial, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, TLSHandshakeTimeout: 8 * time.Second, ResponseHeaderTimeout: 10 * time.Second, MaxResponseHeaderBytes: 32 << 10, DisableKeepAlives: true}
	return &Importer{Client: &http.Client{Transport: tr, Timeout: 20 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) >= 4 {
			return core.Fail("unsafe_url", 422)
		}
		_, e := NormalizeURL(r.URL.String())
		return e
	}}}
}
func (i *Importer) fetch(ctx context.Context, u *url.URL, robots bool) ([]byte, *url.URL, int, error) {
	r, e := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if e != nil {
		return nil, u, 0, e
	}
	r.Header.Set("User-Agent", "SiteAssistantBot/1.0")
	resp, e := i.Client.Do(r)
	if e != nil {
		return nil, u, 0, importError(e)
	}
	defer resp.Body.Close()
	if resp.Request.URL.Hostname() != u.Hostname() {
		return nil, u, 0, core.Fail("unsafe_url", 422)
	}
	if resp.StatusCode != 200 {
		if resp.StatusCode >= 500 || resp.StatusCode == 429 {
			return nil, u, resp.StatusCode, core.Fail("import_failed", 503)
		}
		return nil, u, resp.StatusCode, core.Fail("import_failed", 422)
	}
	if !robots && !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") {
		return nil, u, resp.StatusCode, core.Fail("html_required", 422)
	}
	limit := int64(2 << 20)
	if robots {
		limit = 512 << 10
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if e != nil || int64(len(b)) > limit {
		return nil, u, resp.StatusCode, core.Fail("import_failed", 422)
	}
	return b, resp.Request.URL, resp.StatusCode, nil
}
func (i *Importer) Robots(ctx context.Context, u *url.URL) (*robotstxt.Group, error) {
	r := *u
	r.Path = "/robots.txt"
	r.RawQuery = ""
	b, _, status, e := i.fetch(ctx, &r, true)
	if status == 404 {
		return nil, nil
	}
	if e != nil {
		var f *core.Fault
		if errors.As(e, &f) && f.Status >= 500 {
			return nil, core.Fail("robots_denied", 503)
		}
		return nil, core.Fail("robots_denied", 422)
	}
	rules, e := robotstxt.FromBytes(b)
	if e != nil {
		return nil, core.Fail("robots_denied", 422)
	}
	g := rules.FindGroup("SiteAssistantBot")
	if g.CrawlDelay > 5*time.Second {
		return nil, core.Fail("robots_denied", 422)
	}
	return g, nil
}

type WebPage struct {
	Title, Text, Language, URL string
	Links                      []string
}

func (i *Importer) Page(ctx context.Context, raw string) (WebPage, error) {
	u, e := NormalizeURL(raw)
	if e != nil {
		return WebPage{}, e
	}
	rules, e := i.Robots(ctx, u)
	if e != nil {
		return WebPage{}, e
	}
	if rules != nil && !rules.Test(u.RequestURI()) {
		return WebPage{}, core.Fail("robots_denied", 422)
	}
	if rules != nil && rules.CrawlDelay > 0 {
		timer := time.NewTimer(rules.CrawlDelay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return WebPage{}, ctx.Err()
		case <-timer.C:
		}
	}
	// Redirect targets are fetched only after SSRF checks; disable redirects here to enforce robots on every target.
	current := u
	var b []byte
	for n := 0; n < 4; n++ {
		copyClient := *i.Client
		copyClient.CheckRedirect = func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }
		r, _ := http.NewRequestWithContext(ctx, "GET", current.String(), nil)
		r.Header.Set("User-Agent", "SiteAssistantBot/1.0")
		resp, err := copyClient.Do(r)
		if err != nil {
			return WebPage{}, importError(err)
		}
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			next, err := resp.Location()
			resp.Body.Close()
			if err != nil {
				return WebPage{}, core.Fail("import_failed", 422)
			}
			next, err = NormalizeURL(next.String())
			if err != nil || next.Hostname() != u.Hostname() {
				return WebPage{}, core.Fail("unsafe_url", 422)
			}
			if rules != nil && !rules.Test(next.RequestURI()) {
				return WebPage{}, core.Fail("robots_denied", 422)
			}
			current = next
			continue
		}
		if resp.StatusCode >= 500 || resp.StatusCode == 429 {
			resp.Body.Close()
			return WebPage{}, core.Fail("import_failed", 503)
		}
		if resp.StatusCode != 200 || !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") {
			resp.Body.Close()
			return WebPage{}, core.Fail("html_required", 422)
		}
		b, e = io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
		resp.Body.Close()
		if e != nil || len(b) > 2<<20 {
			return WebPage{}, core.Fail("import_failed", 422)
		}
		break
	}
	if b == nil {
		return WebPage{}, core.Fail("import_failed", 422)
	}
	return ParseHTML(b, current)
}
func ParseHTML(b []byte, u *url.URL) (WebPage, error) {
	doc, e := html.Parse(strings.NewReader(string(b)))
	if e != nil {
		return WebPage{}, core.Fail("import_failed", 422)
	}
	out := WebPage{URL: u.String()}
	var title strings.Builder
	var main *html.Node
	var find func(*html.Node)
	find = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "main" || n.Data == "article") && main == nil {
			main = n
		}
		if n.Type == html.ElementNode && n.Data == "html" {
			for _, a := range n.Attr {
				if a.Key == "lang" {
					out.Language = strings.Split(a.Val, "-")[0]
				}
			}
		}
		if n.Type == html.ElementNode && n.Data == "title" {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.TextNode {
					title.WriteString(c.Data)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			find(c)
		}
	}
	find(doc)
	if main == nil {
		main = doc
	}
	var text strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "nav", "header", "footer", "aside", "noscript", "svg", "form", "head":
				return
			}
			if n.Data == "a" {
				for _, a := range n.Attr {
					if a.Key == "href" {
						l, e := url.Parse(a.Val)
						if e == nil {
							v := u.ResolveReference(l)
							if v.Hostname() == u.Hostname() {
								out.Links = append(out.Links, v.String())
							}
						}
					}
				}
			}
			if n.Data == "p" || n.Data == "div" || n.Data == "li" || strings.HasPrefix(n.Data, "h") {
				text.WriteString("\n\n")
			}
		}
		if n.Type == html.TextNode {
			text.WriteString(strings.Join(strings.Fields(n.Data), " ") + " ")
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(main)
	out.Title = strings.TrimSpace(title.String())
	if out.Title == "" {
		out.Title = u.Hostname()
	}
	if len(out.Title) > 200 {
		out.Title = string([]rune(out.Title)[:min(100, len([]rune(out.Title)))])
	}
	out.Text = strings.TrimSpace(text.String())
	if out.Text == "" {
		return out, core.Fail("empty_document", 422)
	}
	return out, nil
}
