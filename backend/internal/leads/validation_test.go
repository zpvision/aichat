package leads

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"site-assistant/internal/ai"
	"strings"
	"testing"
	"time"
)

func TestFieldValidation(t *testing.T) {
	min, max := 1.0, 100.0
	tests := []struct {
		f         Field
		raw, code string
	}{
		{Field{Type: "email"}, `"Anna@Example.com"`, ""}, {Field{Type: "email"}, `"bad@"`, "lead_value_email"},
		{Field{Type: "phone"}, `"+420 777 123 456"`, ""}, {Field{Type: "phone"}, `"777123456"`, "lead_value_phone"}, {Field{Type: "phone"}, `""`, "lead_value_phone"},
		{Field{Type: "inn_ru"}, `"7707083893"`, ""}, {Field{Type: "inn_ru"}, `"500100732259"`, ""}, {Field{Type: "inn_ru"}, `"7707083894"`, "lead_value_inn"}, {Field{Type: "inn_ru"}, `"0000000000"`, "lead_value_inn"},
		{Field{Type: "number", Min: &min, Max: &max, Integer: true}, `12`, ""}, {Field{Type: "number", Min: &min, Max: &max, Integer: true}, `12.5`, "lead_value_range"}, {Field{Type: "number"}, `1e99`, "lead_value_range"}, {Field{Type: "number"}, `"12"`, "lead_value_type"}, {Field{Type: "number"}, `null`, "lead_value_type"},
		{Field{Type: "select", Options: []string{"CZ", "DE"}}, `"RU"`, "lead_value_select"}, {Field{Type: "text", MaxLength: 4}, `"Český"`, "lead_value_length"}, {Field{Type: "text", MaxLength: 100}, `"<script>alert(1)</script>"`, ""},
	}
	for _, tt := range tests {
		_, code := ValidateValue(tt.f, []byte(tt.raw))
		if code != tt.code {
			t.Errorf("%s %s got %s want %s", tt.f.Type, tt.raw, code, tt.code)
		}
	}
	s := Defaults()
	s.Enabled = true
	if e := ValidateSettings(&s); e != nil {
		t.Fatal(e)
	}
	s.Fields[1].Required = true
	if ValidateSettings(&s) == nil {
		t.Fatal("cannot require both contact fields")
	}
	s = Defaults()
	s.Fields = append(s.Fields, s.Fields[0])
	if ValidateSettings(&s) == nil {
		t.Fatal("duplicate key")
	}
	s = Defaults()
	s.Fields[0].Key = "injected;"
	if ValidateSettings(&s) == nil {
		t.Fatal("invalid key")
	}
	s = Defaults()
	if len(Missing(s, map[string]Value{"email": {Value: json.RawMessage(`"a@example.com"`), Valid: true}})) != 0 {
		t.Fatal("one contact enough")
	}
	s.RequireContact = false
	if len(Missing(s, map[string]Value{})) == 0 {
		t.Fatal("empty lead")
	}
}
func TestUntrustedProposalsAndCorrections(t *testing.T) {
	s := Defaults()
	src := []Source{{"one", 1, `{"email":"bad@"}`}, {"two", 2, `{"email":"a@example.com"}`}}
	candidates := []Candidate{{"email", json.RawMessage(`"bad@"`), "one", "bad@"}, {"email", json.RawMessage(`"a@example.com"`), "two", "a@example.com"}}
	v, e := proposals(s, nil, src, candidates)
	if e != nil || !v["email"].Valid || v["email"].MessageID != "two" {
		t.Fatal(v, e)
	}
	same, e := proposals(s, v, src, candidates)
	if e != nil || !same["email"].UpdatedAt.Equal(v["email"].UpdatedAt) {
		t.Fatal("duplicate changed", e)
	}
	for _, bad := range []Candidate{{"unknown", json.RawMessage(`"x"`), "two", "x"}, {"email", json.RawMessage(`"invented@example.com"`), "two", "a@example.com"}, {"email", json.RawMessage(`"a@example.com"`), "foreign", "a@example.com"}} {
		if _, e = proposals(s, nil, src, []Candidate{bad}); e == nil {
			t.Fatal("accepted invented key/value/source")
		}
	}
	v["email"] = Value{Value: json.RawMessage(`"manual@example.com"`), Valid: true, Source: "visitor_edit", Sequence: 2}
	v, e = proposals(s, v, src, candidates)
	if e != nil || string(v["email"].Value) != `"manual@example.com"` {
		t.Fatal("manual correction overwritten", e)
	}
	for _, lang := range []string{"ru", "en", "cs", "de"} {
		_, q := collectionPrompt(s, v, lang, "")
		if q == "" {
			t.Fatal(lang)
		}
	}
	for _, p := range []string{"Хочу оставить заявку", "Please contact me", "Chci podat žádost", "Bitte kontaktieren Sie mich"} {
		if !explicitIntent(p) {
			t.Fatal(p)
		}
	}
	if explicitIntent("Hello") || explicitIntent("yes") {
		t.Fatal("forced collection")
	}
}
func TestStructuredMocksAndOllamaContract(t *testing.T) {
	fields := Defaults().Fields
	for _, body := range []string{"Меня зовут Анна. У нас 12 сотрудников. Email anna@example.com.", "My name is Anna. I have 12 employees. Email anna@example.com.", "Jmenuji se Anna. Máme 12 zaměstnanců. Email anna@example.com.", "Mein Name ist Anna. Wir haben 12 Mitarbeiter. Email anna@example.com."} {
		in := ExtractInput{Fields: fields, Messages: []Source{{"one", 1, body}}}
		c, e := (&MockStructuredExtractor{}).Extract(context.Background(), in)
		v, e2 := proposals(Defaults(), nil, in.Messages, c)
		if e != nil || e2 != nil || !v["email"].Valid {
			t.Fatal(body, c, e, e2)
		}
	}
	m := MockStructuredExtractor{Delay: time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if _, e := m.Extract(ctx, ExtractInput{}); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	in := ExtractInput{Fields: fields, Messages: []Source{{"one", 1, "anna@example.com"}}}
	response := `{"candidates":[{"key":"email","value":"anna@example.com","message_id":"one","evidence":"anna@example.com"}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Error(r.URL.Path)
		}
		var req struct {
			Format   map[string]any   `json:"format"`
			Stream   bool             `json:"stream"`
			Messages []ai.ChatMessage `json:"messages"`
		}
		if json.NewDecoder(r.Body).Decode(&req) != nil || req.Format["type"] != "object" || req.Stream || len(req.Messages) != 2 {
			t.Error("structured contract", req)
		}
		json.NewEncoder(w).Encode(map[string]any{"model": "contract-test-only", "done": true, "message": map[string]string{"role": "assistant", "content": response}})
	}))
	defer server.Close()
	o := OllamaStructuredExtractor{ai.NewOllama(ai.Config{BaseURL: server.URL, Model: "contract-test-only", Timeout: time.Second, MaxOutputTokens: 2048})}
	out, e := o.Extract(context.Background(), in)
	if e != nil || len(out) != 1 {
		t.Fatal(out, e)
	}
	for _, bad := range []string{`not json`, `{"candidates":[],"secret":1}`, `{"candidates":null}`, `{"candidates":[]} trailing`} {
		response = bad
		if _, e = o.Extract(context.Background(), in); e == nil {
			t.Fatal("accepted invalid response", strings.TrimSpace(bad))
		}
	}
}
