package knowledge

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"site-assistant/internal/core"
)

type Store struct {
	DB     *pgxpool.Pool
	Config Config
}

func (s *Store) Get(ctx context.Context, org, assistant, id string) (Source, error) {
	var v Source
	var b []byte
	var key string
	e := s.DB.QueryRow(ctx, "SELECT row_to_json(s),file_key FROM knowledge_sources s WHERE organization_id=$1 AND assistant_id=$2 AND id=$3", org, assistant, id).Scan(&b, &key)
	if e == nil {
		e = json.Unmarshal(b, &v)
		v.FileKey = key
	}
	return v, e
}
func (s *Store) List(ctx context.Context, org, assistant string) ([]Source, error) {
	out := []Source{}
	rows, e := s.DB.Query(ctx, "SELECT to_jsonb(s)-'content'-'file_key' FROM knowledge_sources s WHERE organization_id=$1 AND assistant_id=$2 ORDER BY created_at DESC,id LIMIT $3", org, assistant, s.Config.MaxSources)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var b []byte
		var v Source
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(b, &v); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) Put(ctx context.Context, v Source, create bool) (Source, error) {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return v, e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", v.OrganizationID); e != nil {
		return v, e
	}
	var sources, jobs int
	if e = tx.QueryRow(ctx, "SELECT (SELECT count(*) FROM knowledge_sources WHERE organization_id=$1),(SELECT count(*) FROM knowledge_jobs WHERE organization_id=$1 AND state IN ('pending','processing'))", v.OrganizationID).Scan(&sources, &jobs); e != nil {
		return v, e
	}
	if (create && sources >= s.Config.MaxSources) || jobs >= s.Config.MaxJobs {
		return v, core.Fail("knowledge_limit", 429)
	}
	if create {
		_, e = tx.Exec(ctx, "INSERT INTO knowledge_sources(id,organization_id,assistant_id,kind,title,language,url,file_key,format,enabled,content,checksum,crawl) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)", v.ID, v.OrganizationID, v.AssistantID, v.Kind, v.Title, v.Language, v.URL, v.FileKey, v.Format, v.Enabled, v.Content, v.Checksum, v.Crawl)
		v.Revision = 1
	} else {
		e = tx.QueryRow(ctx, "UPDATE knowledge_sources SET title=$4,language=$5,enabled=$6,content=$7,checksum=$8,status='pending',error_code='',revision=revision+1,updated_at=now() WHERE organization_id=$1 AND assistant_id=$2 AND id=$3 AND revision=$9 RETURNING revision", v.OrganizationID, v.AssistantID, v.ID, v.Title, v.Language, v.Enabled, v.Content, v.Checksum, v.Revision).Scan(&v.Revision)
	}
	if e != nil {
		return v, e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO knowledge_jobs(id,organization_id,assistant_id,source_id,revision) VALUES($1,$2,$3,$4,$5)", core.ID(), v.OrganizationID, v.AssistantID, v.ID, v.Revision); e != nil {
		return v, e
	}
	if e = tx.Commit(ctx); e != nil {
		return v, e
	}
	return s.Get(ctx, v.OrganizationID, v.AssistantID, v.ID)
}
func (s *Store) Delete(ctx context.Context, org, assistant, id string) (string, error) {
	var key string
	e := s.DB.QueryRow(ctx, "DELETE FROM knowledge_sources WHERE organization_id=$1 AND assistant_id=$2 AND id=$3 RETURNING file_key", org, assistant, id).Scan(&key)
	return key, e
}
func (s *Store) ByURL(ctx context.Context, org, assistant, url string) (Source, error) {
	var id string
	e := s.DB.QueryRow(ctx, "SELECT id FROM knowledge_sources WHERE organization_id=$1 AND assistant_id=$2 AND url=$3 AND kind='web'", org, assistant, url).Scan(&id)
	if e != nil {
		return Source{}, e
	}
	return s.Get(ctx, org, assistant, id)
}
func (s *Store) Claim(ctx context.Context) (Job, error) {
	var j Job
	j.Token = core.ID()
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return j, e
	}
	defer tx.Rollback(ctx)
	// A crashed final attempt becomes visibly failed. Older revisions cannot alter the latest source state.
	_, e = tx.Exec(ctx, "WITH expired AS (UPDATE knowledge_jobs SET state='failed',error_code='processing_interrupted' WHERE state='processing' AND lease_until<now() AND attempts>=3 RETURNING source_id,revision) UPDATE knowledge_sources s SET status='failed',error_code='processing_interrupted' FROM expired e WHERE s.id=e.source_id AND s.revision=e.revision")
	if e != nil {
		return j, e
	}
	e = tx.QueryRow(ctx, "WITH next AS (SELECT id FROM knowledge_jobs WHERE attempts<3 AND ((state='pending' AND available_at<=now()) OR (state='processing' AND lease_until<now())) ORDER BY available_at,id FOR UPDATE SKIP LOCKED LIMIT 1) UPDATE knowledge_jobs j SET state='processing',attempts=attempts+1,lease_until=now()+interval '5 minutes',lease_token=$1 FROM next WHERE j.id=next.id RETURNING j.id,j.organization_id,j.assistant_id,j.source_id,j.revision,j.attempts", j.Token).Scan(&j.ID, &j.OrganizationID, &j.AssistantID, &j.SourceID, &j.Revision, &j.Attempts)
	if e != nil {
		return j, e
	}
	_, e = tx.Exec(ctx, "UPDATE knowledge_sources SET status='processing',error_code='' WHERE organization_id=$1 AND assistant_id=$2 AND id=$3 AND revision=$4", j.OrganizationID, j.AssistantID, j.SourceID, j.Revision)
	if e != nil {
		return j, e
	}
	return j, tx.Commit(ctx)
}
func (s *Store) Failure(ctx context.Context, j Job, code string, retry bool) error {
	state := "failed"
	if retry && j.Attempts < 3 {
		state = "pending"
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	tag, e := tx.Exec(ctx, "UPDATE knowledge_jobs SET state=$3,error_code=$4,available_at=now()+($5 * interval '10 seconds'),lease_until=NULL WHERE id=$1 AND lease_token=$2 AND state='processing'", j.ID, j.Token, state, code, j.Attempts)
	if e != nil || tag.RowsAffected() == 0 {
		return e
	}
	_, e = tx.Exec(ctx, "UPDATE knowledge_sources SET status=$5,error_code=$6 WHERE organization_id=$1 AND assistant_id=$2 AND id=$3 AND revision=$4", j.OrganizationID, j.AssistantID, j.SourceID, j.Revision, state, code)
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (s *Store) Complete(ctx context.Context, j Job, source Source, pages []Page, chunks []Chunk, emb Embeddings) error {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var revision int
	e = tx.QueryRow(ctx, "SELECT revision FROM knowledge_sources WHERE organization_id=$1 AND assistant_id=$2 AND id=$3 FOR UPDATE", j.OrganizationID, j.AssistantID, j.SourceID).Scan(&revision)
	if e != nil {
		return e
	}
	var token string
	if e = tx.QueryRow(ctx, "SELECT lease_token FROM knowledge_jobs WHERE id=$1 AND state='processing' FOR UPDATE", j.ID).Scan(&token); e != nil {
		return e
	}
	if token != j.Token {
		return core.Fail("processing_interrupted", 409)
	}
	if revision == j.Revision {
		for k, c := range chunks {
			var page any
			if c.Page > 0 {
				page = c.Page
			}
			_, e = tx.Exec(ctx, "INSERT INTO knowledge_chunks(id,organization_id,assistant_id,source_id,revision,position,page,content,embedding,model,model_version,dimensions) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::public.vector,$10,$11,$12)", core.ID(), j.OrganizationID, j.AssistantID, j.SourceID, j.Revision, c.Position, page, c.Text, vector(emb.Vectors[k]), emb.Model, emb.Version, emb.Dimensions)
			if e != nil {
				return e
			}
		}
		var content string
		for _, p := range pages {
			if content != "" {
				content += "\n\n"
			}
			content += p.Text
		}
		_, e = tx.Exec(ctx, "UPDATE knowledge_sources SET content=$5,title=$6,language=$7,active_revision=$4,status='ready',error_code=$8,updated_at=now() WHERE organization_id=$1 AND assistant_id=$2 AND id=$3 AND revision=$4", j.OrganizationID, j.AssistantID, j.SourceID, j.Revision, content, source.Title, source.Language, source.ErrorCode)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, "DELETE FROM knowledge_chunks WHERE organization_id=$1 AND assistant_id=$2 AND source_id=$3 AND revision<>$4", j.OrganizationID, j.AssistantID, j.SourceID, j.Revision)
		if e != nil {
			return e
		}
	}
	_, e = tx.Exec(ctx, "UPDATE knowledge_jobs SET state='ready',lease_until=NULL,error_code='' WHERE id=$1 AND lease_token=$2", j.ID, j.Token)
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (s *Store) ImportProgress(ctx context.Context, j Job, n int) error {
	_, e := s.DB.Exec(ctx, "UPDATE knowledge_sources SET imported_pages=$5 WHERE organization_id=$1 AND assistant_id=$2 AND id=$3 AND revision=$4", j.OrganizationID, j.AssistantID, j.SourceID, j.Revision, n)
	return e
}
func (s *Store) Candidates(ctx context.Context, org, assistant, query, mode string, emb Embeddings, minSimilarity float64) ([]Hit, error) {
	out := []Hit{}
	base := `SELECT c.id,c.source_id,s.title,c.content,s.url,c.page,c.revision,c.model,c.model_version,s.language,1-(c.embedding <=> $3::public.vector) AS similarity FROM knowledge_chunks c JOIN knowledge_sources s ON s.organization_id=c.organization_id AND s.assistant_id=c.assistant_id AND s.id=c.source_id AND s.active_revision=c.revision WHERE c.organization_id=$1 AND c.assistant_id=$2 AND s.enabled AND c.model=$4 AND c.model_version=$5 AND c.dimensions=$6 `
	fetch := func(lexical bool) ([]Hit, error) {
		q := base
		args := []any{org, assistant, vector(emb.Vectors[0]), emb.Model, emb.Version, emb.Dimensions}
		if lexical {
			q += `AND (c.search || to_tsvector('simple',s.title)) @@ websearch_to_tsquery('simple',$7) ORDER BY ts_rank_cd(c.search || to_tsvector('simple',s.title),websearch_to_tsquery('simple',$7)) DESC,c.id LIMIT 30`
			args = append(args, query)
		} else {
			q += `AND 1-(c.embedding <=> $3::public.vector)>=$7 ORDER BY c.embedding <=> $3::public.vector,c.id LIMIT 30`
			args = append(args, minSimilarity)
		}
		rows, e := s.DB.Query(ctx, q, args...)
		if e != nil {
			return nil, e
		}
		defer rows.Close()
		hits := []Hit{}
		for rows.Next() {
			var h Hit
			if e = rows.Scan(&h.ID, &h.SourceID, &h.Title, &h.Content, &h.URL, &h.Page, &h.Revision, &h.Model, &h.ModelVersion, &h.Language, &h.Similarity); e != nil {
				return nil, e
			}
			hits = append(hits, h)
		}
		return hits, rows.Err()
	}
	semantic, e := fetch(false)
	if e != nil {
		return out, e
	}
	if mode == "semantic" {
		for k := range semantic {
			semantic[k].Score = semantic[k].Similarity
		}
		return semantic[:min(5, len(semantic))], nil
	}
	lexical, e := fetch(true)
	if e != nil {
		return out, e
	}
	return Fuse(semantic, lexical), nil
}
