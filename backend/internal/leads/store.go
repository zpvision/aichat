package leads

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"site-assistant/internal/ai"
	"site-assistant/internal/core"
	"site-assistant/internal/inbox"
	"time"
)

type Store struct{ DB *pgxpool.Pool }
type queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func decode[T any](row pgx.Row) (T, error) {
	var v T
	var b []byte
	e := row.Scan(&b)
	if e == nil {
		e = json.Unmarshal(b, &v)
	}
	return v, e
}
func (s *Store) Settings(ctx context.Context, q queryer, org, assistant string) (Settings, error) {
	v, e := decode[Settings](q.QueryRow(ctx, `SELECT to_jsonb(s)||jsonb_build_object('fields',COALESCE((SELECT jsonb_agg(f.definition ORDER BY f.position,f.key) FROM assistant_lead_fields f WHERE f.organization_id=s.organization_id AND f.assistant_id=s.assistant_id),'[]')) FROM assistant_lead_settings s WHERE s.organization_id=$1 AND s.assistant_id=$2`, org, assistant))
	if errors.Is(e, pgx.ErrNoRows) {
		return Defaults(), nil
	}
	return v, e
}
func (s *Store) SaveSettings(ctx context.Context, org, assistant string, in Settings) (Settings, error) {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return in, e
	}
	defer tx.Rollback(ctx)
	var id string
	if e = tx.QueryRow(ctx, "SELECT id FROM assistants WHERE organization_id=$1 AND id=$2 FOR UPDATE", org, assistant).Scan(&id); e != nil {
		return in, e
	}
	old, e := s.Settings(ctx, tx, org, assistant)
	if e != nil {
		return in, e
	}
	if old.Revision != in.Revision {
		return in, core.Fail("state_conflict", 409)
	}
	b, _ := json.Marshal(in.Success)
	_, e = tx.Exec(ctx, `INSERT INTO assistant_lead_settings(organization_id,assistant_id,enabled,revision,title,goal,require_contact,success_messages) VALUES($1,$2,$3,2,$4,$5,$6,$7) ON CONFLICT(organization_id,assistant_id) DO UPDATE SET enabled=$3,revision=assistant_lead_settings.revision+1,title=$4,goal=$5,require_contact=$6,success_messages=$7`, org, assistant, in.Enabled, in.Title, in.Goal, in.RequireContact, b)
	if e != nil {
		return in, e
	}
	if _, e = tx.Exec(ctx, "DELETE FROM assistant_lead_fields WHERE organization_id=$1 AND assistant_id=$2", org, assistant); e != nil {
		return in, e
	}
	for _, f := range in.Fields {
		b, _ = json.Marshal(f)
		if _, e = tx.Exec(ctx, "INSERT INTO assistant_lead_fields(organization_id,assistant_id,key,kind,position,definition) VALUES($1,$2,$3,$4,$5,$6)", org, assistant, f.Key, f.Type, f.Position, b); e != nil {
			return in, e
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return in, e
	}
	return s.Settings(ctx, s.DB, org, assistant)
}
func (s *Store) conversation(ctx context.Context, q queryer, v ai.Scope, lock bool) (inbox.Conversation, string, error) {
	suffix := ""
	if lock {
		suffix = " FOR UPDATE OF c"
	}
	var b []byte
	var privacy string
	e := q.QueryRow(ctx, `SELECT to_jsonb(c),COALESCE(w.settings->>'privacy_url','') FROM conversations c JOIN widgets w ON w.site_id=c.site_id AND w.organization_id=c.organization_id WHERE c.organization_id=$1 AND c.assistant_id=$2 AND c.id=$3 AND c.kind='website'`+suffix, v.OrganizationID, v.AssistantID, v.ConversationID).Scan(&b, &privacy)
	var c inbox.Conversation
	if e == nil {
		e = json.Unmarshal(b, &c)
	}
	return c, privacy, e
}
func (s *Store) Collection(ctx context.Context, q queryer, v ai.Scope, lock bool) (Collection, error) {
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	d, e := decode[Collection](q.QueryRow(ctx, "SELECT to_jsonb(d) FROM lead_collections d WHERE organization_id=$1 AND assistant_id=$2 AND conversation_id=$3"+suffix, v.OrganizationID, v.AssistantID, v.ConversationID))
	if e == pgx.ErrNoRows {
		return Collection{State: "idle", Values: map[string]Value{}}, nil
	}
	return d, e
}
func (s *Store) start(ctx context.Context, tx pgx.Tx, v ai.Scope, c inbox.Conversation, privacy string, old Collection) (Collection, error) {
	if c.Mode == "closed" {
		return old, core.Fail("conversation_closed", 409)
	}
	conf, e := s.Settings(ctx, tx, v.OrganizationID, v.AssistantID)
	if e != nil {
		return old, e
	}
	if !conf.Enabled {
		return old, core.Fail("lead_disabled", 409)
	}
	if privacy == "" {
		return old, core.Fail("lead_privacy_required", 422)
	}
	if old.State == "submitted" {
		return old, core.Fail("lead_submitted", 409)
	}
	if old.State == "collecting" || old.State == "confirmation" {
		return old, nil
	}
	snap, _ := json.Marshal(conf)
	id := old.ID
	if id == "" {
		id = core.ID()
	}
	_, e = tx.Exec(ctx, `INSERT INTO lead_collections(id,organization_id,assistant_id,conversation_id,state,snapshot,privacy_url) VALUES($1,$2,$3,$4,'collecting',$5,$6) ON CONFLICT(conversation_id) DO UPDATE SET state='collecting',version=lead_collections.version+1,snapshot=$5,privacy_url=$6,values='{}',error_code='',extract_attempts=0,last_attempt_message_id=NULL,updated_at=now()`, id, v.OrganizationID, v.AssistantID, v.ConversationID, snap, privacy)
	if e != nil {
		return old, e
	}
	return s.Collection(ctx, tx, v, false)
}
func permitted(c inbox.Conversation, user, role string) bool {
	if role == "visitor" {
		return c.Mode == "ai_active"
	}
	return c.Mode == "human_active" && (role != "operator" || c.Assigned != nil && *c.Assigned == user)
}
func (s *Store) Mutate(ctx context.Context, v ai.Scope, user, role, kind string, in Input) (Collection, inbox.Conversation, error) {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return Collection{}, inbox.Conversation{}, e
	}
	defer tx.Rollback(ctx)
	c, privacy, e := s.conversation(ctx, tx, v, true)
	if e != nil {
		return Collection{}, c, e
	}
	if role == "operator" && (c.Assigned != nil && *c.Assigned != user || c.Assigned == nil && c.Mode == "closed") {
		return Collection{}, c, core.Fail("forbidden", 403)
	}
	d, e := s.Collection(ctx, tx, v, true)
	if e != nil {
		return d, c, e
	}
	if kind == "submit" && d.State == "submitted" {
		if d.LeadID == nil {
			return d, c, core.Fail("lead_deleted", 410)
		}
		if d.ConfirmedVersion == nil || *d.ConfirmedVersion != in.Version {
			return d, c, core.Fail("state_conflict", 409)
		}
		return d, c, nil
	}
	if in.ConversationVersion != c.Version || in.Version != d.Version {
		return d, c, core.Fail("state_conflict", 409)
	}
	if kind != "cancel" && !permitted(c, user, role) {
		return d, c, core.Fail("lead_paused", 409)
	}
	if c.Mode == "closed" && kind != "cancel" {
		return d, c, core.Fail("conversation_closed", 409)
	}
	if kind == "start" {
		d, e = s.start(ctx, tx, v, c, privacy, d)
		if e != nil {
			return d, c, e
		}
		return d, c, tx.Commit(ctx)
	}
	if d.ID == "" || d.State == "canceled" || d.State == "submitted" {
		return d, c, core.Fail("lead_state", 409)
	}
	switch kind {
	case "cancel":
		_, e = tx.Exec(ctx, "UPDATE lead_collections SET state='canceled',values='{}',version=version+1,error_code='',updated_at=now() WHERE id=$1 AND organization_id=$2", d.ID, v.OrganizationID)
	case "edit":
		fields := map[string]Field{}
		for _, f := range d.Snapshot.Fields {
			if f.Active {
				fields[f.Key] = f
			}
		}
		var sequence int64
		if e = tx.QueryRow(ctx, "SELECT COALESCE(max(sequence),0) FROM conversation_messages WHERE organization_id=$1 AND conversation_id=$2 AND role='user'", v.OrganizationID, v.ConversationID).Scan(&sequence); e != nil {
			return d, c, e
		}
		for key, raw := range in.Values {
			f, ok := fields[key]
			if !ok {
				return d, c, core.Fail("lead_unknown_field", 422)
			}
			if string(raw) == "null" {
				delete(d.Values, key)
				continue
			}
			value, code := ValidateValue(f, raw)
			if old,ok:=d.Values[key];ok&&bytes.Equal(old.Value,value)&&old.Valid==(code=="") {continue}
			source := "visitor_edit"
			if role != "visitor" {
				source = "staff"
			}
			d.Values[key] = Value{Value: value, Valid: code == "", Error: code, Source: source, Sequence: sequence, UpdatedAt: time.Now().UTC()}
		}
		state := "collecting"
		if len(Missing(d.Snapshot, d.Values)) == 0 {
			state = "confirmation"
		}
		b, _ := json.Marshal(d.Values)
		_, e = tx.Exec(ctx, "UPDATE lead_collections SET values=$3,state=$4,version=version+1,error_code='',updated_at=now() WHERE id=$1 AND organization_id=$2", d.ID, v.OrganizationID, b, state)
	case "submit":
		if d.State != "confirmation" || d.Error != "" || len(Missing(d.Snapshot, d.Values)) > 0 {
			return d, c, core.Fail("lead_incomplete", 422)
		}
		for _, f := range d.Snapshot.Fields {
			if value, ok := d.Values[f.Key]; ok {
				normalized, code := ValidateValue(f, value.Value)
				if !f.Active || code != "" || !value.Valid {
					return d, c, core.Fail("lead_incomplete", 422)
				}
				value.Value = normalized
				d.Values[f.Key] = value
			}
		}
		e = s.submit(ctx, tx, v, c, user, role, d)
	default:
		return d, c, core.Fail("validation", 422)
	}
	if e != nil {
		return d, c, e
	}
	d, e = s.Collection(ctx, tx, v, false)
	if e != nil {
		return d, c, e
	}
	return d, c, tx.Commit(ctx)
}
func (s *Store) submit(ctx context.Context, tx pgx.Tx, v ai.Scope, c inbox.Conversation, user, role string, d Collection) error {
	contacts := map[string]string{"name": "", "email": "", "phone": ""}
	keys := map[string]string{}
	data := map[string]json.RawMessage{}
	meta := map[string]Value{}
	for _, f := range d.Snapshot.Fields {
		value, ok := d.Values[f.Key]
		if !ok {
			continue
		}
		kind := ""
		if f.Type == "email" || f.Type == "phone" {
			kind = f.Type
		}
		if f.Type == "text" && (f.Key == "full_name" || f.Key == "name") {
			kind = "name"
		}
		if kind != "" && keys[kind] == "" {
			var str string
			json.Unmarshal(value.Value, &str)
			contacts[kind] = str
			keys[kind] = f.Key
		} else {
			data[f.Key] = value.Value
		}
		value.Value = nil
		meta[f.Key] = value
	}
	b, _ := json.Marshal(data)
	provenance, _ := json.Marshal(meta)
	keyJSON, _ := json.Marshal(keys)
	snap, _ := json.Marshal(d.Snapshot)
	id := core.ID()
	source := "website"
	assigned := c.Assigned
	if role != "visitor" {
		source = "staff"
		if assigned == nil {
			assigned = &user
		}
	}
	_, e := tx.Exec(ctx, `INSERT INTO leads(id,organization_id,assistant_id,site_id,conversation_id,collection_id,name,email,phone,data,provenance,contact_keys,snapshot,privacy_url,assigned_user_id,source) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`, id, v.OrganizationID, v.AssistantID, c.SiteID, v.ConversationID, d.ID, contacts["name"], contacts["email"], contacts["phone"], b, provenance, keyJSON, snap, d.PrivacyURL, assigned, source)
	if e != nil {
		return e
	}
	// Stage 7 INSERT trigger atomically creates lead.created and delivery jobs. No HTTP runs in this transaction.
	_, e = tx.Exec(ctx, "INSERT INTO lead_events(id,organization_id,lead_id,actor_id,kind,status) VALUES($1,$2,$3,$4,'created','new')", core.ID(), v.OrganizationID, id, nullable(user))
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, "UPDATE lead_collections SET state='submitted',lead_id=$3,confirmed_version=version,version=version+1,values='{}',error_code='',submitted_at=now(),updated_at=now() WHERE id=$1 AND organization_id=$2", d.ID, v.OrganizationID, id)
	return e
}
func nullable(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func (s *Store) readView(ctx context.Context, v ai.Scope, user, role string) (inbox.Conversation, Collection, Settings, string, error) {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return inbox.Conversation{}, Collection{}, Settings{}, "", e
	}
	defer tx.Rollback(ctx)
	var id string
	if e = tx.QueryRow(ctx, "SELECT id FROM conversations WHERE organization_id=$1 AND assistant_id=$2 AND id=$3 FOR SHARE", v.OrganizationID, v.AssistantID, v.ConversationID).Scan(&id); e != nil {
		return inbox.Conversation{}, Collection{}, Settings{}, "", e
	}
	c, privacy, e := s.conversation(ctx, tx, v, false)
	if e != nil {
		return inbox.Conversation{}, Collection{}, Settings{}, "", e
	}
	if role == "operator" && (c.Assigned != nil && *c.Assigned != user || c.Assigned == nil && c.Mode == "closed") {
		return inbox.Conversation{}, Collection{}, Settings{}, "", core.Fail("forbidden", 403)
	}
	d, e := s.Collection(ctx, tx, v, false)
	if e != nil {
		return inbox.Conversation{}, Collection{}, Settings{}, "", e
	}
	conf, e := s.Settings(ctx, tx, v.OrganizationID, v.AssistantID)
	if e != nil {
		return inbox.Conversation{}, Collection{}, Settings{}, "", e
	}
	if e = tx.Commit(ctx); e != nil {
		return inbox.Conversation{}, Collection{}, Settings{}, "", e
	}
	return c, d, conf, privacy, nil
}
