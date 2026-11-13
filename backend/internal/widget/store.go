package widget

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"site-assistant/internal/core"
)

type Store struct{ DB *pgxpool.Pool }

func (s *Store) get(ctx context.Context, q string, args ...any) (Widget, error) {
	var w Widget
	var b []byte
	var key string
	e := s.DB.QueryRow(ctx, q, args...).Scan(&b, &key)
	if e == nil {
		e = json.Unmarshal(b, &w)
		w.LogoKey = key
	}
	return w, e
}

const widgetSelect = "SELECT to_jsonb(w)||jsonb_build_object('domain',s.domain),w.logo_key FROM widgets w JOIN sites s ON s.organization_id=w.organization_id AND s.assistant_id=w.assistant_id AND s.id=w.site_id "

func (s *Store) Public(ctx context.Context, id string) (Widget, error) {
	return s.get(ctx, widgetSelect+"WHERE w.id=$1", id)
}
func (s *Store) Admin(ctx context.Context, org, site string) (Widget, error) {
	return s.get(ctx, widgetSelect+"WHERE w.organization_id=$1 AND w.site_id=$2", org, site)
}
func (s *Store) Site(ctx context.Context, org, site string) (string, string, error) {
	var a, domain string
	e := s.DB.QueryRow(ctx, "SELECT assistant_id,domain FROM sites WHERE organization_id=$1 AND id=$2", org, site).Scan(&a, &domain)
	return a, domain, e
}
func (s *Store) Create(ctx context.Context, w Widget) (Widget, error) {
	b, _ := json.Marshal(w.Settings)
	_, e := s.DB.Exec(ctx, "INSERT INTO widgets(id,organization_id,assistant_id,site_id,origins,settings,verification_token) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(site_id) DO NOTHING", w.ID, w.OrganizationID, w.AssistantID, w.SiteID, w.Origins, b, w.VerificationToken)
	if e != nil {
		return w, e
	}
	return s.Admin(ctx, w.OrganizationID, w.SiteID)
}
func (s *Store) Save(ctx context.Context, w Widget) (Widget, error) {
	b, _ := json.Marshal(w.Settings)
	_, e := s.DB.Exec(ctx, "UPDATE widgets SET enabled=$3,origins=$4,settings=$5,updated_at=now() WHERE organization_id=$1 AND site_id=$2", w.OrganizationID, w.SiteID, w.Enabled, w.Origins, b)
	if e != nil {
		return w, e
	}
	return s.Admin(ctx, w.OrganizationID, w.SiteID)
}
func (s *Store) Logo(ctx context.Context, org, site, key string) error {
	_, e := s.DB.Exec(ctx, "UPDATE widgets SET logo_key=$3,updated_at=now() WHERE organization_id=$1 AND site_id=$2", org, site, key)
	return e
}
func (s *Store) Verified(ctx context.Context, org, site string) error {
	_, e := s.DB.Exec(ctx, "UPDATE widgets SET verified_at=now(),updated_at=now() WHERE organization_id=$1 AND site_id=$2", org, site)
	return e
}
func (s *Store) Session(ctx context.Context, w Widget, origin, hash string, c Config) (Session, error) {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return Session{}, e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,424))", w.OrganizationID); e != nil {
		return Session{}, e
	}
	var count, siteCount int
	e = tx.QueryRow(ctx, "SELECT count(*),count(*) FILTER(WHERE site_id=$2) FROM widget_sessions WHERE organization_id=$1 AND created_at>now()-interval '24 hours'", w.OrganizationID, w.SiteID).Scan(&count, &siteCount)
	if e != nil {
		return Session{}, e
	}
	if count >= 2000 || siteCount >= 500 {
		return Session{}, core.Fail("widget_capacity", 429)
	}
	v := Session{ID: core.ID()}
	e = tx.QueryRow(ctx, "INSERT INTO widget_sessions(id,widget_id,organization_id,assistant_id,site_id,origin,token_hash,expires_at) SELECT $1,id,organization_id,assistant_id,site_id,$3,$4,now()+$5*interval '1 second' FROM widgets WHERE id=$2 AND enabled AND $3=ANY(origins) RETURNING expires_at", v.ID, w.ID, origin, hash, int(c.SessionTTL.Seconds())).Scan(&v.ExpiresAt)
	if e != nil {
		return v, e
	}
	_, e = tx.Exec(ctx, "INSERT INTO conversations(id,organization_id,assistant_id,kind,site_id,anonymous_session_id) VALUES($1,$2,$3,'website',$4,$5)", core.ID(), w.OrganizationID, w.AssistantID, w.SiteID, v.ID)
	if e != nil {
		return v, e
	}
	return v, tx.Commit(ctx)
}
