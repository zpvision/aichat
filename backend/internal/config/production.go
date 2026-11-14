package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
)

// PublicOrigin deliberately does not resolve DNS during startup.
func PublicOrigin(raw string) bool {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil || u.Host == "" || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return false
	}
	h := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	ip := net.ParseIP(h)
	if ip != nil {
		return false // Production browser origins require public DNS names, not IP literals.
	}
	if !strings.Contains(h, ".") {
		return false
	}
	for _, s := range []string{".localhost", ".local", ".test", ".invalid", ".example", "example.com", "example.org", "example.net"} {
		if strings.HasSuffix(h, s) {
			return false
		}
	}
	return true
}

func (c Config) ValidateProduction() error {
	if !c.Production {
		return nil
	}
	if os.Getenv("APP_REGION") == "" {
		return fmt.Errorf("explicit production APP_REGION required")
	}
	if v := os.Getenv("METRICS_TOKEN"); v != "" && len(v) < 32 {
		return fmt.Errorf("invalid METRICS_TOKEN")
	}
	if !PublicOrigin(c.PublicURL) || !PublicOrigin(os.Getenv("WIDGET_PUBLIC_URL")) {
		return fmt.Errorf("production requires explicit public HTTPS origins")
	}
	u, e := url.Parse(c.DatabaseURL)
	if e != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Host == "" || u.User == nil {
		return fmt.Errorf("invalid production database configuration")
	}
	p, _ := u.User.Password()
	if len(p) < 24 || strings.Contains(p, "local-development") || u.Query().Get("sslmode") != "verify-full" {
		return fmt.Errorf("production database requires strong credentials and sslmode=verify-full")
	}
	if os.Getenv("AI_PROVIDER") == "" || os.Getenv("AI_PROVIDER") == "mock" || os.Getenv("WIDGET_ALLOW_LOCALHOST") == "true" || os.Getenv("WEBHOOK_DEV_RECEIVER_URL") != "" || os.Getenv("BILLING_ADMIN_TOKEN") != "" {
		return fmt.Errorf("unsafe production module configuration")
	}
	return nil
}
