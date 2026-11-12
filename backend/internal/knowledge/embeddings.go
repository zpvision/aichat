package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"site-assistant/internal/core"
	"strings"
	"sync"
)

type Embeddings struct {
	Vectors        [][]float32
	Model, Version string
	Dimensions     int
}
type EmbeddingProvider interface {
	Embed(context.Context, []string) (Embeddings, error)
}
type Ollama struct {
	BaseURL, Model string
	Client         *http.Client
	mu             sync.Mutex
}

func (o *Ollama) request(ctx context.Context, method, path string, in, out any) error {
	var b bytes.Buffer
	if in != nil {
		json.NewEncoder(&b).Encode(in)
	}
	r, e := http.NewRequestWithContext(ctx, method, strings.TrimRight(o.BaseURL, "/")+path, &b)
	if e != nil {
		return core.Fail("embedding_unavailable", 503)
	}
	r.Header.Set("Content-Type", "application/json")
	resp, e := o.Client.Do(r)
	if e != nil {
		return core.Fail("embedding_unavailable", 503)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return core.Fail("embedding_unavailable", 503)
	}
	if e = json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(out); e != nil {
		return core.Fail("embedding_invalid", 503)
	}
	return nil
}
func (o *Ollama) version(ctx context.Context) (string, error) {
	var tags struct {
		Models []struct{ Name, Model, Digest string }
	}
	if e := o.request(ctx, "GET", "/api/tags", nil, &tags); e != nil {
		return "", e
	}
	for _, m := range tags.Models {
		if m.Name == o.Model || m.Name == o.Model+":latest" || m.Model == o.Model {
			return m.Digest, nil
		}
	}
	return "", core.Fail("embedding_unavailable", 503)
}
func (o *Ollama) Embed(ctx context.Context, input []string) (Embeddings, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	var out Embeddings
	v, e := o.version(ctx)
	if e != nil {
		return out, e
	}
	var response struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if e = o.request(ctx, "POST", "/api/embed", map[string]any{"model": o.Model, "input": input, "truncate": false}, &response); e != nil {
		return out, e
	}
	after, e := o.version(ctx)
	if e != nil {
		return out, e
	}
	if v == "" || v != after {
		return out, core.Fail("embedding_changed", 503)
	}
	out = Embeddings{Vectors: response.Embeddings, Model: o.Model, Version: v}
	if len(out.Vectors) > 0 {
		out.Dimensions = len(out.Vectors[0])
	}
	return out, ValidateEmbeddings(out, len(input))
}
func ValidateEmbeddings(v Embeddings, count int) error {
	if len(v.Vectors) != count || v.Dimensions < 1 || v.Dimensions > 16000 || v.Model == "" || v.Version == "" {
		return core.Fail("embedding_invalid", 503)
	}
	for _, row := range v.Vectors {
		if len(row) != v.Dimensions {
			return core.Fail("embedding_invalid", 503)
		}
		norm := float64(0)
		for _, f := range row {
			if math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) {
				return core.Fail("embedding_invalid", 503)
			}
			norm += float64(f) * float64(f)
		}
		if norm < 1e-12 {
			return core.Fail("embedding_invalid", 503)
		}
	}
	return nil
}
func vector(v []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%g", f)
	}
	b.WriteByte(']')
	return b.String()
}
