package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"site-assistant/internal/core"
	"strings"
	"time"
	"unicode/utf8"
)

type DisabledProvider struct{}

func (DisabledProvider) Generate(context.Context, GenerateRequest, bool, func(string) error) (GenerateResult, error) {
	return GenerateResult{}, core.Fail("ai_disabled", 503)
}

type MockProvider struct {
	Delay     time.Duration
	FailAfter int
	Failure   error
}

func (m *MockProvider) Generate(ctx context.Context, r GenerateRequest, stream bool, emit func(string) error) (GenerateResult, error) {
	intro := map[string]string{"ru": "Демонстрационный ответ — настоящая AI-модель не подключена. ", "en": "Demonstration response — a real AI model is not connected. ", "cs": "Ukázková odpověď — skutečný AI model není připojen. ", "de": "Demonstrationsantwort — ein echtes AI-Modell ist nicht verbunden. "}[r.Language]
	if intro == "" {
		intro = "Demonstration response — a real AI model is not connected. "
	}
	body := map[string]string{"ru": "Материалы базы знаний переданы в тестовый запрос. Это проверка программного цикла, а не качества ответа модели.", "en": "Knowledge passages were included in the test request. This checks the software flow, not model answer quality.", "cs": "Úryvky znalostí byly předány v testovacím požadavku. Jde o kontrolu softwarového postupu, ne kvality odpovědi modelu.", "de": "Wissensauszüge wurden in die Testanfrage aufgenommen. Dies prüft den Softwareablauf, nicht die Antwortqualität des Modells."}[r.Language]
	if !r.HasKnowledge {
		body = r.Fallback
	}
	if body == "" {
		body = "Test request processed."
	}
	text := intro + body
	runes := []rune(text)
	n := 0
	for start := 0; start < len(runes); start += 16 {
		if m.Delay > 0 {
			timer := time.NewTimer(m.Delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return GenerateResult{}, ctx.Err()
			case <-timer.C:
			}
		}
		if e := ctx.Err(); e != nil {
			return GenerateResult{}, e
		}
		if m.Failure != nil && n >= m.FailAfter {
			return GenerateResult{}, m.Failure
		}
		part := string(runes[start:min(start+16, len(runes))])
		if stream {
			if e := emit(part); e != nil {
				return GenerateResult{}, e
			}
		}
		n++
	}
	return GenerateResult{Content: text}, nil
}

type OllamaProvider struct {
	Config Config
	Client *http.Client
}

func NewOllama(c Config) *OllamaProvider {
	return &OllamaProvider{Config: c, Client: &http.Client{Timeout: c.Timeout, Transport: &http.Transport{Proxy: nil, ResponseHeaderTimeout: c.Timeout}, CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}}
}

type ollamaResponse struct {
	Model   string `json:"model"`
	Error   string `json:"error"`
	Done    bool   `json:"done"`
	Message struct {
		Role      string            `json:"role"`
		Content   string            `json:"content"`
		ToolCalls []json.RawMessage `json:"tool_calls"`
	} `json:"message"`
	PromptEvalCount *int64 `json:"prompt_eval_count"`
	EvalCount       *int64 `json:"eval_count"`
	TotalDuration   *int64 `json:"total_duration"`
}

func (o *OllamaProvider) Generate(ctx context.Context, r GenerateRequest, stream bool, emit func(string) error) (GenerateResult, error) {
	payload := map[string]any{"model": o.Config.Model, "messages": r.Messages, "stream": stream, "think": o.Config.Think, "options": map[string]any{"num_predict": o.Config.MaxOutputTokens, "temperature": o.Config.Temperature}}
	if len(r.JSONSchema) > 0 {
		if !json.Valid(r.JSONSchema) {
			return GenerateResult{}, core.Fail("validation", 422)
		}
		payload["format"] = r.JSONSchema
	}
	b, e := json.Marshal(payload)
	if e != nil {
		return GenerateResult{}, core.Fail("ai_invalid_response", 502)
	}
	req, e := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(o.Config.BaseURL, "/")+"/api/chat", bytes.NewReader(b))
	if e != nil {
		return GenerateResult{}, core.Fail("ai_unavailable", 503)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, e := o.Client.Do(req)
	if e != nil {
		if ctx.Err() != nil {
			return GenerateResult{}, ctx.Err()
		}
		return GenerateResult{}, core.Fail("ai_unavailable", 503)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return GenerateResult{}, core.Fail("ai_unavailable", 503)
	}
	var out strings.Builder
	var usage Usage
	done := false
	accept := func(v ollamaResponse) error {
		if done || v.Error != "" || len(v.Message.ToolCalls) > 0 || (v.Message.Role != "assistant" && !(v.Done && v.Message.Role == "" && v.Message.Content == "")) {
			return core.Fail("ai_invalid_response", 502)
		}
		if v.Model == "" {
			return core.Fail("ai_invalid_response", 502)
		}
		if !utf8.ValidString(v.Message.Content) || strings.ContainsRune(v.Message.Content, 0) || out.Len()+len(v.Message.Content) > 256<<10 {
			return core.Fail("ai_invalid_response", 502)
		}
		out.WriteString(v.Message.Content)
		if stream && v.Message.Content != "" {
			if e := emit(v.Message.Content); e != nil {
				return e
			}
		}
		if v.Done {
			done = true
			usage = Usage{v.PromptEvalCount, v.EvalCount, v.TotalDuration}
			for _, n := range []*int64{usage.InputTokens, usage.OutputTokens, usage.DurationNS} {
				if n != nil && *n < 0 {
					return core.Fail("ai_invalid_response", 502)
				}
			}
		}
		return nil
	}
	if stream {
		scanner := bufio.NewScanner(io.LimitReader(resp.Body, (2<<20)+1))
		scanner.Buffer(make([]byte, 4096), 1<<20)
		size := 0
		for scanner.Scan() {
			line := scanner.Bytes()
			size += len(line) + 1
			if size > 2<<20 || !utf8.Valid(line) {
				return GenerateResult{}, core.Fail("ai_invalid_response", 502)
			}
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			var v ollamaResponse
			if json.Unmarshal(line, &v) != nil {
				return GenerateResult{}, core.Fail("ai_invalid_response", 502)
			}
			if e = accept(v); e != nil {
				return GenerateResult{}, e
			}
		}
		if scanner.Err() != nil {
			return GenerateResult{}, providerError(ctx, scanner.Err())
		}
	} else {
		b, e := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
		if e != nil {
			return GenerateResult{}, providerError(ctx, e)
		}
		var v ollamaResponse
		if len(b) > 2<<20 || !utf8.Valid(b) || json.Unmarshal(b, &v) != nil {
			return GenerateResult{}, core.Fail("ai_invalid_response", 502)
		}
		if e = accept(v); e != nil {
			return GenerateResult{}, e
		}
	}
	if !done || strings.TrimSpace(out.String()) == "" {
		return GenerateResult{}, core.Fail("ai_incomplete", 502)
	}
	return GenerateResult{Content: out.String(), Usage: usage}, nil
}
func providerError(ctx context.Context, e error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return core.Fail("ai_incomplete", 502)
}
