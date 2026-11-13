package integrations

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"site-assistant/internal/core"
	"site-assistant/internal/knowledge"
	"strconv"
	"time"
)

type Config struct {
	Key         []byte
	DevReceiver string
}

func Load(production bool) (Config, error) {
	c := Config{DevReceiver: os.Getenv("WEBHOOK_DEV_RECEIVER_URL")}
	raw := os.Getenv("WEBHOOK_ENCRYPTION_KEY")
	if raw != "" {
		var e error
		c.Key, e = base64.StdEncoding.DecodeString(raw)
		if e != nil || len(c.Key) != 32 {
			return c, fmt.Errorf("invalid WEBHOOK_ENCRYPTION_KEY")
		}
	}
	if c.DevReceiver != "" {
		u, e := url.Parse(c.DevReceiver)
		ip := net.ParseIP(uHost(u))
		if production || e != nil || u.Scheme != "http" || ip == nil || !ip.IsLoopback() || u.Port() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return c, fmt.Errorf("invalid WEBHOOK_DEV_RECEIVER_URL")
		}
	}
	return c, nil
}
func uHost(u *url.URL) string {
	if u == nil {
		return ""
	}
	return u.Hostname()
}
func token() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func digest(v string) string { b := sha256.Sum256([]byte(v)); return hex.EncodeToString(b[:]) }
func seal(key []byte, org, id, secret string) ([]byte, error) {
	if len(key) != 32 {
		return nil, core.Fail("integration_unconfigured", 503)
	}
	a, _ := aes.NewCipher(key)
	g, _ := cipher.NewGCM(a)
	n := make([]byte, g.NonceSize())
	if _, e := rand.Read(n); e != nil {
		return nil, e
	}
	return g.Seal(n, n, []byte(secret), []byte(org+":"+id)), nil
}
func open(key []byte, org, id string, b []byte) (string, error) {
	if len(key) != 32 {
		return "", fmt.Errorf("missing encryption key")
	}
	a, _ := aes.NewCipher(key)
	g, _ := cipher.NewGCM(a)
	if len(b) < g.NonceSize() {
		return "", fmt.Errorf("invalid secret")
	}
	v, e := g.Open(nil, b[:g.NonceSize()], b[g.NonceSize():], []byte(org+":"+id))
	return string(v), e
}
func Signature(secret, timestamp string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(timestamp + "."))
	m.Write(body)
	return "v1=" + hex.EncodeToString(m.Sum(nil))
}
func Verify(secret, timestamp, signature string, body []byte, now time.Time) bool {
	sec, e := strconv.ParseInt(timestamp, 10, 64)
	if e != nil || now.Sub(time.Unix(sec, 0)) > 5*time.Minute || time.Unix(sec, 0).Sub(now) > 5*time.Minute {
		return false
	}
	return hmac.Equal([]byte(Signature(secret, timestamp, body)), []byte(signature))
}
func (c Config) ValidateURL(raw string) error {
	parsed, parseError := url.Parse(raw)
	if parseError != nil || parsed.Fragment != "" {
		return core.Fail("unsafe_url", 422)
	}
	if raw == c.DevReceiver && raw != "" {
		return nil
	}
	u, e := knowledge.NormalizeURL(raw)
	if e != nil || u.Scheme != "https" || u.Fragment != "" || u.RawQuery != "" || u.Port() != "" && u.Port() != "443" {
		return core.Fail("unsafe_url", 422)
	}
	return nil
}
func (c Config) Client(raw string) *http.Client {
	client := *knowledge.NewImporter().Client
	client.Timeout = 8 * time.Second
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if raw != "" && raw == c.DevReceiver {
		client.Transport = &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			u, _ := url.Parse(raw)
			if addr != u.Host {
				return nil, fmt.Errorf("forbidden address")
			}
			d := net.Dialer{Timeout: 3 * time.Second}
			return d.DialContext(ctx, network, addr)
		}, DisableKeepAlives: true, ResponseHeaderTimeout: 5 * time.Second, MaxResponseHeaderBytes: 16 << 10}
	}
	return &client
}
