CREATE TABLE assistant_ai_settings (
 organization_id uuid NOT NULL, assistant_id uuid NOT NULL,
 display_name text NOT NULL DEFAULT '', role text NOT NULL DEFAULT '', company_description text NOT NULL DEFAULT '',
 style text NOT NULL DEFAULT '', instructions text NOT NULL DEFAULT '', no_answer text NOT NULL DEFAULT '',
 language_policy text NOT NULL DEFAULT 'auto' CHECK(language_policy IN ('auto','fixed','default')),
 response_locale text NOT NULL DEFAULT 'en' CHECK(response_locale IN ('ru','en','cs','de')),
 offer_operator boolean NOT NULL DEFAULT true, updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(organization_id,assistant_id), FOREIGN KEY(organization_id,assistant_id) REFERENCES assistants(organization_id,id)
);
CREATE TABLE conversations (
 id uuid PRIMARY KEY, organization_id uuid NOT NULL, assistant_id uuid NOT NULL,
 kind text NOT NULL DEFAULT 'preview' CHECK(kind='preview'), status text NOT NULL DEFAULT 'open' CHECK(status IN ('open','closed')),
 created_by uuid NOT NULL REFERENCES users(id), created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,assistant_id,id),FOREIGN KEY(organization_id,assistant_id) REFERENCES assistants(organization_id,id)
);
CREATE INDEX conversations_scope ON conversations(organization_id,assistant_id,updated_at DESC);
CREATE TABLE ai_executions (
 id uuid PRIMARY KEY, organization_id uuid NOT NULL,assistant_id uuid NOT NULL,conversation_id uuid NOT NULL,
 request_id uuid NOT NULL, request_hash text NOT NULL, provider text NOT NULL, model text NOT NULL,
 status text NOT NULL CHECK(status IN ('processing','completed','failed','canceled')),
 duration_ms bigint NOT NULL DEFAULT 0, input_tokens bigint, output_tokens bigint, provider_duration_ns bigint,
 error_code text NOT NULL DEFAULT '', sources jsonb NOT NULL DEFAULT '[]', settings_snapshot jsonb NOT NULL DEFAULT '{}',
 response_language text NOT NULL DEFAULT '', policy_fallback boolean NOT NULL DEFAULT false, created_at timestamptz NOT NULL DEFAULT now(), finished_at timestamptz, expires_at timestamptz NOT NULL,
 UNIQUE(conversation_id,request_id), UNIQUE(organization_id,assistant_id,conversation_id,id),
 FOREIGN KEY(organization_id,assistant_id,conversation_id) REFERENCES conversations(organization_id,assistant_id,id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX one_active_ai_execution ON ai_executions(conversation_id) WHERE status='processing';
CREATE INDEX ai_execution_expiry ON ai_executions(expires_at) WHERE status='processing';
CREATE TABLE conversation_messages (
 id uuid PRIMARY KEY, organization_id uuid NOT NULL,assistant_id uuid NOT NULL,conversation_id uuid NOT NULL,execution_id uuid NOT NULL,
 sequence bigint GENERATED ALWAYS AS IDENTITY UNIQUE,
 role text NOT NULL CHECK(role IN ('user','assistant')),content text NOT NULL DEFAULT '',language text NOT NULL DEFAULT '',
 status text NOT NULL CHECK(status IN ('processing','completed','failed','canceled')),created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(execution_id,role),
 FOREIGN KEY(organization_id,assistant_id,conversation_id,execution_id) REFERENCES ai_executions(organization_id,assistant_id,conversation_id,id) ON DELETE CASCADE
);
CREATE INDEX conversation_message_history ON conversation_messages(organization_id,assistant_id,conversation_id,sequence);
