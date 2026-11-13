CREATE TABLE assistant_lead_settings(
 organization_id uuid NOT NULL,assistant_id uuid NOT NULL,enabled boolean NOT NULL DEFAULT false,revision bigint NOT NULL DEFAULT 1,
 title text NOT NULL,goal text NOT NULL,require_contact boolean NOT NULL DEFAULT true,success_messages jsonb NOT NULL DEFAULT '{}',
 PRIMARY KEY(organization_id,assistant_id),FOREIGN KEY(organization_id,assistant_id) REFERENCES assistants(organization_id,id)
);
CREATE TABLE assistant_lead_fields(
 organization_id uuid NOT NULL,assistant_id uuid NOT NULL,key text NOT NULL CHECK(key ~ '^[a-z][a-z0-9_]{0,39}$'),
 kind text NOT NULL CHECK(kind IN ('text','email','phone','number','select','inn_ru')),position integer NOT NULL,
 definition jsonb NOT NULL CHECK(jsonb_typeof(definition)='object'),
 PRIMARY KEY(organization_id,assistant_id,key),FOREIGN KEY(organization_id,assistant_id) REFERENCES assistant_lead_settings(organization_id,assistant_id)
);
ALTER TABLE conversation_messages ADD CONSTRAINT messages_lead_scope UNIQUE(organization_id,assistant_id,conversation_id,id);
CREATE TABLE lead_collections(
 id uuid PRIMARY KEY,organization_id uuid NOT NULL,assistant_id uuid NOT NULL,conversation_id uuid NOT NULL UNIQUE,
 state text NOT NULL CHECK(state IN ('idle','collecting','confirmation','submitted','canceled')),version bigint NOT NULL DEFAULT 1,
 snapshot jsonb NOT NULL,privacy_url text NOT NULL,values jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(values)='object'),
 extract_attempts integer NOT NULL DEFAULT 0,last_attempt_message_id uuid,error_code text NOT NULL DEFAULT '',
 lead_id uuid,confirmed_version bigint,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),submitted_at timestamptz,
 UNIQUE(organization_id,assistant_id,conversation_id,id),
 FOREIGN KEY(organization_id,assistant_id,conversation_id) REFERENCES conversations(organization_id,assistant_id,id) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,assistant_id,conversation_id,last_attempt_message_id) REFERENCES conversation_messages(organization_id,assistant_id,conversation_id,id)
);
CREATE TABLE leads(
 id uuid PRIMARY KEY,organization_id uuid NOT NULL,assistant_id uuid NOT NULL,site_id uuid NOT NULL,conversation_id uuid NOT NULL,
 collection_id uuid NOT NULL UNIQUE,name text NOT NULL DEFAULT '',email text NOT NULL DEFAULT '',phone text NOT NULL DEFAULT '',
 data jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(data)='object'),provenance jsonb NOT NULL DEFAULT '{}',contact_keys jsonb NOT NULL DEFAULT '{}',snapshot jsonb NOT NULL,privacy_url text NOT NULL,
 status text NOT NULL DEFAULT 'new' CHECK(status IN ('new','in_progress','won','lost')),assigned_user_id uuid,source text NOT NULL CHECK(source IN ('website','staff')),
 version bigint NOT NULL DEFAULT 1,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,id),
 FOREIGN KEY(organization_id,assistant_id,conversation_id,collection_id) REFERENCES lead_collections(organization_id,assistant_id,conversation_id,id),
 FOREIGN KEY(organization_id,assistant_id,site_id) REFERENCES sites(organization_id,assistant_id,id),
 FOREIGN KEY(organization_id,assigned_user_id) REFERENCES organization_members(organization_id,user_id)
);
ALTER TABLE lead_collections ADD CONSTRAINT collection_lead FOREIGN KEY(organization_id,lead_id) REFERENCES leads(organization_id,id) ON DELETE SET NULL(lead_id);
CREATE INDEX lead_org_created ON leads(organization_id,created_at DESC,id);
CREATE INDEX lead_org_status ON leads(organization_id,status,created_at DESC,id);
CREATE INDEX lead_org_assigned ON leads(organization_id,assigned_user_id,created_at DESC,id);
CREATE INDEX lead_conversation ON leads(organization_id,conversation_id);
CREATE TABLE lead_events(
 id uuid PRIMARY KEY,organization_id uuid NOT NULL,lead_id uuid NOT NULL,actor_id uuid,kind text NOT NULL,
 status text NOT NULL DEFAULT '',target_id uuid,note text NOT NULL DEFAULT '',created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,lead_id) REFERENCES leads(organization_id,id) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,actor_id) REFERENCES organization_members(organization_id,user_id),FOREIGN KEY(organization_id,target_id) REFERENCES organization_members(organization_id,user_id)
);
CREATE INDEX lead_event_history ON lead_events(lead_id,created_at,id);
