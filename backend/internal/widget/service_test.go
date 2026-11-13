package widget

import (
	"context"
	"net/http"
	"net/http/httptest"
	"site-assistant/internal/config"
	"site-assistant/internal/knowledge"
	"testing"
)

func TestValidation(t *testing.T) {
	c, e := Load(config.Config{PublicURL: "http://localhost:5173"})
	if e != nil {
		t.Fatal(e)
	}
	s := &Service{Config: c}
	if _, e = s.origins([]string{"https://evil.example"}, "example.com"); e == nil {
		t.Fatal("foreign domain")
	}
	if _, e = s.origins([]string{"https://shop.example.com"}, "example.com"); e != nil {
		t.Fatal(e)
	}
	v := Defaults()
	v.Primary = "url(javascript:alert(1))"
	if validate(v) == nil {
		t.Fatal("CSS injection")
	}
	if _, e = NormalizeOrigin("https://example.com/path"); e == nil {
		t.Fatal("origin path")
	}
	t.Setenv("WIDGET_ALLOW_LOCALHOST", "true")
	if _, e = Load(config.Config{Production: true, PublicURL: "https://app.example"}); e == nil {
		t.Fatal("production dev bypass")
	}
}
func TestVerificationUsesSafeFetcher(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("private verifier contacted") }))
	defer s.Close()
	if _, e := knowledge.NewImporter().Verification(context.Background(), s.URL+"/.well-known/site-assistant-verification.txt"); e == nil {
		t.Fatal("private URL accepted")
	}
}
