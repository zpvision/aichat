package knowledge

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"testing"
)

func TestQueueQuotaAndFencing(t *testing.T) {
	s, user, org, a := setup(t)
	ctx := context.Background()
	s.Store.Config.MaxJobs = 1
	v, e := s.Article(ctx, user, org, a, Input{Title: str("First"), Content: str("First knowledge")})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Article(ctx, user, org, a, Input{Title: str("Second"), Content: str("Other knowledge")}); e == nil {
		t.Fatal("job quota bypass")
	}
	j, e := s.Store.Claim(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Store.Claim(ctx); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("parallel claim of same job", e)
	}
	s.Store.DB.Exec(ctx, "UPDATE knowledge_jobs SET lease_until=now()-interval '1 minute' WHERE id=$1", j.ID)
	newJob, e := s.Store.Claim(ctx)
	if e != nil || newJob.Token == j.Token {
		t.Fatal("lease token not rotated", e)
	}
	emb, _ := s.Provider.Embed(ctx, []string{v.Content})
	if e = s.Store.Complete(ctx, j, v, []Page{{Text: v.Content}}, []Chunk{{Text: v.Content}}, emb); e == nil {
		t.Fatal("stale worker published index")
	}
	s.Store.Failure(ctx, newJob, "embedding_unavailable", true)
	s.Store.DB.Exec(ctx, "UPDATE knowledge_jobs SET available_at=now() WHERE id=$1", j.ID)
	third, e := s.Store.Claim(ctx)
	if e != nil {
		t.Fatal(e)
	}
	s.Store.Failure(ctx, third, "embedding_unavailable", true)
	v, _ = s.Get(ctx, user, org, a, v.ID)
	if v.Status != "failed" {
		t.Fatal("retry limit not applied", v.Status)
	}
	s.Store.Config.MaxSources = 1
	if _, e = s.Article(ctx, user, org, a, Input{Title: str("Second"), Content: str("Other knowledge")}); e == nil {
		t.Fatal("source quota bypass")
	}
}
