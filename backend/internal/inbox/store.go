package inbox

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"site-assistant/internal/ai"
	"site-assistant/internal/core"
	"sort"
	"strings"
	"time"
)

type Store struct{ DB *pgxpool.Pool }

func (s *Store) Summary(ctx context.Context, org, user, role string) (map[string]int, error) {
	var unread, requested int
	e := s.DB.QueryRow(ctx, `SELECT COALESCE(sum((SELECT count(*) FROM conversation_messages m WHERE m.conversation_id=c.id AND m.role='user' AND m.sequence>COALESCE((SELECT last_sequence FROM conversation_reads WHERE conversation_id=c.id AND user_id=$2),0))),0),count(*) FILTER(WHERE c.control_mode='human_requested') FROM conversations c WHERE c.organization_id=$1 AND c.kind='website' AND ($3<>'operator' OR c.assigned_user_id=$2 OR (c.assigned_user_id IS NULL AND c.control_mode<>'closed'))`, org, user, role).Scan(&unread, &requested)
	return map[string]int{"unread": unread, "requested": requested}, e
}

const selectConversation = `SELECT to_jsonb(c)||jsonb_build_object('site',s.name,'assistant',a.name,'assignee',COALESCE(NULLIF(u.name,''),'Company staff'),'last_content',COALESCE((SELECT content FROM conversation_messages WHERE conversation_id=c.id AND status='completed' ORDER BY sequence DESC LIMIT 1),''),'unread',(SELECT count(*) FROM conversation_messages m WHERE m.conversation_id=c.id AND m.role='user' AND m.sequence>COALESCE((SELECT last_sequence FROM conversation_reads WHERE conversation_id=c.id AND user_id=NULLIF($3,'')::uuid),0)),'generating',EXISTS(SELECT 1 FROM ai_executions WHERE conversation_id=c.id AND status='processing')) FROM conversations c JOIN sites s ON s.id=c.site_id AND s.organization_id=c.organization_id JOIN assistants a ON a.id=c.assistant_id AND a.organization_id=c.organization_id LEFT JOIN users u ON u.id=c.assigned_user_id `

func allowed(c Conversation, user, role string) bool {
	return role != "operator" || c.Assigned != nil && *c.Assigned == user || c.Assigned == nil && c.Mode != "closed"
}
func scanConversation(row pgx.Row) (Conversation, error) {
	var c Conversation
	var b []byte
	e := row.Scan(&b)
	if e == nil {
		e = json.Unmarshal(b, &c)
	}
	return c, e
}
func (s *Store) Conversation(ctx context.Context, org, id, user, role string) (Conversation, error) {
	c, e := scanConversation(s.DB.QueryRow(ctx, selectConversation+"WHERE c.organization_id=$1 AND c.id=$2 AND c.kind='website'", org, id, user))
	if e == nil && !allowed(c, user, role) {
		e = core.Fail("forbidden", 403)
	}
	return c, e
}
func (s *Store) lock(ctx context.Context, tx pgx.Tx, org, id, user, role string) (Conversation, error) {
	c, e := scanConversation(tx.QueryRow(ctx, selectConversation+"WHERE c.organization_id=$1 AND c.id=$2 AND c.kind='website' FOR UPDATE OF c", org, id, user))
	if e == nil && !allowed(c, user, role) {
		e = core.Fail("forbidden", 403)
	}
	return c, e
}
func (s *Store) List(ctx context.Context, org, user, role string, f Filter) (List, error) {
	out := List{Items: []Conversation{}}
	var cursorTime any
	var cursorID string
	if f.Cursor != "" {
		b, e := base64.RawURLEncoding.DecodeString(f.Cursor)
		if e != nil || len(b) > 150 {
			return out, core.Fail("validation", 422)
		}
		parts := strings.Split(string(b), "|")
		if len(parts) != 2 || !uuid.MatchString(parts[1]) {
			return out, core.Fail("validation", 422)
		}
		t, e := time.Parse(time.RFC3339Nano, parts[0])
		if e != nil {
			return out, core.Fail("validation", 422)
		}
		cursorTime = t
		cursorID = parts[1]
	}
	var from, to any
	for _, p := range []struct {
		raw    string
		target *any
		end    bool
	}{{f.From, &from, false}, {f.To, &to, true}} {
		if p.raw != "" {
			v, e := time.Parse("2006-01-02", p.raw)
			if e != nil {
				return out, core.Fail("validation", 422)
			}
			if p.end {
				v = v.AddDate(0, 0, 1)
			}
			*p.target = v
		}
	}
	q := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(f.Query)
	rows, e := s.DB.Query(ctx, selectConversation+`WHERE c.organization_id=$1 AND c.kind='website' AND ($2<>'operator' OR c.assigned_user_id=NULLIF($3,'')::uuid OR (c.assigned_user_id IS NULL AND c.control_mode<>'closed')) AND ($4='' OR c.control_mode=$4) AND ($5='' OR c.site_id=NULLIF($5,'')::uuid) AND ($6='' OR c.assistant_id=NULLIF($6,'')::uuid) AND ($7='' OR c.assigned_user_id=NULLIF($7,'')::uuid) AND ($8='' OR EXISTS(SELECT 1 FROM conversation_messages m WHERE m.conversation_id=c.id AND m.organization_id=c.organization_id AND m.status='completed' AND m.content ILIKE '%'||$8||'%')) AND ($9::timestamptz IS NULL OR c.created_at>=$9) AND ($10::timestamptz IS NULL OR c.created_at<$10) AND ($11::timestamptz IS NULL OR (c.updated_at,c.id)<($11,$12::uuid)) ORDER BY c.updated_at DESC,c.id DESC LIMIT $13`, org, role, user, f.Mode, f.Site, f.Assistant, f.Assigned, q, from, to, cursorTime, nullable(cursorID), f.Limit+1)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		c, e := scanConversation(rows)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, c)
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	if len(out.Items) > f.Limit {
		out.Items = out.Items[:f.Limit]
		c := out.Items[len(out.Items)-1]
		t := c.UpdatedAt
		out.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(t.Format(time.RFC3339Nano) + "|" + c.ID))
	}
	return out, nil
}
func nullable(v string) any {
	if v == "" {
		return nil
	}
	return v
}
func (s *Store) Page(ctx context.Context, org, id, user, role string, after, before int64, public bool) (Page, error) {
	out := Page{Messages: []ai.Message{}, Events: []Event{}}
	if after < 0 || before < 0 || after > 0 && before > 0 {
		return out, core.Fail("validation", 422)
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	out.Conversation, e = scanConversation(tx.QueryRow(ctx, selectConversation+"WHERE c.organization_id=$1 AND c.id=$2 AND c.kind='website' FOR SHARE OF c", org, id, user))
	if e != nil {
		return out, e
	}
	if !public && !allowed(out.Conversation, user, role) {
		return out, core.Fail("forbidden", 403)
	}
	order := "DESC"
	limit := 50
	if after > 0 {
		order = "ASC"
		limit = 100
	}
	rows, e := tx.Query(ctx, `SELECT typ,seq,value FROM(SELECT 'message' typ,change_seq seq,to_jsonb(m)||jsonb_build_object('request_id',COALESCE(m.request_id,(SELECT request_id FROM ai_executions WHERE id=m.execution_id))) value FROM conversation_messages m WHERE organization_id=$1 AND conversation_id=$2 UNION ALL SELECT 'event',sequence,to_jsonb(e) FROM conversation_events e WHERE organization_id=$1 AND conversation_id=$2 AND (NOT $3 OR kind IN ('human_requested','human_active','ai_active','closed'))) changes WHERE ($4::bigint=0 OR seq>$4) AND ($5::bigint=0 OR seq<$5) ORDER BY seq `+order+` LIMIT $6`, org, id, public, after, before, limit+1)
	if e != nil {
		return out, e
	}
	changes := []Change{}
	for rows.Next() {
		var c Change
		if e = rows.Scan(&c.Type, &c.Sequence, &c.Value); e != nil {
			rows.Close()
			return out, e
		}
		changes = append(changes, c)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	if len(changes) > limit {
		changes = changes[:limit]
		out.More = true
	}
	for _, c := range changes {
		if c.Sequence > out.Cursor {
			out.Cursor = c.Sequence
		}
		if out.NextBefore == 0 || c.Sequence < out.NextBefore {
			out.NextBefore = c.Sequence
		}
		if c.Type == "message" {
			var m ai.Message
			if e = json.Unmarshal(c.Value, &m); e != nil {
				return out, e
			}
			out.Messages = append(out.Messages, m)
		} else {
			var ev Event
			if e = json.Unmarshal(c.Value, &ev); e != nil {
				return out, e
			}
			if public {
				ev.Actor = nil
				ev.Target = nil
				ev.Content = ""
			}
			out.Events = append(out.Events, ev)
		}
	}
	sort.Slice(out.Messages, func(i, j int) bool { return out.Messages[i].Sequence < out.Messages[j].Sequence })
	sort.Slice(out.Events, func(i, j int) bool { return out.Events[i].Sequence < out.Events[j].Sequence })
	return out, tx.Commit(ctx)
}
func (s *Store) Action(ctx context.Context, org, id, user, role, kind string, in Action) (Conversation, string, error) {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return Conversation{}, "", e
	}
	defer tx.Rollback(ctx)
	c, e := s.lock(ctx, tx, org, id, user, role)
	if e != nil {
		return c, "", e
	}
	hash := digest([]any{kind, user, in})
	var old string
	e = tx.QueryRow(ctx, "SELECT request_hash FROM conversation_events WHERE organization_id=$1 AND conversation_id=$2 AND request_id=$3", org, id, in.RequestID).Scan(&old)
	if e == nil {
		if old != hash {
			return c, "", core.Fail("request_conflict", 409)
		}
		return c, "", nil
	}
	if e != pgx.ErrNoRows {
		return c, "", e
	}
	if role != "visitor" && in.Version != c.Version {
		return c, "", core.Fail("state_conflict", 409)
	}
	if c.Mode == "closed" {
		return c, "", core.Fail("conversation_closed", 409)
	}
	own := c.Assigned != nil && *c.Assigned == user
	if role == "operator" && kind != "take" && !own {
		return c, "", core.Fail("forbidden", 403)
	}
	mode, assigned := c.Mode, c.Assigned
	switch kind {
	case "take":
		if c.Assigned != nil && !own {
			return c, "", core.Fail("conversation_claimed", 409)
		}
		mode = "human_active"
		assigned = &user
	case "assign":
		if role != "owner" && role != "admin" {
			return c, "", core.Fail("forbidden", 403)
		}
		assigned = nil
		mode = "human_requested"
		if in.Assignee != "" {
			var ok bool
			e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM organization_members WHERE organization_id=$1 AND user_id=$2)", org, in.Assignee).Scan(&ok)
			if e != nil {
				return c, "", e
			}
			if !ok {
				return c, "", core.Fail("validation", 422)
			}
			assigned = &in.Assignee
			mode = "human_active"
		}
	case "resume-ai":
		if c.Mode == "ai_active" {
			return c, "", core.Fail("state_conflict", 409)
		}
		mode = "ai_active"
		assigned = nil
	case "close":
		mode = "closed"
	case "human_requested":
		if role != "visitor" {
			return c, "", core.Fail("forbidden", 403)
		}
		if c.Mode != "ai_active" {
			return c, "", nil
		}
		mode = "human_requested"
	case "note":
	default:
		return c, "", core.Fail("validation", 422)
	}
	eventKind := mode
	if kind == "note" {
		eventKind = "note"
	}
	if kind == "assign" {
		eventKind = "assigned"
	}
	active := ""
	if kind != "note" {
		// Same exclusive row lock as generation begin, delta delivery, and successful Finish.
		_ = tx.QueryRow(ctx, "SELECT id FROM ai_executions WHERE conversation_id=$1 AND organization_id=$2 AND status='processing'", id, org).Scan(&active)
		_, e = tx.Exec(ctx, `WITH stopped AS(UPDATE ai_executions SET status='canceled',error_code='ai_canceled',finished_at=now() WHERE conversation_id=$1 AND organization_id=$2 AND status='processing' RETURNING id) UPDATE conversation_messages SET status='canceled',content='' WHERE execution_id IN(SELECT id FROM stopped) AND role='assistant'`, id, org)
		if e != nil {
			return c, "", e
		}
		_, e = tx.Exec(ctx, `UPDATE conversations SET control_mode=$3,assigned_user_id=$4,state_version=state_version+1,status=CASE WHEN $3='closed' THEN 'closed' ELSE 'open' END,requested_at=CASE WHEN $3='human_requested' THEN now() ELSE requested_at END,closed_at=CASE WHEN $3='closed' THEN now() ELSE NULL END,updated_at=now() WHERE organization_id=$1 AND id=$2`, org, id, mode, assigned)
		if e != nil {
			return c, "", e
		}
	}
	_, e = tx.Exec(ctx, "INSERT INTO conversation_events(id,organization_id,conversation_id,kind,actor_id,target_id,content,request_id,request_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)", core.ID(), org, id, eventKind, nullable(user), assigned, in.Content, in.RequestID, hash)
	if e != nil {
		return c, "", e
	}
	if e = tx.Commit(ctx); e != nil {
		return c, "", e
	}
	c, e = s.Conversation(ctx, org, id, user, "admin")
	return c, active, e
}
func (s *Store) Reply(ctx context.Context, org, id, user, role string, in Action) (ai.Message, error) {
	var m ai.Message
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return m, e
	}
	defer tx.Rollback(ctx)
	c, e := s.lock(ctx, tx, org, id, user, role)
	if e != nil {
		return m, e
	}
	var b []byte
	e = tx.QueryRow(ctx, "SELECT row_to_json(m) FROM conversation_messages m WHERE organization_id=$1 AND conversation_id=$2 AND request_id=$3", org, id, in.RequestID).Scan(&b)
	if e == nil {
		json.Unmarshal(b, &m)
		var author string
		_ = tx.QueryRow(ctx, "SELECT author_user_id FROM conversation_messages WHERE id=$1", m.ID).Scan(&author)
		if m.Role != "operator" || author != user || m.Content != in.Content {
			return m, core.Fail("request_conflict", 409)
		}
		return m, nil
	}
	if e != pgx.ErrNoRows {
		return m, e
	}
	if c.Mode != "human_active" || c.Version != in.Version {
		return m, core.Fail("state_conflict", 409)
	}
	if c.Assigned == nil || (role == "operator" && *c.Assigned != user) {
		return m, core.Fail("forbidden", 403)
	}
	e = tx.QueryRow(ctx, "INSERT INTO conversation_messages(id,organization_id,assistant_id,conversation_id,role,content,status,author_user_id,request_id,language) VALUES($1,$2,$3,$4,'operator',$5,'completed',$6,$7,$8) RETURNING row_to_json(conversation_messages)", core.ID(), org, c.AssistantID, id, in.Content, user, in.RequestID, ai.DetectLanguage(in.Content)).Scan(&b)
	if e != nil {
		return m, e
	}
	json.Unmarshal(b, &m)
	_, e = tx.Exec(ctx, "UPDATE conversations SET last_message_at=now(),updated_at=now() WHERE organization_id=$1 AND id=$2", org, id)
	if e != nil {
		return m, e
	}
	return m, tx.Commit(ctx)
}
func (s *Store) Read(ctx context.Context, org, id, user string, sequence int64) error {
	_, e := s.DB.Exec(ctx, `INSERT INTO conversation_reads(organization_id,conversation_id,user_id,last_sequence) SELECT $1,$2,$3,LEAST($4,COALESCE(max(sequence),0)) FROM conversation_messages WHERE organization_id=$1 AND conversation_id=$2 ON CONFLICT(conversation_id,user_id) DO UPDATE SET last_sequence=GREATEST(conversation_reads.last_sequence,EXCLUDED.last_sequence)`, org, id, user, sequence)
	return e
}
func (s *Store) Members(ctx context.Context, org string) ([]Member, error) {
	out := []Member{}
	rows, e := s.DB.Query(ctx, "SELECT u.id,u.name,m.role FROM organization_members m JOIN users u ON u.id=m.user_id WHERE m.organization_id=$1 ORDER BY u.name,u.id LIMIT 200", org)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var m Member
		if e = rows.Scan(&m.ID, &m.Name, &m.Role); e != nil {
			return out, e
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
