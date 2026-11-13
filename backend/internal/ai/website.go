package ai

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"site-assistant/internal/core"
)

// Website authorization lives at the engine boundary too: an unchecked tenant ID is never a grant.
func (s *Store) WebsiteScope(ctx context.Context, widget, session, token, origin string) (Scope, error) {
	var v Scope
	e := s.DB.QueryRow(ctx, `SELECT c.organization_id,c.assistant_id,c.id FROM widget_sessions s JOIN widgets w ON w.id=s.widget_id AND w.organization_id=s.organization_id AND w.assistant_id=s.assistant_id AND w.site_id=s.site_id JOIN conversations c ON c.anonymous_session_id=s.id AND c.organization_id=s.organization_id AND c.assistant_id=s.assistant_id AND c.site_id=s.site_id AND c.kind='website' WHERE w.id=$1 AND s.id=$2 AND s.token_hash=$3 AND s.origin=$4 AND s.expires_at>now() AND w.enabled AND $4=ANY(w.origins)`, widget, session, core.TokenHash(token), origin).Scan(&v.OrganizationID, &v.AssistantID, &v.ConversationID)
	if e != nil {
		return v, core.Fail("visitor_unauthorized", 401)
	}
	return v, nil
}
func (s *Service) WebsiteMessage(ctx context.Context, widget, session, token, origin string, in MessageInput, stream bool, emit func(Event) error) (Result, error) {
	v, e := s.Store.WebsiteScope(ctx, widget, session, token, origin)
	if e != nil {
		return Result{}, e
	}
	return s.generate(ctx, "", v, in, stream, emit, true)
}
func (s *Service) WebsiteHistory(ctx context.Context, widget, session, token, origin string) (Detail, error) {
	v, e := s.Store.WebsiteScope(ctx, widget, session, token, origin)
	if e != nil {
		return Detail{}, e
	}
	return s.Store.detail(ctx, v, 0, false)
}
func (s *Service) WebsiteCancel(ctx context.Context, widget, session, token, origin string) error {
	v, e := s.Store.WebsiteScope(ctx, widget, session, token, origin)
	if e != nil {
		return e
	}
	return s.cancel(ctx, v)
}
func (s *Service) cancel(ctx context.Context, v Scope) error {
	id, e := s.Store.Active(ctx, v)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	s.mu.Lock()
	cancel := s.active[id]
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	e = s.Store.Finish(ctx, v, id, "canceled", "", "", "ai_canceled", 0, Usage{})
	var f *core.Fault
	if errors.As(e, &f) && f.Code == "ai_canceled" {
		return nil
	}
	return e
}

type WebsiteLimits struct{ SessionMessages, SiteHourly, OrgHourly, SiteConcurrent, OrgConcurrent int }

func DefaultWebsiteLimits() WebsiteLimits { return WebsiteLimits{50, 300, 1000, 2, 4} }
func (s *Store) websiteBudget(ctx context.Context, tx pgx.Tx, v Scope, limit WebsiteLimits, generation bool) error {
	if _, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,424))", v.OrganizationID); e != nil {
		return e
	}
	var site, session string
	var count int
	e := tx.QueryRow(ctx, `SELECT c.site_id,c.anonymous_session_id,s.message_count FROM conversations c JOIN widget_sessions s ON s.id=c.anonymous_session_id JOIN widgets w ON w.id=s.widget_id WHERE c.organization_id=$1 AND c.assistant_id=$2 AND c.id=$3 AND c.kind='website' AND w.enabled AND s.expires_at>now() AND s.origin=ANY(w.origins) FOR UPDATE OF s`, v.OrganizationID, v.AssistantID, v.ConversationID).Scan(&site, &session, &count)
	if e != nil {
		return core.Fail("visitor_unauthorized", 401)
	}
	if count >= limit.SessionMessages {
		return core.Fail("widget_session_limit", 429)
	}
	var siteHour, orgHour, siteActive, orgActive int
	e = tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE c.site_id=$2),count(*) FROM conversation_messages m JOIN conversations c ON c.id=m.conversation_id AND c.organization_id=m.organization_id WHERE m.organization_id=$1 AND c.kind='website' AND m.role='user' AND m.created_at>now()-interval '1 hour'`, v.OrganizationID, site).Scan(&siteHour, &orgHour)
	if e != nil {
		return e
	}
	if generation {
		e = tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE c.site_id=$2),count(*) FROM ai_executions e JOIN conversations c ON c.id=e.conversation_id AND c.organization_id=e.organization_id WHERE e.organization_id=$1 AND c.kind='website' AND e.status='processing'`, v.OrganizationID, site).Scan(&siteActive, &orgActive)
		if e != nil {
			return e
		}
	}
	if siteHour >= limit.SiteHourly || orgHour >= limit.OrgHourly || generation && (siteActive >= limit.SiteConcurrent || orgActive >= limit.OrgConcurrent) {
		return core.Fail("widget_capacity", 429)
	}
	_, e = tx.Exec(ctx, "UPDATE widget_sessions SET message_count=message_count+1 WHERE id=$1 AND organization_id=$2 AND assistant_id=$3", session, v.OrganizationID, v.AssistantID)
	return e
}
