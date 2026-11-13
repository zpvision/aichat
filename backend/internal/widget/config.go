package widget

import (
	"fmt"
	"net/url"
	"os"
	"site-assistant/internal/ai"
	"site-assistant/internal/config"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	PublicURL, AppURL, DistDir, FileRoot string
	Production, AllowLocalhost           bool
	SessionTTL                           time.Duration
	Limits                               ai.WebsiteLimits
}

func Load(c config.Config) (Config, error) {
	v := Config{PublicURL: env("WIDGET_PUBLIC_URL", "http://localhost:8080"), AppURL: c.PublicURL, DistDir: env("WIDGET_DIST_DIR", "../widget/dist"), FileRoot: env("WIDGET_FILE_ROOT", "./data/widget"), Production: c.Production, SessionTTL: 4 * time.Hour, Limits: ai.DefaultWebsiteLimits()}
	u, e := url.Parse(v.PublicURL)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "http" && u.Scheme != "https") {
		return v, fmt.Errorf("invalid WIDGET_PUBLIC_URL")
	}
	v.PublicURL = strings.TrimRight(v.PublicURL, "/")
	v.AllowLocalhost, e = strconv.ParseBool(env("WIDGET_ALLOW_LOCALHOST", "false"))
	if e != nil || v.Production && (v.AllowLocalhost || u.Scheme != "https") {
		return v, fmt.Errorf("invalid production widget configuration")
	}
	hours := 4
	for _, p := range []struct {
		k        string
		p        *int
		min, max int
	}{{"WIDGET_SESSION_TTL_HOURS", &hours, 1, 24}, {"WIDGET_SESSION_MESSAGE_LIMIT", &v.Limits.SessionMessages, 1, 50}, {"WIDGET_SITE_HOURLY_MESSAGES", &v.Limits.SiteHourly, 1, 10000}, {"WIDGET_ORG_HOURLY_MESSAGES", &v.Limits.OrgHourly, 1, 50000}, {"WIDGET_SITE_CONCURRENT", &v.Limits.SiteConcurrent, 1, 8}} {
		if s := os.Getenv(p.k); s != "" {
			n, e := strconv.Atoi(s)
			if e != nil || n < p.min || n > p.max {
				return v, fmt.Errorf("invalid %s", p.k)
			}
			*p.p = n
		}
	}
	v.SessionTTL = time.Duration(hours) * time.Hour
	return v, nil
}
func env(k, d string) string {
	if s, ok := os.LookupEnv(k); ok {
		return s
	}
	return d
}
