package ai

import (
	"context"
	"site-assistant/internal/core"
)

// Deliver holds a shared conversation row lock THROUGH the bounded network flush.
// Handoff/finish hold the exclusive lock: after their commit no stale delta can be sent.
// This is deliberately DB based so it also works with multiple Go processes.
func (s *Store) Deliver(ctx context.Context, v Scope, id string, complete bool, emit func() error) error {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var mode, kind, status string
	var version int64
	e = tx.QueryRow(ctx, `SELECT control_mode,kind,status,state_version FROM conversations WHERE organization_id=$1 AND assistant_id=$2 AND id=$3 FOR SHARE`, v.OrganizationID, v.AssistantID, v.ConversationID).Scan(&mode, &kind, &status, &version)
	if e != nil {
		return e
	}
	// Separate statement AFTER acquiring the lock: READ COMMITTED must see a cancellation
	// that committed while the row lock was being awaited, not an earlier join snapshot.
	var executionVersion int64
	var executionStatus string
	e = tx.QueryRow(ctx, `SELECT state_version,status FROM ai_executions WHERE organization_id=$1 AND assistant_id=$2 AND conversation_id=$3 AND id=$4`, v.OrganizationID, v.AssistantID, v.ConversationID, id).Scan(&executionVersion, &executionStatus)
	if e != nil {
		return e
	}
	if kind != "preview" && (mode != "ai_active" || status != "open" || version != executionVersion || (executionStatus != "processing" && !(complete && executionStatus == "completed"))) {
		return core.Fail("ai_canceled", 409)
	}
	if e = emit(); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (s *Service) SignalID(id string) {
	s.mu.Lock()
	cancel := s.active[id]
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}
