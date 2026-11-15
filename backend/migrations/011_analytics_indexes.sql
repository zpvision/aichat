-- Read-only analytics indexes; no new event stream or customer-data copies.
CREATE INDEX analytics_conversations_created ON conversations(organization_id,created_at,site_id,assistant_id) WHERE kind='website';
CREATE INDEX analytics_execution_period ON ai_executions(organization_id,created_at,conversation_id);
CREATE INDEX analytics_event_period ON conversation_events(organization_id,created_at,conversation_id) WHERE kind<>'note';
CREATE INDEX analytics_operator_messages ON conversation_messages(organization_id,created_at,author_user_id,conversation_id) WHERE role='operator' AND status='completed';
CREATE INDEX analytics_first_visitor ON conversation_messages(organization_id,conversation_id,created_at) WHERE role='user';
CREATE INDEX analytics_first_operator ON conversation_messages(organization_id,conversation_id,created_at) WHERE role='operator' AND status='completed';
CREATE INDEX analytics_first_takeover ON conversation_events(organization_id,conversation_id,created_at) WHERE kind='human_active';
