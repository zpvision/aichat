package config

import "testing"

func TestConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("APP_REGION", "eu")
	t.Setenv("APP_ENV", "development")
	t.Setenv("DEFAULT_LOCALE", "de")
	t.Setenv("PUBLIC_APP_URL", "http://localhost:5173")
	if _, e := Load(); e != nil {
		t.Fatal(e)
	}
	t.Setenv("APP_REGION", "us")
	if _, e := Load(); e == nil {
		t.Fatal("invalid region accepted")
	}
	t.Setenv("APP_REGION", "ru")
	t.Setenv("APP_ENV", "production")
	if _, e := Load(); e == nil {
		t.Fatal("insecure production origin accepted")
	}
}
