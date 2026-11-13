package leads

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"site-assistant/internal/core"
	"strings"
	"time"
)

const leadSelect = `SELECT to_jsonb(l)||jsonb_build_object('site',s.name,'can_open_conversation',($3<>'operator' OR c.assigned_user_id=NULLIF($2,'')::uuid OR (c.assigned_user_id IS NULL AND c.control_mode<>'closed'))) FROM leads l JOIN sites s ON s.id=l.site_id AND s.organization_id=l.organization_id JOIN conversations c ON c.id=l.conversation_id AND c.organization_id=l.organization_id `

func visible(l Lead, user, role string) bool {
	return role != "operator" || l.Assigned != nil && *l.Assigned == user || l.Assigned == nil && (l.Status == "new" || l.Status == "in_progress")
}
func restore(l *Lead) {
	l.Values = map[string]Value{}
	for k, m := range l.Provenance {
		if v, ok := l.Data[k]; ok {
			m.Value = v
		} else {
			for kind, key := range l.ContactKeys {
				if key == k {
					value := ""
					switch kind {
					case "name":
						value = l.Name
					case "email":
						value = l.Email
					case "phone":
						value = l.Phone
					}
					m.Value = encode(value)
				}
			}
		}
		l.Values[k] = m
	}
}
func (s *Service) List(ctx context.Context, user, org string, f Filter) (map[string]any, error) {
	role, e := s.Auth.Access(ctx, user, org, false, false)
	if e != nil {
		return nil, e
	}
	if f.Limit == 0 {
		f.Limit = 30
	}
	if f.Limit < 1 || f.Limit > 50 || !status(f.Status, true) {
		return nil, core.Fail("validation", 422)
	}
	for _, id := range []string{f.Site, f.Assistant, f.Assigned} {
		if id != "" && !keyUUID(id) {
			return nil, core.Fail("validation", 422)
		}
	}
	return s.Store.List(ctx, user, role, org, f)
}
func keyUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, r := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if r != '-' {
				return false
			}
		} else if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}
func status(v string, empty bool) bool {
	return empty && v == "" || v == "new" || v == "in_progress" || v == "won" || v == "lost"
}
func (s *Service) Detail(ctx context.Context, user, org, id string) (Detail, error) {
	role, e := s.Auth.Access(ctx, user, org, false, false)
	if e != nil {
		return Detail{}, e
	}
	return s.Store.Detail(ctx, user, role, org, id)
}
func (s *Service) Patch(ctx context.Context, user, org, id string, in LeadPatch) (Detail, error) {
	role, e := s.Auth.Access(ctx, user, org, false, false)
	if e != nil {
		return Detail{}, e
	}
	if in.Version < 1 || in.Status != nil && !status(*in.Status, false) || in.Assigned != nil && *in.Assigned != "" && !keyUUID(*in.Assigned) || in.Note != nil && (!validText(*in.Note, 4000) || strings.TrimSpace(*in.Note) == "") {
		return Detail{}, core.Fail("validation", 422)
	}
	if e = s.Store.Patch(ctx, user, role, org, id, in); e != nil {
		return Detail{}, e
	}
	return s.Store.Detail(ctx, user, role, org, id)
}
func (s *Service) Delete(ctx context.Context, user, org, id string) error {
	if _, e := s.Auth.Access(ctx, user, org, true, false); e != nil {
		return e
	}
	return s.Store.Delete(ctx, org, id)
}
func (s *Store) List(ctx context.Context, user, role, org string, f Filter) (map[string]any, error) {
	items := []Lead{}
	var date, from, to any
	var id any
	if f.Cursor != "" {
		b, e := base64.RawURLEncoding.DecodeString(f.Cursor)
		parts := strings.Split(string(b), "|")
		if e != nil || len(parts) != 2 || !keyUUID(parts[1]) {
			return nil, core.Fail("validation", 422)
		}
		t, e := time.Parse(time.RFC3339Nano, parts[0])
		if e != nil {
			return nil, core.Fail("validation", 422)
		}
		date = t
		id = parts[1]
	}
	for _, p := range []struct {
		raw string
		v   *any
		end bool
	}{{f.From, &from, false}, {f.To, &to, true}} {
		if p.raw != "" {
			v, e := time.Parse("2006-01-02", p.raw)
			if e != nil {
				return nil, core.Fail("validation", 422)
			}
			if p.end {
				v = v.AddDate(0, 0, 1)
			}
			*p.v = v
		}
	}
	rows, e := s.DB.Query(ctx, leadSelect+`WHERE l.organization_id=$1 AND ($3<>'operator' OR l.assigned_user_id=NULLIF($2,'')::uuid OR (l.assigned_user_id IS NULL AND l.status IN ('new','in_progress'))) AND ($4='' OR l.status=$4) AND ($5='' OR l.site_id=NULLIF($5,'')::uuid) AND ($6='' OR l.assistant_id=NULLIF($6,'')::uuid) AND ($7='' OR l.assigned_user_id=NULLIF($7,'')::uuid) AND ($8::timestamptz IS NULL OR l.created_at>=$8) AND ($9::timestamptz IS NULL OR l.created_at<$9) AND ($10::timestamptz IS NULL OR(l.created_at,l.id)<($10,$11::uuid)) ORDER BY l.created_at DESC,l.id DESC LIMIT $12`, org, user, role, f.Status, f.Site, f.Assistant, f.Assigned, from, to, date, id, f.Limit+1)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		l, e := decode[Lead](rows)
		if e != nil {
			return nil, e
		}
		restore(&l)
		items = append(items, l)
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	next := ""
	if len(items) > f.Limit {
		items = items[:f.Limit]
		l := items[len(items)-1]
		next = base64.RawURLEncoding.EncodeToString([]byte(l.CreatedAt.Format(time.RFC3339Nano) + "|" + l.ID))
	}
	return map[string]any{"items": items, "next_cursor": next}, nil
}
func (s *Store) Detail(ctx context.Context, user, role, org, id string) (Detail, error) {
	d := Detail{Events: []Event{}}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return d, e
	}
	defer tx.Rollback(ctx)
	l, e := decode[Lead](tx.QueryRow(ctx, leadSelect+"WHERE l.organization_id=$1 AND l.id=$4 FOR SHARE OF l", org, user, role, id))
	if e != nil {
		return d, e
	}
	if !visible(l, user, role) {
		return d, core.Fail("forbidden", 403)
	}
	restore(&l)
	d.Lead = l
	rows, e := tx.Query(ctx, "SELECT to_jsonb(e) FROM lead_events e WHERE organization_id=$1 AND lead_id=$2 ORDER BY created_at DESC,id DESC LIMIT 100", org, id)
	if e != nil {
		return d, e
	}
	defer rows.Close()
	for rows.Next() {
		v, e := decode[Event](rows)
		if e != nil {
			return d, e
		}
		d.Events = append(d.Events, v)
	}
	if e = rows.Err(); e != nil {
		return d, e
	}
	rows.Close()
	return d, tx.Commit(ctx)
}
func (s *Store) Patch(ctx context.Context, user, role, org, id string, in LeadPatch) error {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	l, e := decode[Lead](tx.QueryRow(ctx, "SELECT to_jsonb(l) FROM leads l WHERE organization_id=$1 AND id=$2 FOR UPDATE", org, id))
	if e != nil {
		return e
	}
	if !visible(l, user, role) {
		return core.Fail("forbidden", 403)
	}
	if l.Version != in.Version {
		return core.Fail("state_conflict", 409)
	}
	if role == "operator" {
		if in.Values != nil {
			return core.Fail("forbidden", 403)
		}
		if l.Assigned == nil {
			if in.Assigned == nil || *in.Assigned != user {
				return core.Fail("forbidden", 403)
			}
		} else if *l.Assigned != user {
			return core.Fail("forbidden", 403)
		}
		if in.Assigned != nil && *in.Assigned != user {
			return core.Fail("forbidden", 403)
		}
	}
	eventStatus := ""
	if in.Status != nil {
		l.Status = *in.Status
		eventStatus = l.Status
	}
	if in.Assigned != nil {
		if *in.Assigned == "" {
			l.Assigned = nil
		} else {
			var member bool
			if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM organization_members WHERE organization_id=$1 AND user_id=$2)", org, *in.Assigned).Scan(&member); e != nil {
				return e
			}
			if !member {
				return core.Fail("validation", 422)
			}
			l.Assigned = in.Assigned
		}
	}
	if in.Values != nil {
		restore(&l)
		fields := map[string]Field{}
		for _, f := range l.Snapshot.Fields {
			if f.Active {
				fields[f.Key] = f
			}
		}
		for key, raw := range in.Values {
			f, ok := fields[key]
			if !ok {
				return core.Fail("lead_unknown_field", 422)
			}
			if string(raw) == "null" {
				delete(l.Values, key)
				continue
			}
			v, code := ValidateValue(f, raw)
			if code != "" {
				return core.Fail(code, 422)
			}
			if old,ok:=l.Values[key];ok&&bytes.Equal(old.Value,v)&&old.Valid{continue}
			l.Values[key] = Value{Value: v, Valid: true, Source: "staff", UpdatedAt: time.Now().UTC()}
		}
		if len(Missing(l.Snapshot, l.Values)) > 0 {
			return core.Fail("lead_incomplete", 422)
		}
		l.Name = ""
		l.Email = ""
		l.Phone = ""
		l.Data = map[string]json.RawMessage{}
		l.Provenance = map[string]Value{}
		l.ContactKeys = map[string]string{}
		for _, f := range l.Snapshot.Fields {
			v, ok := l.Values[f.Key]
			if !ok {
				continue
			}
			contact := ""
			if f.Type == "email" || f.Type == "phone" {
				contact = f.Type
			}
			if f.Key == "full_name" || f.Key == "name" {
				contact = "name"
			}
			if contact != "" && l.ContactKeys[contact] == "" {
				var text string
				json.Unmarshal(v.Value, &text)
				switch contact {
				case "name":
					l.Name = text
				case "email":
					l.Email = text
				case "phone":
					l.Phone = text
				}
				l.ContactKeys[contact] = f.Key
			} else {
				l.Data[f.Key] = v.Value
			}
			v.Value = nil
			l.Provenance[f.Key] = v
		}
	}
	note := ""
	if in.Note != nil {
		note = *in.Note
	}
	_, e = tx.Exec(ctx, `UPDATE leads SET status=$3,assigned_user_id=$4,name=$5,email=$6,phone=$7,data=$8,provenance=$9,contact_keys=$10,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2`, org, id, l.Status, l.Assigned, l.Name, l.Email, l.Phone, encode(l.Data), encode(l.Provenance), encode(l.ContactKeys))
	if e != nil {
		return e
	}
	kinds := []string{}
	if in.Status != nil {
		kinds = append(kinds, "status")
	}
	if in.Assigned != nil {
		kinds = append(kinds, "assigned")
	}
	if in.Values != nil {
		kinds = append(kinds, "contacts_updated")
	}
	if in.Note != nil {
		kinds = append(kinds, "note")
	}
	if len(kinds) == 0 {
		return core.Fail("validation", 422)
	}
	for _, k := range kinds {
		n := ""
		if k == "note" {
			n = note
		}
		_, e = tx.Exec(ctx, "INSERT INTO lead_events(id,organization_id,lead_id,actor_id,kind,status,target_id,note) VALUES($1,$2,$3,$4,$5,$6,$7,$8)", core.ID(), org, id, user, k, eventStatus, l.Assigned, n)
		if e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
func (s *Store) Delete(ctx context.Context, org, id string) error {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var conversation string
	e = tx.QueryRow(ctx, "SELECT conversation_id FROM leads WHERE organization_id=$1 AND id=$2", org, id).Scan(&conversation)
	if e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "SELECT id FROM conversations WHERE organization_id=$1 AND id=$2 FOR UPDATE", org, conversation); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "SELECT id FROM lead_collections WHERE organization_id=$1 AND conversation_id=$2 FOR UPDATE", org, conversation); e != nil {
		return e
	}
	tag, e := tx.Exec(ctx, "DELETE FROM leads WHERE organization_id=$1 AND id=$2", org, id)
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		return core.Fail("not_found", 404)
	}
	return tx.Commit(ctx)
}
