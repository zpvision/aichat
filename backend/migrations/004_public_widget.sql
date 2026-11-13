ALTER TABLE sites ADD CONSTRAINT sites_widget_scope UNIQUE(organization_id,assistant_id,id);
CREATE TABLE widgets (
 id uuid PRIMARY KEY,organization_id uuid NOT NULL,assistant_id uuid NOT NULL,site_id uuid NOT NULL UNIQUE,
 enabled boolean NOT NULL DEFAULT false,origins text[] NOT NULL DEFAULT '{}',settings jsonb NOT NULL DEFAULT '{}',
 logo_key text NOT NULL DEFAULT '',verification_token text NOT NULL,verified_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,assistant_id,site_id,id),
 FOREIGN KEY(organization_id,assistant_id,site_id) REFERENCES sites(organization_id,assistant_id,id) ON DELETE CASCADE
);
CREATE TABLE widget_sessions (
 id uuid PRIMARY KEY,widget_id uuid NOT NULL,organization_id uuid NOT NULL,assistant_id uuid NOT NULL,site_id uuid NOT NULL,
 token_hash text NOT NULL UNIQUE,origin text NOT NULL,message_count integer NOT NULL DEFAULT 0,
 created_at timestamptz NOT NULL DEFAULT now(),expires_at timestamptz NOT NULL,
 UNIQUE(organization_id,assistant_id,site_id,id),
 FOREIGN KEY(organization_id,assistant_id,site_id,widget_id) REFERENCES widgets(organization_id,assistant_id,site_id,id) ON DELETE CASCADE
);
CREATE INDEX widget_sessions_site_created ON widget_sessions(site_id,created_at);
CREATE INDEX widget_sessions_expiry ON widget_sessions(expires_at);
ALTER TABLE conversations DROP CONSTRAINT conversations_kind_check;
ALTER TABLE conversations ADD CONSTRAINT conversations_kind_check CHECK(kind IN ('preview','website'));
ALTER TABLE conversations ALTER COLUMN created_by DROP NOT NULL;
ALTER TABLE conversations ADD COLUMN site_id uuid;
ALTER TABLE conversations ADD COLUMN anonymous_session_id uuid;
ALTER TABLE conversations ADD CONSTRAINT conversations_channel CHECK(
 (kind='preview' AND created_by IS NOT NULL AND site_id IS NULL AND anonymous_session_id IS NULL) OR
 (kind='website' AND created_by IS NULL AND site_id IS NOT NULL AND anonymous_session_id IS NOT NULL));
ALTER TABLE conversations ADD CONSTRAINT conversations_visitor_scope FOREIGN KEY(organization_id,assistant_id,site_id,anonymous_session_id)
 REFERENCES widget_sessions(organization_id,assistant_id,site_id,id) ON DELETE CASCADE;
CREATE UNIQUE INDEX one_website_conversation_per_session ON conversations(anonymous_session_id) WHERE kind='website';
CREATE INDEX ai_execution_org_created ON ai_executions(organization_id,created_at);
