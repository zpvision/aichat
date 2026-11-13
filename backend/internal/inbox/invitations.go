package inbox

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"site-assistant/internal/core"
	"time"
)

func (s *Service) CreateInvitation(ctx context.Context, user, org, role string) (Invitation, error) {
	if _, e := s.Auth.Access(ctx, user, org, true, false); e != nil {
		return Invitation{}, e
	}
	if role != "admin" && role != "operator" {
		return Invitation{}, core.Fail("validation", 422)
	}
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return Invitation{}, e
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	v, e := s.Store.CreateInvitation(ctx, org, user, role, core.TokenHash(token))
	if e == nil {
		v.URL = s.AppURL + "/#invite=" + token
	}
	return v, e
}
func (s *Service) Invitations(ctx context.Context, user, org string) ([]Invitation, error) {
	if _, e := s.Auth.Access(ctx, user, org, true, false); e != nil {
		return nil, e
	}
	return s.Store.Invitations(ctx, org)
}
func (s *Service) Revoke(ctx context.Context, user, org, id string) error {
	if _, e := s.Auth.Access(ctx, user, org, true, false); e != nil {
		return e
	}
	if !uuid.MatchString(id) {
		return core.Fail("not_found", 404)
	}
	return s.Store.Revoke(ctx, org, id)
}
func (s *Service) Accept(ctx context.Context, user, token string) (string, error) {
	if len(token) != 43 {
		return "", core.Fail("invitation_invalid", 422)
	}
	return s.Store.Accept(ctx, user, core.TokenHash(token))
}
func (s *Store) CreateInvitation(ctx context.Context, org, user, role, hash string) (Invitation, error) {
	v := Invitation{}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return v, e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,555))", org); e != nil {
		return v, e
	}
	var count int
	if e = tx.QueryRow(ctx, "SELECT count(*) FROM organization_invitations WHERE organization_id=$1 AND revoked_at IS NULL AND accepted_at IS NULL AND expires_at>now()", org).Scan(&count); e != nil {
		return v, e
	}
	if count >= 50 {
		return v, core.Fail("invitation_limit", 429)
	}
	var b []byte
	e = tx.QueryRow(ctx, "INSERT INTO organization_invitations(id,organization_id,created_by,role,token_hash,expires_at) VALUES($1,$2,$3,$4,$5,now()+interval '7 days') RETURNING row_to_json(organization_invitations)", core.ID(), org, user, role, hash).Scan(&b)
	if e != nil {
		return v, e
	}
	json.Unmarshal(b, &v)
	return v, tx.Commit(ctx)
}
func (s *Store) Invitations(ctx context.Context, org string) ([]Invitation, error) {
	out := []Invitation{}
	rows, e := s.DB.Query(ctx, "SELECT row_to_json(i) FROM organization_invitations i WHERE organization_id=$1 ORDER BY created_at DESC LIMIT 100", org)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var b []byte
		var v Invitation
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
func (s *Store) Revoke(ctx context.Context, org, id string) error {
	tag, e := s.DB.Exec(ctx, "UPDATE organization_invitations SET revoked_at=COALESCE(revoked_at,now()) WHERE organization_id=$1 AND id=$2 AND accepted_at IS NULL", org, id)
	if e == nil && tag.RowsAffected() == 0 {
		return core.Fail("not_found", 404)
	}
	return e
}
func (s *Store) Accept(ctx context.Context, user, hash string) (string, error) {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return "", e
	}
	defer tx.Rollback(ctx)
	var id, org, role string
	var revoked, accepted *time.Time
	var by *string
	var valid bool
	e = tx.QueryRow(ctx, "SELECT id,organization_id,role,revoked_at,accepted_at,accepted_by,expires_at>now() FROM organization_invitations WHERE token_hash=$1 FOR UPDATE", hash).Scan(&id, &org, &role, &revoked, &accepted, &by, &valid)
	if e == pgx.ErrNoRows {
		return "", core.Fail("invitation_invalid", 422)
	}
	if e != nil {
		return "", e
	}
	if accepted != nil {
		if by != nil && *by == user {
			return org, nil
		}
		return "", core.Fail("invitation_invalid", 422)
	}
	if revoked != nil || !valid {
		return "", core.Fail("invitation_invalid", 422)
	}
	// Joining never upgrades or downgrades an existing membership, especially owner.
	if _, e = tx.Exec(ctx, "INSERT INTO organization_members(organization_id,user_id,role) VALUES($1,$2,$3) ON CONFLICT(organization_id,user_id) DO NOTHING", org, user, role); e != nil {
		return "", e
	}
	if _, e = tx.Exec(ctx, "UPDATE organization_invitations SET accepted_at=now(),accepted_by=$2 WHERE id=$1", id, user); e != nil {
		return "", e
	}
	return org, tx.Commit(ctx)
}
