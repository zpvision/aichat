package ai

import (
	"fmt"
	"math"
	"net/url"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Provider, BaseURL, Model                                                       string
	Timeout                                                                        time.Duration
	MaxOutputTokens, Concurrency, HistoryMessages, ContextChars, RAGChars, RAGTopK int
	Temperature                                                                    float64
	Think                                                                          bool
}

func LoadConfig(production bool) (Config, error) {
	c := Config{Provider: env("AI_PROVIDER", "mock"), BaseURL: env("AI_BASE_URL", "http://localhost:11434"), Model: env("AI_MODEL", "gemma4:26b"), Timeout: 120 * time.Second, MaxOutputTokens: 500, Concurrency: 4, HistoryMessages: 20, ContextChars: 24000, RAGChars: 12000, RAGTopK: 5, Temperature: .3}
	if c.Provider != "mock" && c.Provider != "disabled" && c.Provider != "ollama" {
		return c, fmt.Errorf("invalid AI_PROVIDER")
	}
	if production && c.Provider == "mock" {
		return c, fmt.Errorf("mock AI is forbidden in production; set AI_PROVIDER=disabled or ollama")
	}
	u, e := url.Parse(c.BaseURL)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "http" && u.Scheme != "https") {
		return c, fmt.Errorf("invalid AI_BASE_URL")
	}
	if len(c.Model) == 0 || len(c.Model) > 160 {
		return c, fmt.Errorf("invalid AI_MODEL")
	}
	seconds := 120
	for _, v := range []struct {
		k        string
		p        *int
		min, max int
	}{{"AI_TIMEOUT_SECONDS", &seconds, 1, 120}, {"AI_MAX_OUTPUT_TOKENS", &c.MaxOutputTokens, 1, 4096}, {"AI_MAX_CONCURRENT", &c.Concurrency, 1, 16}, {"AI_HISTORY_MESSAGES", &c.HistoryMessages, 0, 100}, {"AI_CONTEXT_CHARS", &c.ContextChars, 12000, 64000}, {"AI_RAG_CHARS", &c.RAGChars, 1000, 20000}, {"AI_RAG_TOP_K", &c.RAGTopK, 1, 5}} {
		if s := os.Getenv(v.k); s != "" {
			n, e := strconv.Atoi(s)
			if e != nil || n < v.min || n > v.max {
				return c, fmt.Errorf("invalid %s", v.k)
			}
			*v.p = n
		}
	}
	c.Timeout = time.Duration(seconds) * time.Second
	if s := os.Getenv("AI_TEMPERATURE"); s != "" {
		c.Temperature, e = strconv.ParseFloat(s, 64)
		if e != nil || math.IsNaN(c.Temperature) || c.Temperature < 0 || c.Temperature > 2 {
			return c, fmt.Errorf("invalid AI_TEMPERATURE")
		}
	}
	if s := os.Getenv("AI_THINK"); s != "" {
		c.Think, e = strconv.ParseBool(s)
		if e != nil {
			return c, fmt.Errorf("invalid AI_THINK")
		}
	}
	if c.RAGChars > c.ContextChars-8000 {
		return c, fmt.Errorf("AI_RAG_CHARS leaves insufficient instruction budget")
	}
	return c, nil
}
func env(k, d string) string {
	if s, ok := os.LookupEnv(k); ok {
		return s
	}
	return d
}
