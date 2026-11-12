package knowledge

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"
)

type Config struct {
	BaseURL, Model, FileRoot                                         string
	MaxUpload                                                        int64
	ChunkChars, Overlap, Batch, MaxPages, Depth, MaxSources, MaxJobs int
	Timeout                                                          time.Duration
	MinSimilarity                                                    float64
}

func LoadConfig() (Config, error) {
	c := Config{BaseURL: env("OLLAMA_BASE_URL", "http://localhost:11434"), Model: env("EMBEDDING_MODEL", "bge-m3"), FileRoot: env("KNOWLEDGE_FILE_ROOT", "./data/knowledge"), MaxUpload: 10 << 20, ChunkChars: 2200, Overlap: 260, Batch: 8, MaxPages: 20, Depth: 2, MaxSources: 500, MaxJobs: 40, Timeout: 120 * time.Second, MinSimilarity: .4}
	if env("EMBEDDING_PROVIDER", "ollama") != "ollama" {
		return c, fmt.Errorf("invalid EMBEDDING_PROVIDER")
	}
	u, e := url.Parse(c.BaseURL)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return c, fmt.Errorf("invalid OLLAMA_BASE_URL")
	}
	for _, v := range []struct {
		k        string
		p        *int
		min, max int
	}{{"KNOWLEDGE_CHUNK_CHARS", &c.ChunkChars, 400, 6000}, {"KNOWLEDGE_OVERLAP_CHARS", &c.Overlap, 0, 1000}, {"EMBEDDING_BATCH_SIZE", &c.Batch, 1, 32}, {"IMPORT_MAX_PAGES", &c.MaxPages, 1, 20}, {"IMPORT_MAX_DEPTH", &c.Depth, 0, 5}, {"KNOWLEDGE_MAX_SOURCES", &c.MaxSources, 1, 10000}, {"KNOWLEDGE_MAX_JOBS", &c.MaxJobs, 1, 1000}} {
		if s := os.Getenv(v.k); s != "" {
			n, e := strconv.Atoi(s)
			if e != nil || n < v.min || n > v.max {
				return c, fmt.Errorf("invalid %s", v.k)
			}
			*v.p = n
		}
	}
	if v := os.Getenv("KNOWLEDGE_MAX_UPLOAD_MB"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil || n < 1 || n > 50 {
			return c, fmt.Errorf("invalid KNOWLEDGE_MAX_UPLOAD_MB")
		}
		c.MaxUpload = int64(n) << 20
	}
	if v := os.Getenv("RETRIEVAL_MIN_SIMILARITY"); v != "" {
		n, e := strconv.ParseFloat(v, 64)
		if e != nil || !(n >= 0 && n <= 1) {
			return c, fmt.Errorf("invalid RETRIEVAL_MIN_SIMILARITY")
		}
		c.MinSimilarity = n
	}
	if c.Overlap >= c.ChunkChars/2 || c.Model == "" || c.FileRoot == "" {
		return c, fmt.Errorf("invalid knowledge configuration")
	}
	return c, nil
}
func env(k, d string) string {
	if s, ok := os.LookupEnv(k); ok {
		return s
	}
	return d
}
