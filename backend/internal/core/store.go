package core

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ DB *pgxpool.Pool }

func one[T any](ctx context.Context, s *Store, q string, args ...any) (T, error) {
	var v T
	var b []byte
	e := s.DB.QueryRow(ctx, q, args...).Scan(&b)
	if e == nil {
		e = json.Unmarshal(b, &v)
	}
	return v, e
}
func many[T any](ctx context.Context, s *Store, q string, args ...any) ([]T, error) {
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

const userFields = "id,email,name,preferred_locale,created_at"

func (s *Store) User(ctx context.Context, id string) (User, error) {
	return one[User](ctx, s, "SELECT row_to_json(x) FROM (SELECT "+userFields+" FROM users WHERE id=$1) x", id)
}
func (s *Store) Credentials(ctx context.Context, email string) (string, string, error) {
	var id, h string
	e := s.DB.QueryRow(ctx, "SELECT id,password_hash FROM users WHERE email=$1", email).Scan(&id, &h)
	return id, h, e
}
func (s *Store) AddUser(ctx context.Context, id, email, hash, locale string) (User, error) {
	_, e := s.DB.Exec(ctx, "INSERT INTO users(id,email,password_hash,preferred_locale) VALUES($1,$2,$3,$4)", id, email, hash, locale)
	if e != nil {
		return User{}, e
	}
	return s.User(ctx, id)
}
func (s *Store) Profile(ctx context.Context, id, name, locale string) (User, error) {
	_, e := s.DB.Exec(ctx, "UPDATE users SET name=$2,preferred_locale=$3 WHERE id=$1", id, name, locale)
	if e != nil {
		return User{}, e
	}
	return s.User(ctx, id)
}
func (s *Store) Session(ctx context.Context, hash string) (User, error) {
	return one[User](ctx, s, "SELECT row_to_json(x) FROM (SELECT u.id,u.email,u.name,u.preferred_locale,u.created_at FROM users u JOIN sessions s ON s.user_id=u.id WHERE s.token_hash=$1 AND s.expires_at>now()) x", hash)
}
func (s *Store) AddSession(ctx context.Context, hash, id string) error {
	_, e := s.DB.Exec(ctx, "INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,$2,now()+interval '7 days')", hash, id)
	return e
}
func (s *Store) DeleteSession(ctx context.Context, hash string) error {
	_, e := s.DB.Exec(ctx, "DELETE FROM sessions WHERE token_hash=$1", hash)
	return e
}
func (s *Store) Cleanup(ctx context.Context) error {
	_, e := s.DB.Exec(ctx, "DELETE FROM sessions WHERE expires_at<=now()")
	return e
}
func (s *Store) Role(ctx context.Context, org, user string) (string, error) {
	var r string
	e := s.DB.QueryRow(ctx, "SELECT role FROM organization_members WHERE organization_id=$1 AND user_id=$2", org, user).Scan(&r)
	return r, e
}
func (s *Store) Organizations(ctx context.Context, user string) ([]Organization, error) {
	return many[Organization](ctx, s, "SELECT row_to_json(x) FROM (SELECT o.*,m.role FROM organizations o JOIN organization_members m ON m.organization_id=o.id WHERE m.user_id=$1 ORDER BY o.created_at,o.id) x", user)
}
func (s *Store) Organization(ctx context.Context, id string) (Organization, error) {
	return one[Organization](ctx, s, "SELECT row_to_json(o) FROM organizations o WHERE id=$1", id)
}
func (s *Store) AddOrganization(ctx context.Context, user string, o Organization) (Organization, error) {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return o, e
	}
	defer tx.Rollback(ctx)
	_, e = tx.Exec(ctx, "INSERT INTO organizations(id,name,country,timezone,default_locale) VALUES($1,$2,$3,$4,$5)", o.ID, o.Name, o.Country, o.Timezone, o.Locale)
	if e != nil {
		return o, e
	}
	_, e = tx.Exec(ctx, "INSERT INTO organization_members(organization_id,user_id,role) VALUES($1,$2,'owner')", o.ID, user)
	if e != nil {
		return o, e
	}
	if e = tx.Commit(ctx); e != nil {
		return o, e
	}
	return s.Organization(ctx, o.ID)
}
func (s *Store) UpdateOrganization(ctx context.Context, o Organization) (Organization, error) {
	_, e := s.DB.Exec(ctx, "UPDATE organizations SET name=$2,country=$3,timezone=$4,default_locale=$5 WHERE id=$1", o.ID, o.Name, o.Country, o.Timezone, o.Locale)
	if e != nil {
		return o, e
	}
	return s.Organization(ctx, o.ID)
}
func (s *Store) Assistants(ctx context.Context, org string) ([]Assistant, error) {
	return many[Assistant](ctx, s, "SELECT row_to_json(a) FROM assistants a WHERE organization_id=$1 ORDER BY created_at,id", org)
}
func (s *Store) Assistant(ctx context.Context, org, id string) (Assistant, error) {
	return one[Assistant](ctx, s, "SELECT row_to_json(a) FROM assistants a WHERE organization_id=$1 AND id=$2", org, id)
}
func (s *Store) SaveAssistant(ctx context.Context, a Assistant, create bool) (Assistant, error) {
	var e error
	if create {
		_, e = s.DB.Exec(ctx, "INSERT INTO assistants(id,organization_id,name,description,default_locale,language_policy,status) VALUES($1,$2,$3,$4,$5,$6,$7)", a.ID, a.OrganizationID, a.Name, a.Description, a.Locale, a.LanguagePolicy, a.Status)
	} else {
		_, e = s.DB.Exec(ctx, "UPDATE assistants SET name=$3,description=$4,default_locale=$5,language_policy=$6,status=$7,updated_at=now() WHERE organization_id=$1 AND id=$2", a.OrganizationID, a.ID, a.Name, a.Description, a.Locale, a.LanguagePolicy, a.Status)
	}
	if e != nil {
		return a, e
	}
	return s.Assistant(ctx, a.OrganizationID, a.ID)
}
func (s *Store) Sites(ctx context.Context, org string) ([]Site, error) {
	return many[Site](ctx, s, "SELECT row_to_json(s) FROM sites s WHERE organization_id=$1 ORDER BY created_at,id", org)
}
func (s *Store) AddSite(ctx context.Context, v Site) (Site, error) {
	_, e := s.DB.Exec(ctx, "INSERT INTO sites(id,organization_id,assistant_id,name,domain) VALUES($1,$2,$3,$4,$5)", v.ID, v.OrganizationID, v.AssistantID, v.Name, v.Domain)
	if e != nil {
		return v, e
	}
	return one[Site](ctx, s, "SELECT row_to_json(s) FROM sites s WHERE organization_id=$1 AND id=$2", v.OrganizationID, v.ID)
}
func (s *Store) DeleteSite(ctx context.Context, org, id string) error {
	tag, e := s.DB.Exec(ctx, "DELETE FROM sites WHERE organization_id=$1 AND id=$2", org, id)
	if e == nil && tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return e
}
