package ai

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"site-assistant/internal/core"
	"site-assistant/internal/knowledge"
	"time"
)

type Store struct{ DB *pgxpool.Pool }

func rowJSON[T any](ctx context.Context, s *Store, q string, args ...any) (T, error) {
	var out T
	var b []byte
	e := s.DB.QueryRow(ctx, q, args...).Scan(&b)
	if e == nil {
		e = json.Unmarshal(b, &out)
	}
	return out, e
}
func rowsJSON[T any](ctx context.Context, s *Store, q string, args ...any) ([]T, error) {
	out := []T{}
	rows, e := s.DB.Query(ctx, q, args...)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var b []byte
		var v T
		if e = rows.Scan(&b); e != nil {
			return out, e
		}
		if e = json.Unmarshal(b, &v); e != nil {
			return out, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) Settings(ctx context.Context, org, a string) (Settings, error) {
	v, e := rowJSON[Settings](ctx, s, "SELECT row_to_json(s) FROM assistant_ai_settings s WHERE organization_id=$1 AND assistant_id=$2", org, a)
	if errors.Is(e, pgx.ErrNoRows) {
		return Settings{LanguagePolicy: "auto", ResponseLocale: "en", OfferOperator: true}, nil
	}
	return v, e
}
func (s *Store) SaveSettings(ctx context.Context, org, a string, v Settings) error {
	_, e := s.DB.Exec(ctx, `INSERT INTO assistant_ai_settings(organization_id,assistant_id,display_name,role,company_description,style,instructions,no_answer,language_policy,response_locale,offer_operator) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT(organization_id,assistant_id) DO UPDATE SET display_name=EXCLUDED.display_name,role=EXCLUDED.role,company_description=EXCLUDED.company_description,style=EXCLUDED.style,instructions=EXCLUDED.instructions,no_answer=EXCLUDED.no_answer,language_policy=EXCLUDED.language_policy,response_locale=EXCLUDED.response_locale,offer_operator=EXCLUDED.offer_operator,updated_at=now()`, org, a, v.DisplayName, v.Role, v.CompanyDescription, v.Style, v.Instructions, v.NoAnswer, v.LanguagePolicy, v.ResponseLocale, v.OfferOperator)
	return e
}
func (s *Store) Create(ctx context.Context, user, org, a string) (Conversation, error) {
	id := core.ID()
	_, e := s.DB.Exec(ctx, "INSERT INTO conversations(id,organization_id,assistant_id,created_by) VALUES($1,$2,$3,$4)", id, org, a, user)
	if e != nil {
		return Conversation{}, e
	}
	return s.Conversation(ctx, Scope{org, a, id})
}
func (s *Store) List(ctx context.Context, org, a string) ([]Conversation, error) {
	return rowsJSON[Conversation](ctx, s, "SELECT row_to_json(c) FROM conversations c WHERE organization_id=$1 AND assistant_id=$2 AND kind='preview' ORDER BY updated_at DESC,id LIMIT 100", org, a)
}
func (s *Store) Conversation(ctx context.Context, v Scope) (Conversation, error) {
	return s.conversation(ctx, v, true)
}
func (s *Store) conversation(ctx context.Context, v Scope, preview bool) (Conversation, error) {
	return rowJSON[Conversation](ctx, s, "SELECT row_to_json(c) FROM conversations c WHERE organization_id=$1 AND assistant_id=$2 AND id=$3 AND ($4=false OR kind='preview')", v.OrganizationID, v.AssistantID, v.ConversationID, preview)
}
func (s *Store) Detail(ctx context.Context, v Scope, before int64) (Detail, error) {
	return s.detail(ctx, v, before, true)
}
func (s *Store) detail(ctx context.Context, v Scope, before int64, preview bool) (Detail, error) {
	var d Detail
	var e error
	d.Conversation, e = s.conversation(ctx, v, preview)
	if e != nil {
		return d, e
	}
	d.Messages, e = rowsJSON[Message](ctx, s, "SELECT row_to_json(m) FROM conversation_messages m WHERE organization_id=$1 AND assistant_id=$2 AND conversation_id=$3 AND ($4::bigint=0 OR sequence<$4) ORDER BY sequence DESC LIMIT 101", v.OrganizationID, v.AssistantID, v.ConversationID, before)
	if e != nil {
		return d, e
	}
	if len(d.Messages) > 100 {
		d.Messages = d.Messages[:100]
		cursor := d.Messages[len(d.Messages)-1].Sequence
		d.NextBefore = &cursor
	}
	for i, j := 0, len(d.Messages)-1; i < j; i, j = i+1, j-1 {
		d.Messages[i], d.Messages[j] = d.Messages[j], d.Messages[i]
	}
	d.Executions, e = rowsJSON[Execution](ctx, s, "SELECT row_to_json(e) FROM ai_executions e WHERE organization_id=$1 AND assistant_id=$2 AND conversation_id=$3 AND id IN (SELECT execution_id FROM conversation_messages WHERE organization_id=$1 AND assistant_id=$2 AND conversation_id=$3 AND ($4::bigint=0 OR sequence<$4) ORDER BY sequence DESC LIMIT 100)", v.OrganizationID, v.AssistantID, v.ConversationID, before)
	return d, e
}
func (s *Store) History(ctx context.Context, v Scope, limit int) ([]ChatMessage, error) {
	rows, e := s.DB.Query(ctx, "SELECT m.role,m.content FROM conversation_messages m JOIN ai_executions e ON e.id=m.execution_id AND e.organization_id=m.organization_id AND e.assistant_id=m.assistant_id AND e.conversation_id=m.conversation_id WHERE m.organization_id=$1 AND m.assistant_id=$2 AND m.conversation_id=$3 AND e.status='completed' AND m.status='completed' ORDER BY m.sequence DESC LIMIT $4", v.OrganizationID, v.AssistantID, v.ConversationID, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []ChatMessage{}
	for rows.Next() {
		var m ChatMessage
		if e = rows.Scan(&m.Role, &m.Content); e != nil {
			return nil, e
		}
		out = append(out, m)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, rows.Err()
}
func (s *Store) Begin(ctx context.Context, v Scope, in MessageInput, language, hash, provider, model string, timeout time.Duration) (Result, error) {
	return s.begin(ctx, v, in, language, hash, provider, model, timeout, nil)
}
func (s *Store) begin(ctx context.Context, v Scope, in MessageInput, language, hash, provider, model string, timeout time.Duration, budget *WebsiteLimits) (Result, error) {
	out := Result{}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	var status string
	kind := "preview"
	if budget != nil {
		kind = "website"
	}
	e = tx.QueryRow(ctx, "SELECT status FROM conversations WHERE organization_id=$1 AND assistant_id=$2 AND id=$3 AND kind=$4 FOR UPDATE", v.OrganizationID, v.AssistantID, v.ConversationID, kind).Scan(&status)
	if e != nil {
		return out, e
	}
	if status != "open" {
		return out, core.Fail("conversation_closed", 409)
	}
	var priorID, priorHash string
	e = tx.QueryRow(ctx, "SELECT id,request_hash FROM ai_executions WHERE organization_id=$1 AND assistant_id=$2 AND conversation_id=$3 AND request_id=$4", v.OrganizationID, v.AssistantID, v.ConversationID, in.RequestID).Scan(&priorID, &priorHash)
	if e == nil {
		if hash != priorHash {
			return out, core.Fail("request_conflict", 409)
		}
		tx.Rollback(ctx)
		out, e = s.Result(ctx, v, priorID)
		out.Replay = true
		return out, e
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return out, e
	}
	var active bool
	if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM ai_executions WHERE organization_id=$1 AND assistant_id=$2 AND conversation_id=$3 AND status='processing')", v.OrganizationID, v.AssistantID, v.ConversationID).Scan(&active); e != nil {
		return out, e
	}
	if active {
		return out, core.Fail("conversation_busy", 409)
	}
	if budget != nil {
		if e = s.websiteBudget(ctx, tx, v, *budget); e != nil {
			return out, e
		}
	}
	id := core.ID()
	_, e = tx.Exec(ctx, "INSERT INTO ai_executions(id,organization_id,assistant_id,conversation_id,request_id,request_hash,provider,model,status,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'processing',now()+$9 * interval '1 second')", id, v.OrganizationID, v.AssistantID, v.ConversationID, in.RequestID, hash, provider, model, int(timeout.Seconds())+15)
	if e != nil {
		return out, e
	}
	for _, role := range []string{"user", "assistant"} {
		content, state := "", "processing"
		if role == "user" {
			content = in.Content
			state = "completed"
		}
		_, e = tx.Exec(ctx, "INSERT INTO conversation_messages(id,organization_id,assistant_id,conversation_id,execution_id,role,content,language,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)", core.ID(), v.OrganizationID, v.AssistantID, v.ConversationID, id, role, content, language, state)
		if e != nil {
			return out, e
		}
	}
	_, e = tx.Exec(ctx, "UPDATE conversations SET updated_at=now() WHERE organization_id=$1 AND assistant_id=$2 AND id=$3", v.OrganizationID, v.AssistantID, v.ConversationID)
	if e != nil {
		return out, e
	}
	if e = tx.Commit(ctx); e != nil {
		return out, e
	}
	return s.Result(ctx, v, id)
}
func (s *Store) Result(ctx context.Context, v Scope, id string) (Result, error) {
	out := Result{}
	var e error
	out.Execution, e = rowJSON[Execution](ctx, s, "SELECT row_to_json(e) FROM ai_executions e WHERE organization_id=$1 AND assistant_id=$2 AND conversation_id=$3 AND id=$4", v.OrganizationID, v.AssistantID, v.ConversationID, id)
	if e != nil {
		return out, e
	}
	messages, e := rowsJSON[Message](ctx, s, "SELECT row_to_json(m) FROM conversation_messages m WHERE organization_id=$1 AND assistant_id=$2 AND conversation_id=$3 AND execution_id=$4 ORDER BY sequence", v.OrganizationID, v.AssistantID, v.ConversationID, id)
	if e != nil {
		return out, e
	}
	for _, m := range messages {
		if m.Role == "user" {
			out.UserMessage = m
		} else {
			out.AssistantMessage = m
		}
	}
	return out, nil
}
func (s *Store) Context(ctx context.Context, v Scope, id string, settings Settings, sources []knowledge.Hit, language string) error {
	b, _ := json.Marshal(sources)
	conf, _ := json.Marshal(settings)
	tag, e := s.DB.Exec(ctx, "UPDATE ai_executions SET sources=$5,settings_snapshot=$6,response_language=$7,policy_fallback=$8 WHERE organization_id=$1 AND assistant_id=$2 AND conversation_id=$3 AND id=$4 AND status='processing'", v.OrganizationID, v.AssistantID, v.ConversationID, id, b, conf, language, len(sources) == 0)
	if e == nil && tag.RowsAffected() == 0 {
		return core.Fail("ai_canceled", 409)
	}
	return e
}
func (s *Store) Finish(ctx context.Context, v Scope, id, status, content, language, code string, duration int64, usage Usage) error {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	tag, e := tx.Exec(ctx, "UPDATE ai_executions SET status=$5,error_code=$6,duration_ms=CASE WHEN $7::bigint=0 THEN GREATEST(0,(extract(epoch FROM now()-created_at)*1000)::bigint) ELSE $7 END,input_tokens=$8,output_tokens=$9,provider_duration_ns=$10,finished_at=now() WHERE organization_id=$1 AND assistant_id=$2 AND conversation_id=$3 AND id=$4 AND status='processing'", v.OrganizationID, v.AssistantID, v.ConversationID, id, status, code, duration, usage.InputTokens, usage.OutputTokens, usage.DurationNS)
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		return core.Fail("ai_canceled", 409)
	}
	_, e = tx.Exec(ctx, "UPDATE conversation_messages SET status=$5,content=$6,language=$7 WHERE organization_id=$1 AND assistant_id=$2 AND conversation_id=$3 AND execution_id=$4 AND role='assistant'", v.OrganizationID, v.AssistantID, v.ConversationID, id, status, content, language)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, "UPDATE conversations SET updated_at=now() WHERE organization_id=$1 AND assistant_id=$2 AND id=$3", v.OrganizationID, v.AssistantID, v.ConversationID)
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (s *Store) Active(ctx context.Context, v Scope) (string, error) {
	var id string
	e := s.DB.QueryRow(ctx, "SELECT id FROM ai_executions WHERE organization_id=$1 AND assistant_id=$2 AND conversation_id=$3 AND status='processing'", v.OrganizationID, v.AssistantID, v.ConversationID).Scan(&id)
	return id, e
}
func (s *Store) Running(ctx context.Context, v Scope, id string) bool {
	var running bool
	e := s.DB.QueryRow(ctx, `SELECT e.status='processing' AND (c.kind='preview' OR EXISTS(SELECT 1 FROM widget_sessions s JOIN widgets w ON w.id=s.widget_id WHERE s.id=c.anonymous_session_id AND s.expires_at>now() AND w.enabled AND s.origin=ANY(w.origins))) FROM ai_executions e JOIN conversations c ON c.id=e.conversation_id AND c.organization_id=e.organization_id AND c.assistant_id=e.assistant_id WHERE e.organization_id=$1 AND e.assistant_id=$2 AND e.conversation_id=$3 AND e.id=$4`, v.OrganizationID, v.AssistantID, v.ConversationID, id).Scan(&running)
	return e == nil && running
}
func (s *Store) Delete(ctx context.Context, v Scope) error {
	tag, e := s.DB.Exec(ctx, "DELETE FROM conversations WHERE organization_id=$1 AND assistant_id=$2 AND id=$3 AND kind='preview'", v.OrganizationID, v.AssistantID, v.ConversationID)
	if e == nil && tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return e
}
func (s *Store) Recover(ctx context.Context) error {
	_, e := s.DB.Exec(ctx, "WITH abandoned AS (UPDATE ai_executions SET status='failed',error_code='ai_interrupted',finished_at=now(),duration_ms=GREATEST(0,(extract(epoch FROM now()-created_at)*1000)::bigint) WHERE status='processing' AND expires_at<now() RETURNING id,organization_id,assistant_id,conversation_id) UPDATE conversation_messages m SET status='failed',content='' FROM abandoned a WHERE m.organization_id=a.organization_id AND m.assistant_id=a.assistant_id AND m.conversation_id=a.conversation_id AND m.execution_id=a.id AND m.role='assistant'")
	return e
}
