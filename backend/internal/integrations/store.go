package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"site-assistant/internal/billing"
	"site-assistant/internal/core"
	"site-assistant/internal/leads"
	"strconv"
	"time"
)

type Store struct{ DB *pgxpool.Pool }

func raw(v any) []byte { b, _ := json.Marshal(v); return b }
func (s *Store) CreateKey(ctx context.Context, org, id string, in KeyInput, hash string) error {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SELECT id FROM organizations WHERE id=$1 FOR UPDATE", org); e != nil {
		return e
	}
	var n int
	if e = tx.QueryRow(ctx, "SELECT count(*) FROM integration_keys WHERE organization_id=$1 AND revoked_at IS NULL AND expires_at>now()", org).Scan(&n); e != nil {
		return e
	}
	if n >= 20 {
		return core.Fail("integration_limit", 422)
	}
	_, e = tx.Exec(ctx, "INSERT INTO integration_keys(id,organization_id,name,token_hash,scopes,expires_at) VALUES($1,$2,$3,$4,$5,now()+$6*interval '1 day')", id, org, in.Name, hash, in.Scopes, in.Days)
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (s *Store) Authenticate(ctx context.Context, hash string) (Principal, error) {
	p := Principal{}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return p, e
	}
	defer tx.Rollback(ctx)
	var n int
	e = tx.QueryRow(ctx, `UPDATE integration_keys SET last_used_at=now(),request_count=CASE WHEN window_start<now()-interval '1 minute' THEN 1 ELSE request_count+1 END,window_start=CASE WHEN window_start<now()-interval '1 minute' THEN now() ELSE window_start END WHERE token_hash=$1 AND revoked_at IS NULL AND expires_at>now() RETURNING organization_id,id,scopes,request_count`, hash).Scan(&p.OrganizationID, &p.KeyID, &p.Scopes, &n)
	if errors.Is(e, pgx.ErrNoRows) {
		return p, core.Fail("api_unauthorized", 401)
	}
	if e != nil {
		return p, e
	}
	if n > 120 {
		return p, core.Fail("api_rate", 429)
	}
	e = tx.QueryRow(ctx, `INSERT INTO integration_budgets VALUES($1,now(),1) ON CONFLICT(organization_id) DO UPDATE SET request_count=CASE WHEN integration_budgets.window_start<now()-interval '1 minute' THEN 1 ELSE integration_budgets.request_count+1 END,window_start=CASE WHEN integration_budgets.window_start<now()-interval '1 minute' THEN now() ELSE integration_budgets.window_start END RETURNING request_count`, p.OrganizationID).Scan(&n)
	if e != nil {
		return p, e
	}
	if n > 600 {
		return p, core.Fail("api_rate", 429)
	}
	cap, e := billing.APICap(ctx, tx, p.OrganizationID)
	if e != nil {
		return p, e
	}
	if cap != nil && int64(n) > *cap {
		return p, core.Fail("plan_limit_exceeded", 429)
	}
	return p, tx.Commit(ctx)
}
func (s *Store) Rows(ctx context.Context, org, kind string, cursors ...string) ([]json.RawMessage, error) {
	q := ""
	switch kind {
	case "keys":
		q = "SELECT to_jsonb(k)-'token_hash'-'window_start'-'request_count' FROM integration_keys k WHERE organization_id=$1 ORDER BY (revoked_at IS NULL AND expires_at>now()) DESC,created_at DESC LIMIT 100"
	case "webhooks":
		q = "SELECT to_jsonb(w)-'secret_cipher' FROM webhook_endpoints w WHERE organization_id=$1 ORDER BY enabled DESC,created_at DESC LIMIT 100"
	case "deliveries":
		q = "SELECT to_jsonb(d) FROM webhook_deliveries d WHERE organization_id=$1 ORDER BY created_at DESC,id DESC LIMIT 100"
	}
	args := []any{org}
	if kind == "deliveries" && len(cursors) > 0 && cursors[0] != "" {
		q = "SELECT to_jsonb(d) FROM webhook_deliveries d WHERE organization_id=$1 AND (created_at,id)<(SELECT created_at,id FROM webhook_deliveries WHERE organization_id=$1 AND id=$2) ORDER BY created_at DESC,id DESC LIMIT 100"
		args = append(args, cursors[0])
	}
	rows, e := s.DB.Query(ctx, q, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		out = append(out, json.RawMessage(b))
	}
	return out, rows.Err()
}
func (s *Store) Revoke(ctx context.Context, org, id string) error {
	tag, e := s.DB.Exec(ctx, "UPDATE integration_keys SET revoked_at=COALESCE(revoked_at,now()) WHERE organization_id=$1 AND id=$2", org, id)
	if e == nil && tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return e
}
func (s *Store) CreateEndpoint(ctx context.Context, org, id, url string, secret []byte) error {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SELECT id FROM organizations WHERE id=$1 FOR UPDATE", org); e != nil {
		return e
	}
	var n int
	if e = tx.QueryRow(ctx, "SELECT count(*) FROM webhook_endpoints WHERE organization_id=$1 AND enabled", org).Scan(&n); e != nil {
		return e
	}
	if n >= 5 {
		return core.Fail("integration_limit", 422)
	}
	_, e = tx.Exec(ctx, "INSERT INTO webhook_endpoints(id,organization_id,url,secret_cipher) VALUES($1,$2,$3,$4)", id, org, url, secret)
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (s *Store) Rotate(ctx context.Context, org, id string, b []byte) error {
	tag, e := s.DB.Exec(ctx, "UPDATE webhook_endpoints SET secret_cipher=$3,version=version+1 WHERE organization_id=$1 AND id=$2", org, id, b)
	if e == nil && tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return e
}
func (s *Store) Toggle(ctx context.Context, org, id string, on bool) error {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SELECT id FROM organizations WHERE id=$1 FOR UPDATE", org); e != nil {
		return e
	}
	if on {
		var n int
		if e = tx.QueryRow(ctx, "SELECT count(*) FROM webhook_endpoints WHERE organization_id=$1 AND enabled AND id<>$2", org, id).Scan(&n); e != nil {
			return e
		}
		if n >= 5 {
			return core.Fail("integration_limit", 422)
		}
	}
	tag, e := tx.Exec(ctx, "UPDATE webhook_endpoints SET enabled=$3,version=version+1 WHERE organization_id=$1 AND id=$2", org, id, on)
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return tx.Commit(ctx)
}
func (s *Store) Test(ctx context.Context, org, id string) (string, error) {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return "", e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SELECT id FROM organizations WHERE id=$1 FOR UPDATE", org); e != nil {
		return "", e
	}
	var on bool
	if e = tx.QueryRow(ctx, "SELECT enabled FROM webhook_endpoints WHERE organization_id=$1 AND id=$2", org, id).Scan(&on); e != nil {
		return "", e
	}
	if !on {
		return "", core.Fail("integration_disabled", 409)
	}
	var n int
	if e = tx.QueryRow(ctx, "SELECT count(*) FROM webhook_deliveries WHERE organization_id=$1 AND endpoint_id=$2 AND created_at>now()-interval '1 minute'", org, id).Scan(&n); e != nil {
		return "", e
	}
	if n >= 10 {
		return "", core.Fail("api_rate", 429)
	}
	event, delivery := core.ID(), core.ID()
	if _, e = tx.Exec(ctx, "INSERT INTO integration_events(id,organization_id,event_type) VALUES($1,$2,'webhook.test')", event, org); e != nil {
		return "", e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO webhook_deliveries(id,organization_id,endpoint_id,event_id) VALUES($1,$2,$3,$4)", delivery, org, id, event); e != nil {
		return "", e
	}
	return delivery, tx.Commit(ctx)
}
func (s *Store) Retry(ctx context.Context, org, id string) error {
	tag, e := s.DB.Exec(ctx, "UPDATE webhook_deliveries SET status='pending',max_attempts=attempts+8,next_attempt_at=now(),updated_at=now() WHERE organization_id=$1 AND id=$2 AND status IN ('failed','canceled')", org, id)
	if e == nil && tag.RowsAffected() == 0 {
		return core.Fail("state_conflict", 409)
	}
	return e
}
func (s *Store) Attempts(ctx context.Context, org, id string) ([]json.RawMessage, error) {
	rows, e := s.DB.Query(ctx, "SELECT to_jsonb(a) FROM webhook_attempts a WHERE organization_id=$1 AND delivery_id=$2 ORDER BY created_at DESC LIMIT 100", org, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		out = append(out, json.RawMessage(b))
	}
	return out, rows.Err()
}
func (s *Store) LeadSettings(ctx context.Context, org, id string) (leads.Settings, error) {
	var exists bool
	if e := s.DB.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM assistants WHERE organization_id=$1 AND id=$2)", org, id).Scan(&exists); e != nil {
		return leads.Settings{}, e
	}
	if !exists {
		return leads.Settings{}, pgx.ErrNoRows
	}
	return (&leads.Store{DB: s.DB}).Settings(ctx, s.DB, org, id)
}
func (s *Store) CreateLead(ctx context.Context, org, key, hash string, in CreateInput, conf leads.Settings, values map[string]leads.Value) (ExternalLead, error) {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return ExternalLead{}, e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SELECT id FROM organizations WHERE id=$1 FOR UPDATE", org); e != nil {
		return ExternalLead{}, e
	}
	var id, old string
	e = tx.QueryRow(ctx, "SELECT lead_id,body_hash FROM integration_requests WHERE organization_id=$1 AND request_key=$2", org, key).Scan(&id, &old)
	if e == nil {
		if old != hash {
			return ExternalLead{}, core.Fail("idempotency_conflict", 409)
		}
		if e = tx.Rollback(ctx); e != nil {
			return ExternalLead{}, e
		}
		return s.Lead(ctx, org, id)
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return ExternalLead{}, e
	}
	var site any
	if in.SiteID != "" {
		var valid bool
		if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM sites WHERE organization_id=$1 AND assistant_id=$2 AND id=$3)", org, in.AssistantID, in.SiteID).Scan(&valid); e != nil {
			return ExternalLead{}, e
		}
		if !valid {
			return ExternalLead{}, pgx.ErrNoRows
		}
		site = in.SiteID
	}
	name, email, phone := "", "", ""
	extras := map[string]json.RawMessage{}
	keys := map[string]string{}
	meta := map[string]leads.Value{}
	for _, f := range conf.Fields {
		v, ok := values[f.Key]
		if !ok {
			continue
		}
		kind := ""
		if f.Type == "email" || f.Type == "phone" {
			kind = f.Type
		}
		if f.Type == "text" && (f.Key == "name" || f.Key == "full_name") {
			kind = "name"
		}
		if kind != "" && keys[kind] == "" {
			var str string
			json.Unmarshal(v.Value, &str)
			switch kind {
			case "name":
				name = str
			case "email":
				email = str
			case "phone":
				phone = str
			}
			keys[kind] = f.Key
		} else {
			extras[f.Key] = v.Value
		}
		v.Value = nil
		meta[f.Key] = v
	}
	id = core.ID()
	_, e = tx.Exec(ctx, `INSERT INTO leads(id,organization_id,assistant_id,site_id,name,email,phone,data,provenance,contact_keys,snapshot,privacy_url,source) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'','api')`, id, org, in.AssistantID, site, name, email, phone, raw(extras), raw(meta), raw(keys), raw(conf))
	if e != nil {
		return ExternalLead{}, e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO integration_requests(organization_id,request_key,body_hash,lead_id) VALUES($1,$2,$3,$4)", org, key, hash, id); e != nil {
		return ExternalLead{}, e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO lead_events(id,organization_id,lead_id,kind,status) VALUES($1,$2,$3,'created','new')", core.ID(), org, id); e != nil {
		return ExternalLead{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return ExternalLead{}, e
	}
	return s.Lead(ctx, org, id)
}

const externalSelect = "SELECT id,assistant_id,site_id,conversation_id,name,email,phone,data,status,version,created_at,updated_at FROM leads WHERE organization_id=$1 AND id=$2"

func (s *Store) Lead(ctx context.Context, org, id string) (ExternalLead, error) {
	if !ValidID(id) {
		return ExternalLead{}, core.Fail("validation", 422)
	}
	v := ExternalLead{}
	e := s.DB.QueryRow(ctx, externalSelect, org, id).Scan(&v.ID, &v.AssistantID, &v.SiteID, &v.ConversationID, &v.Name, &v.Email, &v.Phone, &v.Data, &v.Status, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}
func (s *Store) Feed(ctx context.Context, org, cursor string, limit int) (map[string]any, error) {
	after := int64(0)
	var e error
	if cursor != "" {
		after, e = strconv.ParseInt(cursor, 10, 64)
	}
	if e != nil || after < 0 || limit < 1 || limit > 100 {
		return nil, core.Fail("validation", 422)
	}
	rows, e := s.DB.Query(ctx, "SELECT e.seq,e.id,e.lead_id,e.created_at,to_jsonb(l) FROM integration_events e LEFT JOIN leads l ON l.id=e.lead_id AND l.organization_id=e.organization_id WHERE e.organization_id=$1 AND e.seq>$2 AND e.event_type='lead.created' ORDER BY e.seq LIMIT $3", org, after, limit+1)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	items := []map[string]any{}
	next := after
	more := false
	for rows.Next() {
		var seq int64
		var event, id string
		var created time.Time
		var b []byte
		if e = rows.Scan(&seq, &event, &id, &created, &b); e != nil {
			return nil, e
		}
		if len(items) == limit {
			more = true
			break
		}
		var lead *ExternalLead
		if len(b) > 0 {
			lead = &ExternalLead{}
			if e = json.Unmarshal(b, lead); e != nil {
				return nil, e
			}
		}
		items = append(items, map[string]any{"event_id": event, "lead_id": id, "created_at": created, "deleted": lead == nil, "lead": lead})
		next = seq
	}
	return map[string]any{"items": items, "next_cursor": strconv.FormatInt(next, 10), "has_more": more}, rows.Err()
}

func (s *Store) Replay(ctx context.Context, org, key, hash string) (ExternalLead, bool, error) {
	var id, old string
	e := s.DB.QueryRow(ctx, "SELECT lead_id,body_hash FROM integration_requests WHERE organization_id=$1 AND request_key=$2", org, key).Scan(&id, &old)
	if errors.Is(e, pgx.ErrNoRows) {
		return ExternalLead{}, false, nil
	}
	if e != nil {
		return ExternalLead{}, false, e
	}
	if old != hash {
		return ExternalLead{}, true, core.Fail("idempotency_conflict", 409)
	}
	v, e := s.Lead(ctx, org, id)
	if errors.Is(e, pgx.ErrNoRows) {
		e = core.Fail("lead_deleted", 410)
	}
	return v, true, e
}

func (s *Store) endpoint(ctx context.Context, org, id string) (pgx.Tx, string, []byte, bool, error) {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return nil, "", nil, false, e
	}
	var url string
	var b []byte
	var on bool
	e = tx.QueryRow(ctx, "SELECT url,secret_cipher,enabled FROM webhook_endpoints WHERE organization_id=$1 AND id=$2 FOR SHARE", org, id).Scan(&url, &b, &on)
	if e != nil {
		tx.Rollback(ctx)
		return nil, "", nil, false, e
	}
	return tx, url, b, on, nil
}
