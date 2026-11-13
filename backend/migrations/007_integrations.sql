CREATE TABLE integration_keys(
 id uuid PRIMARY KEY,organization_id uuid NOT NULL REFERENCES organizations(id),name text NOT NULL,
 token_hash text NOT NULL UNIQUE,scopes text[] NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),
 expires_at timestamptz NOT NULL,revoked_at timestamptz,last_used_at timestamptz,
 window_start timestamptz NOT NULL DEFAULT now(),request_count integer NOT NULL DEFAULT 0,
 UNIQUE(organization_id,id)
);
CREATE TABLE webhook_endpoints(
 id uuid PRIMARY KEY,organization_id uuid NOT NULL REFERENCES organizations(id),url text NOT NULL,
 secret_cipher bytea NOT NULL,enabled boolean NOT NULL DEFAULT true,version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(),UNIQUE(organization_id,id)
);
CREATE TABLE integration_events(
 seq bigserial PRIMARY KEY,id uuid NOT NULL UNIQUE,organization_id uuid NOT NULL REFERENCES organizations(id),
 event_type text NOT NULL CHECK(event_type IN ('lead.created','webhook.test')),lead_id uuid,
 created_at timestamptz NOT NULL DEFAULT now(),UNIQUE(organization_id,id)
);
CREATE INDEX integration_event_org_seq ON integration_events(organization_id,seq);
CREATE TABLE webhook_deliveries(
 id uuid PRIMARY KEY,organization_id uuid NOT NULL,endpoint_id uuid NOT NULL,event_id uuid NOT NULL,
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','processing','delivered','failed','canceled')),
 attempts integer NOT NULL DEFAULT 0,max_attempts integer NOT NULL DEFAULT 8,next_attempt_at timestamptz NOT NULL DEFAULT now(),
 lease_token uuid,lease_until timestamptz,last_code text NOT NULL DEFAULT '',http_status integer NOT NULL DEFAULT 0,
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(endpoint_id,event_id),FOREIGN KEY(organization_id,endpoint_id) REFERENCES webhook_endpoints(organization_id,id),
 FOREIGN KEY(organization_id,event_id) REFERENCES integration_events(organization_id,id)
);
CREATE INDEX webhook_queue ON webhook_deliveries(next_attempt_at) WHERE status IN ('pending','processing');
CREATE INDEX webhook_history ON webhook_deliveries(organization_id,created_at DESC,id);
CREATE TABLE webhook_attempts(
 id uuid PRIMARY KEY,organization_id uuid NOT NULL,delivery_id uuid NOT NULL REFERENCES webhook_deliveries(id),
 attempt integer NOT NULL,code text NOT NULL,http_status integer NOT NULL,duration_ms bigint NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE integration_requests(
 organization_id uuid NOT NULL REFERENCES organizations(id),request_key text NOT NULL,body_hash text NOT NULL,
 lead_id uuid NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(organization_id,request_key)
);
CREATE TABLE integration_budgets(organization_id uuid PRIMARY KEY REFERENCES organizations(id),window_start timestamptz NOT NULL,request_count integer NOT NULL);
ALTER TABLE leads ALTER COLUMN site_id DROP NOT NULL;
ALTER TABLE leads ALTER COLUMN conversation_id DROP NOT NULL;
ALTER TABLE leads ALTER COLUMN collection_id DROP NOT NULL;
ALTER TABLE leads DROP CONSTRAINT leads_source_check;
ALTER TABLE leads ADD CONSTRAINT leads_source_check CHECK(source IN ('website','staff','api'));

-- Serialize event allocation per tenant before assigning seq: a committed cursor cannot skip a late commit.
CREATE FUNCTION integration_event_lock() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN PERFORM id FROM organizations WHERE id=NEW.organization_id FOR UPDATE;RETURN NEW;END $$;
CREATE TRIGGER lead_integration_lock BEFORE INSERT ON leads FOR EACH ROW EXECUTE FUNCTION integration_event_lock();
CREATE FUNCTION integration_lead_created() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE event_uuid uuid:=gen_random_uuid();
BEGIN
 INSERT INTO integration_events(id,organization_id,event_type,lead_id) VALUES(event_uuid,NEW.organization_id,'lead.created',NEW.id);
 INSERT INTO webhook_deliveries(id,organization_id,endpoint_id,event_id)
 SELECT gen_random_uuid(),NEW.organization_id,id,event_uuid FROM webhook_endpoints WHERE organization_id=NEW.organization_id AND enabled;
 RETURN NEW;
END $$;
CREATE TRIGGER lead_integration_created AFTER INSERT ON leads FOR EACH ROW EXECUTE FUNCTION integration_lead_created();
-- Existing leads are available through the recovery feed; no retroactive network deliveries.
INSERT INTO integration_events(id,organization_id,event_type,lead_id,created_at)
 SELECT gen_random_uuid(),organization_id,'lead.created',id,created_at FROM leads ORDER BY created_at,id;
