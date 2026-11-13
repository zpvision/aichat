package ai

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/url"
	"os"
	foundation "site-assistant"
	"site-assistant/internal/core"
	"site-assistant/internal/database"
	"site-assistant/internal/knowledge"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeRetriever struct {
	hits    []knowledge.Hit
	failure error
}

func (f fakeRetriever) Search(context.Context, string, string, string, knowledge.SearchInput) ([]knowledge.Hit, error) {
	return f.hits, f.failure
}

type recorder struct {
	mu       sync.Mutex
	requests []GenerateRequest
	provider AIProvider
	started  chan struct{}
	once     sync.Once
}

func (r *recorder) Generate(ctx context.Context, in GenerateRequest, stream bool, emit func(string) error) (GenerateResult, error) {
	r.mu.Lock()
	r.requests = append(r.requests, in)
	r.mu.Unlock()
	if r.started != nil {
		r.once.Do(func() { close(r.started) })
	}
	return r.provider.Generate(ctx, in, stream, emit)
}
func setupAI(t *testing.T) (*Service, *knowledge.Service, string, Scope) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	u, e := url.Parse(dsn)
	if e != nil || !strings.HasSuffix(u.Path, "_test") {
		t.Fatal("test DB required")
	}
	ctx := context.Background()
	admin, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	schema := "ai_" + strings.ReplaceAll(core.ID(), "-", "")
	if _, e = admin.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal(e)
	}
	cfg, _ := pgxpool.ParseConfig(dsn)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { pool.Close(); admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); admin.Close() })
	for i := 0; i < 2; i++ {
		if e = database.Migrate(ctx, pool, foundation.Migrations); e != nil {
			t.Fatal(e)
		}
	}
	auth := core.NewService(&core.Store{DB: pool})
	user, e := auth.Register(ctx, "ai@example.com", "test-password-12345", "en")
	if e != nil {
		t.Fatal(e)
	}
	org, e := auth.SaveOrganization(ctx, user.ID, "", core.OrganizationInput{Name: ptr("Company"), Country: ptr("CZ")})
	if e != nil {
		t.Fatal(e)
	}
	assistant, e := auth.SaveAssistant(ctx, user.ID, org.ID, "", core.AssistantInput{Name: ptr("Guide")})
	if e != nil {
		t.Fatal(e)
	}
	kc, e := knowledge.LoadConfig()
	if e != nil {
		t.Fatal(e)
	}
	kc.FileRoot = t.TempDir()
	k := knowledge.New(auth, kc)
	s := New(auth, fakeRetriever{hits: []knowledge.Hit{{ID: core.ID(), SourceID: core.ID(), Title: "Knowledge", Content: "Tariff Aurora costs 450 EUR per month.", Revision: 1}}}, testConfig(t))
	s.Provider = &MockProvider{}
	scope := Scope{OrganizationID: org.ID, AssistantID: assistant.ID}
	conv, e := s.Create(ctx, user.ID, scope)
	if e != nil {
		t.Fatal(e)
	}
	scope.ConversationID = conv.ID
	return s, k, user.ID, scope
}
func ptr(s string) *string { return &s }
func TestConversationFlow(t *testing.T) {
	s, _, user, v := setupAI(t)
	ctx := context.Background()
	r := &recorder{provider: &MockProvider{}}
	s.Provider = r
	first := MessageInput{RequestID: core.ID(), Content: "What is the price?"}
	var text strings.Builder
	result, e := s.Message(ctx, user, v, first, true, func(ev Event) error {
		if ev.Type == "delta" {
			text.WriteString(ev.Data.(map[string]string)["text"])
		}
		return nil
	})
	if e != nil || result.Execution.Status != "completed" || result.AssistantMessage.Content != text.String() || len(result.Execution.Sources) != 1 {
		t.Fatal(result.Execution, e)
	}
	replay, e := s.Message(ctx, user, v, first, false, nil)
	if e != nil || !replay.Replay || replay.Execution.ID != result.Execution.ID || len(r.requests) != 1 {
		t.Fatal("idempotency", e)
	}
	first.Content = "Different request body"
	if _, e = s.Message(ctx, user, v, first, false, nil); e == nil {
		t.Fatal("idempotency body mismatch")
	}
	second, e := s.Message(ctx, user, v, MessageInput{RequestID: core.ID(), Content: "And what is included?"}, false, nil)
	if e != nil {
		t.Fatal(e)
	}
	if len(r.requests) != 2 {
		t.Fatal("provider calls")
	}
	found := false
	for _, m := range r.requests[1].Messages {
		if m.Content == "What is the price?" {
			found = true
		}
	}
	if !found {
		t.Fatal("history missing")
	}
	d, e := s.Detail(ctx, user, v, 0)
	if e != nil || len(d.Messages) != 4 || len(d.Executions) != 2 || second.AssistantMessage.Sequence <= result.AssistantMessage.Sequence {
		t.Fatal("history order", d, e)
	}
	if e = s.Delete(ctx, user, v); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Detail(ctx, user, v, 0); e == nil {
		t.Fatal("deleted conversation accessible")
	}
}
func TestFailuresAndCancellation(t *testing.T) {
	for _, mode := range []string{"provider", "cancel", "timeout", "stream_disconnect"} {
		t.Run(mode, func(t *testing.T) {
			s, _, user, v := setupAI(t)
			m := &MockProvider{}
			if mode == "provider" {
				m.Failure = core.Fail("ai_unavailable", 503)
				m.FailAfter = 1
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "timeout" {
				s.Config.Timeout = 10 * time.Millisecond
				m.Delay = time.Second
			}
			if mode == "cancel" {
				m.Delay = time.Second
			}
			s.Provider = m
			_, e := s.Message(ctx, user, v, MessageInput{RequestID: core.ID(), Content: "Hello"}, true, func(ev Event) error {
				if mode == "cancel" && ev.Type == "start" {
					if e := s.Cancel(context.Background(), user, v); e != nil {
						t.Error(e)
					}
				}
				if mode == "stream_disconnect" && ev.Type == "delta" {
					cancel()
					return context.Canceled
				}
				return nil
			})
			if e == nil {
				t.Fatal("expected failure")
			}
			d, e := s.Detail(context.Background(), user, v, 0)
			if e != nil || len(d.Messages) != 2 || d.Messages[1].Content != "" || d.Messages[1].Status == "completed" || d.Executions[0].Status == "processing" {
				t.Fatal("partial response persisted", d, e)
			}
			if (mode == "cancel" || mode == "stream_disconnect") && d.Executions[0].Status != "canceled" {
				t.Fatal("cancellation misclassified", d.Executions[0].Status)
			}
		})
	}
}
func TestConcurrentAndIsolation(t *testing.T) {
	s, _, user, v := setupAI(t)
	r := &recorder{provider: &MockProvider{Delay: time.Second}, started: make(chan struct{})}
	s.Provider = r
	in := MessageInput{RequestID: core.ID(), Content: "Hello"}
	done := make(chan error, 1)
	go func() { _, e := s.Message(context.Background(), user, v, in, false, nil); done <- e }()
	select {
	case <-r.started:
	case <-time.After(3 * time.Second):
		t.Fatal("provider did not start")
	}
	if _, e := s.Message(context.Background(), user, v, MessageInput{RequestID: core.ID(), Content: "Second"}, false, nil); e == nil || e.Error() != "conversation_busy" {
		t.Fatal("parallel execution", e)
	}
	if _, e := s.Message(context.Background(), user, v, in, false, nil); e == nil || e.Error() != "request_in_progress" {
		t.Fatal("parallel duplicate", e)
	}
	if e := s.Cancel(context.Background(), user, v); e != nil {
		t.Fatal(e)
	}
	if e := <-done; e == nil {
		t.Fatal("cancel ignored")
	}
	ctx := context.Background()
	org, e := s.Auth.SaveOrganization(ctx, user, "", core.OrganizationInput{Name: ptr("Other"), Country: ptr("DE")})
	if e != nil {
		t.Fatal(e)
	}
	as, e := s.Auth.SaveAssistant(ctx, user, org.ID, "", core.AssistantInput{Name: ptr("Other")})
	if e != nil {
		t.Fatal(e)
	}
	foreign := Scope{org.ID, as.ID, v.ConversationID}
	if _, e = s.Detail(ctx, user, foreign, 0); e == nil {
		t.Fatal("cross tenant history")
	}
	if _, e = s.Message(ctx, user, foreign, MessageInput{RequestID: core.ID(), Content: "secret"}, false, nil); e == nil {
		t.Fatal("cross tenant write")
	}
	otherAs, e := s.Auth.SaveAssistant(ctx, user, v.OrganizationID, "", core.AssistantInput{Name: ptr("Other assistant")})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Detail(ctx, user, Scope{v.OrganizationID, otherAs.ID, v.ConversationID}, 0); e == nil {
		t.Fatal("cross assistant history")
	}
	op, e := s.Auth.Register(ctx, "operator@example.com", "test-password-12345", "en")
	if e != nil {
		t.Fatal(e)
	}
	s.Store.DB.Exec(ctx, "INSERT INTO organization_members(organization_id,user_id,role) VALUES($1,$2,'operator')", v.OrganizationID, op.ID)
	if _, e = s.Detail(ctx, op.ID, v, 0); e == nil {
		t.Fatal("operator read preview")
	}
	if _, e = s.Message(ctx, op.ID, v, in, false, nil); e == nil {
		t.Fatal("operator generated")
	}
	if _, e = s.UpdateSettings(ctx, op.ID, v, SettingsPatch{Role: ptr("Admin")}); e == nil {
		t.Fatal("operator configured")
	}
}
func TestFallbackAndRecovery(t *testing.T) {
	s, _, user, v := setupAI(t)
	s.Knowledge = fakeRetriever{}
	r := &recorder{provider: &MockProvider{}}
	s.Provider = r
	result, e := s.Message(context.Background(), user, v, MessageInput{RequestID: core.ID(), Content: "Сколько стоит услуга?"}, false, nil)
	if e != nil || !result.Execution.PolicyFallback || len(result.Execution.Sources) != 0 || !strings.Contains(result.AssistantMessage.Content, "подтверждённой информации") {
		t.Fatal(result, e)
	}
	if r.requests[0].HasKnowledge {
		t.Fatal("fallback prompt")
	}
	s.Config.Provider = "ollama"
	result, e = s.Message(context.Background(), user, v, MessageInput{RequestID: core.ID(), Content: "What is the price?"}, false, nil)
	if e != nil || len(r.requests) != 1 || !result.Execution.PolicyFallback {
		t.Fatal("server fallback invoked provider", e)
	}
	reserved, e := s.Store.Begin(context.Background(), v, MessageInput{RequestID: core.ID(), Content: "Abandoned"}, "en", "hash", "mock", "mock", time.Second)
	if e != nil {
		t.Fatal(e)
	}
	s.Store.DB.Exec(context.Background(), "UPDATE ai_executions SET expires_at=now()-interval '1 second' WHERE id=$1", reserved.Execution.ID)
	if e = s.Store.Recover(context.Background()); e != nil {
		t.Fatal(e)
	}
	recovered, _ := s.Store.Result(context.Background(), v, reserved.Execution.ID)
	if recovered.Execution.Status != "failed" || recovered.Execution.ErrorCode != "ai_interrupted" || recovered.AssistantMessage.Status != "failed" {
		t.Fatal(recovered)
	}
}
func TestRealRAGIntoMockAI(t *testing.T) {
	if os.Getenv("REAL_EMBEDDINGS") != "1" {
		t.Skip("REAL_EMBEDDINGS=1 requires real BGE-M3; generation remains mock")
	}
	s, k, user, v := setupAI(t)
	s.Knowledge = k
	r := &recorder{provider: &MockProvider{}}
	s.Provider = r
	ctx := context.Background()
	article, e := k.Article(ctx, user, v.OrganizationID, v.AssistantID, knowledge.Input{Title: ptr("Aurora"), Content: ptr("Тариф Aurora включает бухгалтерское обслуживание и расчёт заработной платы. Стоимость составляет 450 евро в месяц."), Language: ptr("ru")})
	if e != nil {
		t.Fatal(e)
	}
	if e = k.ProcessOne(ctx); e != nil {
		t.Fatal(e)
	}
	other, e := s.Auth.SaveAssistant(ctx, user, v.OrganizationID, "", core.AssistantInput{Name: ptr("Isolated assistant")})
	if e != nil {
		t.Fatal(e)
	}
	_, e = k.Article(ctx, user, v.OrganizationID, other.ID, knowledge.Input{Title: ptr("Aurora secret"), Content: ptr("Тариф Aurora включает бухгалтерское обслуживание. Секретная цена другого ассистента составляет 9999 евро в месяц.")})
	if e != nil {
		t.Fatal(e)
	}
	if e = k.ProcessOne(ctx); e != nil {
		t.Fatal(e)
	}
	result, e := s.Message(ctx, user, v, MessageInput{RequestID: core.ID(), Content: "Wie viel kostet der Tarif Aurora?"}, false, nil)
	if e != nil || len(result.Execution.Sources) == 0 {
		t.Fatal(result.Execution, e)
	}
	for _, h := range result.Execution.Sources {
		if h.SourceID != article.ID || h.Revision != 1 {
			t.Fatal("incorrect source scope/version", h)
		}
	}
	if len(r.requests) == 0 || !strings.Contains(r.requests[0].Messages[1].Content, "450") {
		t.Fatal("real RAG not passed into prompt")
	}
	_, e = k.Update(ctx, user, v.OrganizationID, v.AssistantID, article.ID, knowledge.Input{Content: ptr("Тариф Aurora включает бухгалтерское обслуживание и расчёт заработной платы. Стоимость составляет 500 евро в месяц.")})
	if e != nil {
		t.Fatal(e)
	}
	k.ProcessOne(ctx)
	result, e = s.Message(ctx, user, v, MessageInput{RequestID: core.ID(), Content: "Wie viel kostet der Tarif Aurora jetzt?"}, false, nil)
	if e != nil || len(result.Execution.Sources) == 0 || result.Execution.Sources[0].Revision != 2 || !strings.Contains(r.requests[1].Messages[1].Content, "500") {
		t.Fatal("stale RAG", result.Execution, e)
	}
	t.Log("Real BGE-M3 RAG delivered scoped revision 1 and revision 2 into Mock AI; no Gemma used")
}
