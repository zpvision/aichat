package knowledge

import (
	"context"
	"encoding/json"
	"os"
	"site-assistant/internal/core"
	"testing"
)

func TestRealMultilingualRetrieval(t *testing.T) {
	if os.Getenv("REAL_EMBEDDINGS") != "1" {
		t.Skip("REAL_EMBEDDINGS=1 requires local Ollama bge-m3; no mock used")
	}
	s, user, org, a := setup(t)
	s.Provider = &Ollama{BaseURL: s.Config.BaseURL, Model: s.Config.Model, Client: embeddingClient(s.Config)}
	ctx := context.Background()
	var corpus struct {
		Documents []struct{ Key, Language, Title, Content string }
		Questions []struct{ Query, Expected, Kind string }
	}
	b, e := os.ReadFile("testdata/multilingual.json")
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(b, &corpus); e != nil {
		t.Fatal(e)
	}
	ids := map[string]string{}
	for _, d := range corpus.Documents {
		v, e := s.Article(ctx, user, org, a, Input{Title: &d.Title, Content: &d.Content, Language: &d.Language})
		if e != nil {
			t.Fatal(e)
		}
		ids[d.Key] = v.ID
		if e = s.ProcessOne(ctx); e != nil {
			t.Fatal(e)
		}
		v, _ = s.Get(ctx, user, org, a, v.ID)
		if v.Status != "ready" {
			t.Fatal("real indexing failed", v.ErrorCode)
		}
	}
	report := map[string]any{"model": s.Config.Model, "dataset": "internal/knowledge/testdata/multilingual.json", "mock": false}
	details := []map[string]any{}
	for _, mode := range []string{"semantic", "hybrid"} {
		correct, total := 0, 0
		for _, q := range corpus.Questions {
			hits, e := s.Search(ctx, user, org, a, SearchInput{Query: q.Query, Mode: mode})
			if e != nil {
				t.Fatal(e)
			}
			found := false
			titles := []string{}
			for _, h := range hits {
				titles = append(titles, h.Title)
				if h.SourceID == ids[q.Expected] {
					found = true
				}
			}
			if q.Expected != "" {
				total++
				if found {
					correct++
				}
			}
			row := map[string]any{"mode": mode, "kind": q.Kind, "query": q.Query, "expected": q.Expected, "found_in_top5": found, "returned": len(hits), "titles": titles}
			details = append(details, row)
			t.Logf("%s %s found=%v hits=%d", mode, q.Kind, found, len(hits))
		}
		report[mode+"_recall_at_5"] = float64(correct) / float64(total)
		t.Logf("%s Recall@5 = %d/%d", mode, correct, total)
	}
	var dimensions int
	var version string
	s.Store.DB.QueryRow(ctx, "SELECT dimensions,model_version FROM knowledge_chunks WHERE organization_id=$1 AND assistant_id=$2 LIMIT 1", org, a).Scan(&dimensions, &version)
	report["dimensions"] = dimensions
	report["model_digest"] = version
	report["results"] = details
	b, _ = json.MarshalIndent(report, "", "  ")
	if path := os.Getenv("RAG_REPORT_PATH"); path != "" {
		if e = os.WriteFile(path, append(b, '\n'), 0600); e != nil {
			t.Fatal(e)
		}
	}
	// A real query in a different assistant/company scope must never return this corpus.
	other, e := s.Auth.SaveOrganization(ctx, user, "", core.OrganizationInput{Name: str("Other tenant"), Country: str("CZ")})
	if e != nil {
		t.Fatal(e)
	}
	as, e := s.Auth.SaveAssistant(ctx, user, other.ID, "", core.AssistantInput{Name: str("Other assistant")})
	if e != nil {
		t.Fatal(e)
	}
	hits, e := s.Search(ctx, user, other.ID, as.ID, SearchInput{Query: "Jantar Plus"})
	if e != nil || len(hits) != 0 {
		t.Fatal("real cross-tenant leak", hits, e)
	}
}
