package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	Region, Brand, Locale, PublicURL, DatabaseURL, Addr string
	Production                                          bool
}

func Load() (Config, error) {
	if v := env("APP_ENV", "development"); v != "development" && v != "production" {
		return Config{}, fmt.Errorf("invalid APP_ENV")
	}
	c := Config{Region: env("APP_REGION", "eu"), Brand: env("APP_BRAND_NAME", "Site Assistant"), Locale: env("DEFAULT_LOCALE", "en"), PublicURL: env("PUBLIC_APP_URL", "http://localhost:5173"), DatabaseURL: os.Getenv("DATABASE_URL"), Addr: env("HTTP_ADDR", "127.0.0.1:8080"), Production: env("APP_ENV", "development") == "production"}
	if c.Region != "ru" && c.Region != "eu" {
		return c, fmt.Errorf("invalid APP_REGION")
	}
	if !Locale(c.Locale) || strings.TrimSpace(c.Brand) == "" {
		return c, fmt.Errorf("invalid locale or brand")
	}
	u, e := url.Parse(c.PublicURL)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return c, fmt.Errorf("PUBLIC_APP_URL must be an origin")
	}
	c.PublicURL = strings.TrimRight(c.PublicURL, "/")
	if c.Production && u.Scheme != "https" {
		return c, fmt.Errorf("production requires HTTPS")
	}
	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL is required")
	}
	return c, c.ValidateProduction()
}
func env(k, d string) string {
	if v, ok := os.LookupEnv(k); ok {
		return v
	}
	return d
}
func Locale(v string) bool { return v == "ru" || v == "en" || v == "cs" || v == "de" }
