package config

import "testing"

func TestProductionConfiguration(t *testing.T) {
	t.Setenv("APP_REGION", "eu")
	t.Setenv("WIDGET_PUBLIC_URL", "https://widget.company.cz")
	t.Setenv("AI_PROVIDER", "disabled")
	t.Setenv("WIDGET_ALLOW_LOCALHOST", "false")
	t.Setenv("WEBHOOK_DEV_RECEIVER_URL", "")
	t.Setenv("BILLING_ADMIN_TOKEN", "")
	c := Config{Production: true, PublicURL: "https://app.company.cz", DatabaseURL: "postgres://app:long-random-test-credential-12345@db.internal/app?sslmode=verify-full"}
	if e := c.ValidateProduction(); e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct{ k, v string }{{"AI_PROVIDER", "mock"}, {"WEBHOOK_DEV_RECEIVER_URL", "http://127.0.0.1:1"}, {"BILLING_ADMIN_TOKEN", "not-allowed"}, {"WIDGET_PUBLIC_URL", "https://localhost."}, {"METRICS_TOKEN", "short"}} {
		t.Run(tc.k, func(t *testing.T) {
			t.Setenv(tc.k, tc.v)
			if c.ValidateProduction() == nil {
				t.Fatal("unsafe configuration accepted")
			}
		})
	}
}

func TestProductionOrigins(t *testing.T) {
	for _, s := range []string{"https://localhost", "https://127.0.0.1", "https://10.1.1.1", "https://x.test", "https://example.com", "https://a.local", "http://company.cz", "https://company.cz/path"} {
		if PublicOrigin(s) {
			t.Fatal(s)
		}
	}
	if !PublicOrigin("https://chat.company.cz") {
		t.Fatal("public origin rejected")
	}
}
