package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"site-assistant/internal/config"
	"site-assistant/internal/core"
	"site-assistant/internal/knowledge"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type Retriever interface {
	Search(context.Context, string, string, string, knowledge.SearchInput) ([]knowledge.Hit, error)
}
type Service struct {
	Auth          *core.Service
	Store         *Store
	Knowledge     Retriever
	Provider      AIProvider
	Config        Config
	slots         chan struct{}
	mu            sync.Mutex
	active        map[string]context.CancelFunc
	WebsiteLimits WebsiteLimits
}

func New(auth *core.Service, k Retriever, c Config) *Service {
	s := &Service{Auth: auth, Knowledge: k, Store: &Store{DB: auth.Store.DB}, Config: c, slots: make(chan struct{}, c.Concurrency), active: map[string]context.CancelFunc{}}
	s.WebsiteLimits = DefaultWebsiteLimits()
	switch c.Provider {
	case "mock":
		s.Provider = &MockProvider{Delay: 40 * time.Millisecond}
	case "ollama":
		s.Provider = NewOllama(c)
	default:
		s.Provider = DisabledProvider{}
	}
	return s
}
func (s *Service) model() string {
	if s.Config.Provider == "mock" {
		return "deterministic-demo-v1"
	}
	if s.Config.Provider == "disabled" {
		return ""
	}
	return s.Config.Model
}
func (s *Service) Access(ctx context.Context, user string, v Scope, write bool) error {
	if role, e := s.Auth.Access(ctx, user, v.OrganizationID, write, false); e != nil {
		return e
	} else if role == "operator" {
		return core.Fail("forbidden", 403)
	}
	if _, e := s.Auth.Assistant(ctx, user, v.OrganizationID, v.AssistantID); e != nil {
		return e
	}
	if v.ConversationID != "" {
		_, e := s.Store.Conversation(ctx, v)
		return e
	}
	return nil
}
func (s *Service) Settings(ctx context.Context, user string, v Scope) (SettingsResponse, error) {
	out := SettingsResponse{Provider: s.Config.Provider, Model: s.model(), Demo: s.Config.Provider == "mock", MaxMessageChars: 2000}
	if e := s.Access(ctx, user, v, false); e != nil {
		return out, e
	}
	settings, e := s.Store.Settings(ctx, v.OrganizationID, v.AssistantID)
	if e != nil {
		return out, e
	}
	a, e := s.Auth.Assistant(ctx, user, v.OrganizationID, v.AssistantID)
	if settings.DisplayName == "" {
		settings.DisplayName = ClipRunes(a.Name, 120)
	}
	out.Settings = settings
	return out, e
}
func (s *Service) UpdateSettings(ctx context.Context, user string, v Scope, in SettingsPatch) (SettingsResponse, error) {
	if e := s.Access(ctx, user, v, true); e != nil {
		return SettingsResponse{}, e
	}
	conf, e := s.Store.Settings(ctx, v.OrganizationID, v.AssistantID)
	if e != nil {
		return SettingsResponse{}, e
	}
	for _, p := range []struct {
		in    *string
		out   *string
		limit int
	}{{in.DisplayName, &conf.DisplayName, 120}, {in.Role, &conf.Role, 200}, {in.CompanyDescription, &conf.CompanyDescription, 2000}, {in.Style, &conf.Style, 300}, {in.Instructions, &conf.Instructions, 3000}, {in.NoAnswer, &conf.NoAnswer, 600}, {in.LanguagePolicy, &conf.LanguagePolicy, 16}, {in.ResponseLocale, &conf.ResponseLocale, 2}} {
		if p.in != nil {
			if utf8.RuneCountInString(*p.in) > p.limit || strings.ContainsRune(*p.in, 0) {
				return SettingsResponse{}, core.Fail("validation", 422)
			}
			*p.out = strings.TrimSpace(*p.in)
		}
	}
	if in.OfferOperator != nil {
		conf.OfferOperator = *in.OfferOperator
	}
	if (conf.LanguagePolicy != "auto" && conf.LanguagePolicy != "fixed" && conf.LanguagePolicy != "default") || !config.Locale(conf.ResponseLocale) {
		return SettingsResponse{}, core.Fail("validation", 422)
	}
	if e = s.Store.SaveSettings(ctx, v.OrganizationID, v.AssistantID, conf); e != nil {
		return SettingsResponse{}, e
	}
	return s.Settings(ctx, user, v)
}
func (s *Service) Create(ctx context.Context, user string, v Scope) (Conversation, error) {
	if e := s.Access(ctx, user, v, true); e != nil {
		return Conversation{}, e
	}
	return s.Store.Create(ctx, user, v.OrganizationID, v.AssistantID)
}
func (s *Service) List(ctx context.Context, user string, v Scope) ([]Conversation, error) {
	if e := s.Access(ctx, user, v, false); e != nil {
		return nil, e
	}
	return s.Store.List(ctx, v.OrganizationID, v.AssistantID)
}
func (s *Service) Detail(ctx context.Context, user string, v Scope, before int64) (Detail, error) {
	if e := s.Access(ctx, user, v, false); e != nil {
		return Detail{}, e
	}
	return s.Store.Detail(ctx, v, before)
}
func (s *Service) Cancel(ctx context.Context, user string, v Scope) error {
	if e := s.Access(ctx, user, v, true); e != nil {
		return e
	}
	return s.cancel(ctx, v)
}
func (s *Service) Delete(ctx context.Context, user string, v Scope) error {
	if e := s.Cancel(ctx, user, v); e != nil {
		return e
	}
	return s.Store.Delete(ctx, v)
}

var requestUUID = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

func Error(e error) error {
	if errors.Is(e, context.DeadlineExceeded) {
		return core.Fail("ai_timeout", 504)
	}
	if errors.Is(e, context.Canceled) {
		return core.Fail("ai_canceled", 409)
	}
	return core.TranslateError(e)
}
func ErrorIfPresent(e error) error {
	if e == nil {
		return nil
	}
	return Error(e)
}
func (s *Service) Message(parent context.Context, user string, v Scope, in MessageInput, stream bool, emit func(Event) error) (result Result, err error) {
	if e := s.Access(parent, user, v, true); e != nil {
		return result, e
	}
	return s.generate(parent, user, v, in, stream, emit, false)
}
func (s *Service) generate(parent context.Context, user string, v Scope, in MessageInput, stream bool, emit func(Event) error, website bool) (result Result, err error) {
	if !requestUUID.MatchString(in.RequestID) || strings.TrimSpace(in.Content) == "" || !utf8.ValidString(in.Content) || utf8.RuneCountInString(in.Content) > 2000 || strings.ContainsRune(in.Content, 0) {
		return result, core.Fail("validation", 422)
	}
	if s.Config.Provider == "disabled" && !website {
		return result, core.Fail("ai_disabled", 503)
	}
	if !website {
		select {
		case s.slots <- struct{}{}:
			defer func() { <-s.slots }()
		default:
			return result, core.Fail("ai_busy", 503)
		}
	}
	if e := s.Store.Recover(parent); e != nil {
		return result, e
	}
	digest := sha256.Sum256([]byte(in.Content))
	var budget *WebsiteLimits
	if website {
		budget = &s.WebsiteLimits
	}
	result, err = s.Store.begin(parent, v, in, DetectLanguage(in.Content), hex.EncodeToString(digest[:]), s.Config.Provider, s.model(), s.Config.Timeout, budget)
	if err != nil {
		return result, err
	}
	if result.Replay {
		if result.Execution.Status == "processing" {
			return result, core.Fail("request_in_progress", 409)
		}
		return result, nil
	}
	if result.Manual {
		return result, nil
	}
	start := time.Now()
	id := result.Execution.ID
	ctx, cancel := context.WithTimeout(parent, s.Config.Timeout)
	defer cancel()
	s.mu.Lock()
	s.active[id] = cancel
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.active, id); s.mu.Unlock() }()
	defer func() {
		if err != nil {
			if contextError := ctx.Err(); contextError != nil {
				err = contextError
			}
			classified := Error(err)
			var f *core.Fault
			errors.As(classified, &f)
			state := "failed"
			if f.Code == "ai_canceled" {
				state = "canceled"
			}
			finish, done := context.WithTimeout(context.Background(), 5*time.Second)
			defer done()
			s.Store.Finish(finish, v, id, state, "", "", f.Code, time.Since(start).Milliseconds(), Usage{})
			err = classified
		}
	}()
	if website {
		select {
		case s.slots <- struct{}{}:
			defer func() { <-s.slots }()
		default:
			return result, core.Fail("ai_busy", 503)
		}
	}
	// Cross-process cancellation and deletion become observable without waiting for an inference chunk.
	watchCtx, stopWatch := context.WithCancel(ctx)
	watchDone := make(chan struct{})
	defer func() { stopWatch(); <-watchDone }()
	go func() {
		defer close(watchDone)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-watchCtx.Done():
				return
			case <-ticker.C:
				if !s.Store.Running(watchCtx, v, id) {
					if watchCtx.Err() != nil {
						return
					}
					cancel()
					return
				}
			}
		}
	}()
	if stream {
		startEvent := func() error { return emit(Event{Type: "start", Data: result}) }
		if website {
			err = s.Store.Deliver(ctx, v, id, false, startEvent)
		} else {
			err = startEvent()
		}
		if err != nil {
			return result, err
		}
	}
	settings, e := s.Store.Settings(ctx, v.OrganizationID, v.AssistantID)
	if e != nil {
		return result, e
	}
	assistant, e := s.Auth.Store.Assistant(ctx, v.OrganizationID, v.AssistantID)
	if e != nil {
		return result, e
	}
	if settings.DisplayName == "" {
		settings.DisplayName = ClipRunes(assistant.Name, 120)
	}
	history, e := s.Store.History(ctx, v, s.Config.HistoryMessages)
	if e != nil {
		return result, e
	}
	var hits []knowledge.Hit
	query := knowledge.SearchInput{Query: clipBytes(in.Content, 2000), Mode: "hybrid"}
	if website {
		retriever, ok := s.Knowledge.(interface {
			Retrieve(context.Context, string, string, knowledge.SearchInput) ([]knowledge.Hit, error)
		})
		if !ok {
			return result, core.Fail("internal", 500)
		}
		hits, e = retriever.Retrieve(ctx, v.OrganizationID, v.AssistantID, query)
	} else {
		hits, e = s.Knowledge.Search(ctx, user, v.OrganizationID, v.AssistantID, query)
	}
	if e != nil {
		return result, e
	}
	request, sources, e := BuildPrompt(s.Config, settings, assistant.Locale, in.Content, history, hits)
	if e != nil {
		return result, e
	}
	if err = s.Store.Context(ctx, v, id, settings, sources, request.Language); err != nil {
		return result, err
	}
	delta := func(text string) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		if stream {
			send := func() error { return emit(Event{Type: "delta", Data: map[string]string{"text": text}}) }
			if website {
				return s.Store.Deliver(ctx, v, id, false, send)
			}
			return send()
		}
		return nil
	}
	var answer GenerateResult
	if !request.HasKnowledge && s.Config.Provider != "mock" {
		answer.Content = request.Fallback
		if err = delta(answer.Content); err != nil {
			return result, err
		}
	} else {
		answer, err = s.Provider.Generate(ctx, request, stream, delta)
		if err != nil {
			return result, err
		}
	}
	if e = ctx.Err(); e != nil {
		return result, e
	}
	if strings.TrimSpace(answer.Content) == "" || !utf8.ValidString(answer.Content) || strings.ContainsRune(answer.Content, 0) || utf8.RuneCountInString(answer.Content) > max(1024, s.Config.MaxOutputTokens*16) {
		return result, core.Fail("ai_invalid_response", 502)
	}
	stopWatch()
	<-watchDone
	if website && !s.Store.Running(ctx, v, id) {
		return result, core.Fail("ai_canceled", 409)
	}
	if err = s.Store.Finish(ctx, v, id, "completed", answer.Content, request.Language, "", time.Since(start).Milliseconds(), answer.Usage); err != nil {
		return result, err
	}
	result, err = s.Store.Result(ctx, v, id)
	return result, err
}
func (s *Service) Run(ctx context.Context) {
	s.Store.Recover(ctx)
	timer := time.NewTicker(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			s.mu.Lock()
			for _, cancel := range s.active {
				cancel()
			}
			s.mu.Unlock()
			return
		case <-timer.C:
			s.Store.Recover(ctx)
		}
	}
}
