CREATE SEQUENCE conversation_change_seq;
ALTER TABLE conversations ADD COLUMN control_mode text NOT NULL DEFAULT 'ai_active' CHECK(control_mode IN ('ai_active','human_requested','human_active','closed'));
ALTER TABLE conversations ADD COLUMN state_version bigint NOT NULL DEFAULT 1;
ALTER TABLE conversations ADD COLUMN assigned_user_id uuid;
ALTER TABLE conversations ADD COLUMN last_message_at timestamptz;
ALTER TABLE conversations ADD COLUMN last_visitor_at timestamptz;
ALTER TABLE conversations ADD COLUMN requested_at timestamptz;
ALTER TABLE conversations ADD COLUMN closed_at timestamptz;
ALTER TABLE conversations ADD CONSTRAINT conversation_assignee FOREIGN KEY(organization_id,assigned_user_id) REFERENCES organization_members(organization_id,user_id);
ALTER TABLE conversations ADD CONSTRAINT conversations_inbox_scope UNIQUE(organization_id,id);
UPDATE conversations SET control_mode='closed',closed_at=updated_at WHERE status='closed';
ALTER TABLE ai_executions ADD COLUMN state_version bigint NOT NULL DEFAULT 1;
ALTER TABLE conversation_messages ALTER COLUMN execution_id DROP NOT NULL;
ALTER TABLE conversation_messages DROP CONSTRAINT conversation_messages_role_check;
ALTER TABLE conversation_messages ADD CONSTRAINT conversation_messages_role_check CHECK(role IN ('user','assistant','operator'));
ALTER TABLE conversation_messages ADD COLUMN author_user_id uuid;
ALTER TABLE conversation_messages ADD COLUMN request_id uuid;
ALTER TABLE conversation_messages ADD COLUMN change_seq bigint NOT NULL DEFAULT nextval('conversation_change_seq');
ALTER TABLE conversation_messages ADD CONSTRAINT messages_conversation_scope FOREIGN KEY(organization_id,assistant_id,conversation_id) REFERENCES conversations(organization_id,assistant_id,id) ON DELETE CASCADE;
ALTER TABLE conversation_messages ADD CONSTRAINT messages_author_scope FOREIGN KEY(organization_id,author_user_id) REFERENCES organization_members(organization_id,user_id);
ALTER TABLE conversation_messages ADD CONSTRAINT messages_authorship CHECK((role='operator' AND author_user_id IS NOT NULL AND execution_id IS NULL) OR (role IN ('user','assistant') AND author_user_id IS NULL));
UPDATE conversation_messages m SET request_id=e.request_id FROM ai_executions e WHERE e.id=m.execution_id AND m.role='user';
CREATE UNIQUE INDEX messages_request_id ON conversation_messages(conversation_id,request_id) WHERE request_id IS NOT NULL;
CREATE INDEX message_changes ON conversation_messages(conversation_id,change_seq);
CREATE OR REPLACE FUNCTION touch_conversation_message() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='UPDATE' THEN NEW.change_seq=nextval('conversation_change_seq'); END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER message_changed BEFORE UPDATE ON conversation_messages FOR EACH ROW EXECUTE FUNCTION touch_conversation_message();
CREATE TABLE conversation_events(
 id uuid PRIMARY KEY,organization_id uuid NOT NULL,conversation_id uuid NOT NULL,
 sequence bigint NOT NULL DEFAULT nextval('conversation_change_seq'),kind text NOT NULL CHECK(kind IN ('human_requested','human_active','ai_active','closed','assigned','note')),
 actor_id uuid,target_id uuid,content text NOT NULL DEFAULT '',request_id uuid NOT NULL,request_hash text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),UNIQUE(conversation_id,request_id),
 FOREIGN KEY(organization_id,conversation_id) REFERENCES conversations(organization_id,id) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,actor_id) REFERENCES organization_members(organization_id,user_id),
 FOREIGN KEY(organization_id,target_id) REFERENCES organization_members(organization_id,user_id)
);
CREATE INDEX conversation_event_history ON conversation_events(conversation_id,sequence);
CREATE TABLE conversation_reads(
 organization_id uuid NOT NULL,conversation_id uuid NOT NULL,user_id uuid NOT NULL,last_sequence bigint NOT NULL DEFAULT 0,
 PRIMARY KEY(conversation_id,user_id),FOREIGN KEY(organization_id,conversation_id) REFERENCES conversations(organization_id,id) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,user_id) REFERENCES organization_members(organization_id,user_id)
);
CREATE INDEX inbox_activity ON conversations(organization_id,updated_at DESC,id) WHERE kind='website';
CREATE INDEX inbox_assigned ON conversations(organization_id,assigned_user_id,control_mode) WHERE kind='website';
CREATE INDEX inbox_mode ON conversations(organization_id,control_mode,updated_at DESC) WHERE kind='website';
CREATE INDEX unread_visitor_messages ON conversation_messages(conversation_id,sequence) WHERE role='user';
CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;
CREATE INDEX inbox_message_search ON conversation_messages USING gin(content public.gin_trgm_ops) WHERE status='completed';
UPDATE conversations c SET last_message_at=(SELECT max(created_at) FROM conversation_messages WHERE conversation_id=c.id),last_visitor_at=(SELECT max(created_at) FROM conversation_messages WHERE conversation_id=c.id AND role='user');
CREATE TABLE organization_invitations(
 id uuid PRIMARY KEY,organization_id uuid NOT NULL REFERENCES organizations(id),created_by uuid NOT NULL,
 role text NOT NULL CHECK(role IN ('admin','operator')),token_hash text NOT NULL UNIQUE,
 created_at timestamptz NOT NULL DEFAULT now(),expires_at timestamptz NOT NULL,revoked_at timestamptz,accepted_at timestamptz,accepted_by uuid REFERENCES users(id),
 FOREIGN KEY(organization_id,created_by) REFERENCES organization_members(organization_id,user_id)
);
CREATE INDEX invitations_organization ON organization_invitations(organization_id,created_at DESC);
