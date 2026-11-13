package leads

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"site-assistant/internal/ai"
	"site-assistant/internal/core"
)

func (s *Store) reserve(ctx context.Context, v ai.Scope, execution, message, language string, allowStart bool) (Collection, ai.CollectionPreparation, bool, error) {
	d := Collection{}
	out := ai.CollectionPreparation{}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return d, out, false, e
	}
	defer tx.Rollback(ctx)
	c, privacy, e := s.conversation(ctx, tx, v, true)
	if e != nil {
		return d, out, false, e
	}
	var running bool
	e = tx.QueryRow(ctx, "SELECT status='processing' AND state_version=$2 FROM ai_executions WHERE id=$1 AND organization_id=$3 AND conversation_id=$4", execution, c.Version, v.OrganizationID, v.ConversationID).Scan(&running)
	if e != nil {
		return d, out, false, e
	}
	if c.Mode != "ai_active" || !running {
		return d, out, false, core.Fail("ai_canceled", 409)
	}
	d, e = s.Collection(ctx, tx, v, true)
	if e != nil {
		return d, out, false, e
	}
	if d.State == "idle" {
		conf, e := s.Settings(ctx, tx, v.OrganizationID, v.AssistantID)
		if e != nil {
			return d, out, false, e
		}
		if !conf.Enabled || privacy == "" {
			return d, out, false, nil
		}
		if !allowStart {
			out.Instructions = `{"application_action_available":true,"rule":"Consult normally. If visitor wants to apply, offer the Leave a request action. Never request mandatory contacts before explicit agreement."}`
			return d, out, false, nil
		}
		d, e = s.start(ctx, tx, v, c, privacy, d)
		if e != nil {
			return d, out, false, e
		}
	}
	if d.State != "collecting" && d.State != "confirmation" {
		return d, out, false, nil
	}
	if d.Attempts >= 40 {
		out.Instructions, out.Question = collectionPrompt(d.Snapshot, d.Values, language, "lead_extraction_limit")
		return d, out, false, nil
	}
	if d.LastAttempt != nil && *d.LastAttempt == message {
		return d, out, false, nil
	}
	if _, e = tx.Exec(ctx, "UPDATE lead_collections SET extract_attempts=extract_attempts+1,last_attempt_message_id=$3 WHERE organization_id=$1 AND id=$2", v.OrganizationID, d.ID, message); e != nil {
		return d, out, false, e
	}
	if e = tx.Commit(ctx); e != nil {
		return d, out, false, e
	}
	return d, out, true, nil
}
func (s *Store) sources(ctx context.Context, v ai.Scope) ([]Source, error) {
	rows, e := s.DB.Query(ctx, "SELECT id,sequence,content FROM conversation_messages WHERE organization_id=$1 AND assistant_id=$2 AND conversation_id=$3 AND role='user' AND status='completed' ORDER BY sequence DESC LIMIT 12", v.OrganizationID, v.AssistantID, v.ConversationID)
	if e != nil {
		return nil, e
	}
	sources := []Source{}
	size := 0
	for rows.Next() {
		var source Source
		if e = rows.Scan(&source.ID, &source.Sequence, &source.Content); e != nil {
			rows.Close()
			return nil, e
		}
		if size+len(source.Content) > 24000 {
			break
		}
		size += len(source.Content)
		sources = append(sources, source)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	for i, j := 0, len(sources)-1; i < j; i, j = i+1, j-1 {
		sources[i], sources[j] = sources[j], sources[i]
	}
	return sources, nil
}
func (s *Store) publish(ctx context.Context, tx pgx.Tx, v ai.Scope, d Collection, values map[string]Value, code string) error {
	current, e := s.Collection(ctx, tx, v, true)
	if e != nil {
		return e
	}
	if current.Version != d.Version || (current.State != "collecting" && current.State != "confirmation") {
		return nil
	}
	state := "collecting"
	if code == "" && len(Missing(d.Snapshot, values)) == 0 {
		state = "confirmation"
	}
	old, _ := json.Marshal(d.Values)
	b, _ := json.Marshal(values)
	if string(old) == string(b) && current.State == state && current.Error == code {
		return nil
	}
	_, e = tx.Exec(ctx, "UPDATE lead_collections SET values=$3,state=$4,error_code=$5,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2", v.OrganizationID, d.ID, b, state, code)
	return e
}
