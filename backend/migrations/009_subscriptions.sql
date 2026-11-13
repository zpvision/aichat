CREATE TABLE billing_settings(singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),region text NOT NULL CHECK(region IN ('ru','eu')),trial_days integer NOT NULL CHECK(trial_days BETWEEN 1 AND 90));
INSERT INTO billing_settings VALUES(true,COALESCE(NULLIF(current_setting('app.region',true),''),'eu'),14);
CREATE TABLE billing_plans(
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),code text NOT NULL CHECK(code IN ('trial','start','business')),name text NOT NULL,description text NOT NULL,
 version integer NOT NULL CHECK(version>0),region text NOT NULL CHECK(region IN ('ru','eu')),status text NOT NULL CHECK(status IN ('active','retired')),
 features text[] NOT NULL,offer jsonb NOT NULL DEFAULT '{"price":null}',period_days integer NOT NULL DEFAULT 30 CHECK(period_days BETWEEN 1 AND 366),
 assistants bigint CHECK(assistants>=0),sites bigint CHECK(sites>=0),members bigint CHECK(members>=0),knowledge_sources bigint CHECK(knowledge_sources>=0),
 knowledge_bytes bigint CHECK(knowledge_bytes>=0),ai_responses bigint CHECK(ai_responses>=0),demo_responses bigint CHECK(demo_responses>=0),
 ai_concurrent bigint CHECK(ai_concurrent>=0),api_per_minute bigint CHECK(api_per_minute>=0),UNIQUE(region,code,version),UNIQUE(id,region)
);
INSERT INTO billing_plans(code,name,description,version,region,status,features,assistants,sites,members,knowledge_sources,knowledge_bytes,ai_responses,demo_responses,ai_concurrent,api_per_minute)
SELECT c.code,c.name,'Demonstration limits; commercial prices are not configured.',1,r.region,'active',ARRAY['knowledge','website_chat','operator_inbox','leads','integrations'],
 c.assistants,c.sites,c.members,c.sources,c.bytes,c.responses,c.demo,c.concurrent,c.api
FROM (VALUES ('trial','Trial',3,3,5,30,52428800::bigint,100,500,2,120),('start','Start',5,5,10,200,524288000::bigint,1000,1000,4,300),('business','Business',20,20,50,500,5368709120::bigint,10000,2000,8,NULL::integer)) c(code,name,assistants,sites,members,sources,bytes,responses,demo,concurrent,api)
CROSS JOIN (VALUES('ru'),('eu'))r(region);
CREATE FUNCTION billing_plan_limits(p billing_plans) RETURNS jsonb LANGUAGE sql IMMUTABLE AS $$ SELECT jsonb_build_object('assistants',p.assistants,'sites',p.sites,'members',p.members,'knowledge_sources',p.knowledge_sources,'knowledge_bytes',p.knowledge_bytes,'ai_responses',p.ai_responses,'demo_responses',p.demo_responses,'ai_concurrent',p.ai_concurrent,'api_per_minute',p.api_per_minute) $$;
CREATE TABLE billing_subscriptions(
 organization_id uuid PRIMARY KEY REFERENCES organizations(id),plan_id uuid NOT NULL REFERENCES billing_plans(id),region text NOT NULL,
 status text NOT NULL CHECK(status IN ('trialing','active','expired','suspended')),started_at timestamptz NOT NULL,valid_until timestamptz NOT NULL,
 trial_end timestamptz,activation_source text NOT NULL,updated_at timestamptz NOT NULL DEFAULT now(),
 resources jsonb NOT NULL DEFAULT '{"assistants":0,"sites":0,"members":0,"knowledge_sources":0,"knowledge_bytes":0,"reserved_bytes":0}',
 FOREIGN KEY(plan_id,region) REFERENCES billing_plans(id,region)
);
CREATE TABLE billing_periods(
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),organization_id uuid NOT NULL REFERENCES billing_subscriptions(organization_id),plan_id uuid NOT NULL REFERENCES billing_plans(id),
 starts_at timestamptz NOT NULL,ends_at timestamptz NOT NULL CHECK(ends_at>starts_at),limits jsonb NOT NULL,
 ai_used bigint NOT NULL DEFAULT 0 CHECK(ai_used>=0),ai_reserved bigint NOT NULL DEFAULT 0 CHECK(ai_reserved>=0),
 demo_used bigint NOT NULL DEFAULT 0 CHECK(demo_used>=0),demo_reserved bigint NOT NULL DEFAULT 0 CHECK(demo_reserved>=0),
 UNIQUE(organization_id,starts_at),UNIQUE(organization_id,id)
);
CREATE INDEX billing_period_history ON billing_periods(organization_id,starts_at DESC);
CREATE TABLE billing_reservations(
 execution_id uuid PRIMARY KEY,organization_id uuid NOT NULL,period_id uuid NOT NULL,
 bucket text NOT NULL CHECK(bucket IN ('ai','demo')),state text NOT NULL CHECK(state IN ('reserved','used','released')),
 created_at timestamptz NOT NULL DEFAULT now(),settled_at timestamptz,
 FOREIGN KEY(organization_id,period_id) REFERENCES billing_periods(organization_id,id)
);
CREATE INDEX billing_active_reservations ON billing_reservations(organization_id) WHERE state='reserved';
CREATE TABLE billing_files(
 id uuid PRIMARY KEY,organization_id uuid NOT NULL REFERENCES billing_subscriptions(organization_id),file_key text NOT NULL UNIQUE,
 bytes bigint NOT NULL CHECK(bytes>=0),accounting_bucket text NOT NULL DEFAULT 'reserved' CHECK(accounting_bucket IN ('used','reserved')),state text NOT NULL CHECK(state IN ('reserved','attached','deleting','deleted')),
 lease_until timestamptz NOT NULL DEFAULT now()+interval '10 minutes',created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX billing_file_cleanup ON billing_files(lease_until) WHERE state IN ('reserved','deleting');
CREATE TABLE billing_audit(id uuid PRIMARY KEY DEFAULT gen_random_uuid(),organization_id uuid NOT NULL REFERENCES organizations(id),request_id uuid NOT NULL,
 actor text NOT NULL,action text NOT NULL,request_hash text NOT NULL,before_state jsonb NOT NULL,after_state jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),UNIQUE(organization_id,request_id));
ALTER TABLE ai_executions ADD COLUMN provider_invoked boolean NOT NULL DEFAULT false;
ALTER TABLE ai_executions ADD COLUMN commercial_counted boolean NOT NULL DEFAULT false;

CREATE FUNCTION billing_enroll() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p billing_plans;cfg billing_settings;finish timestamptz;
BEGIN SELECT * INTO cfg FROM billing_settings;SELECT * INTO STRICT p FROM billing_plans WHERE code='trial' AND region=cfg.region AND status='active' ORDER BY version DESC LIMIT 1;
 finish:=now()+cfg.trial_days*interval '1 day';
 INSERT INTO billing_subscriptions(organization_id,plan_id,region,status,started_at,valid_until,trial_end,activation_source) VALUES(NEW.id,p.id,cfg.region,'trialing',now(),finish,finish,'trial');
 INSERT INTO billing_periods(organization_id,plan_id,starts_at,ends_at,limits) VALUES(NEW.id,p.id,now(),finish,billing_plan_limits(p));RETURN NEW;END $$;
CREATE TRIGGER organization_trial AFTER INSERT ON organizations FOR EACH ROW EXECUTE FUNCTION billing_enroll();
INSERT INTO billing_subscriptions(organization_id,plan_id,region,status,started_at,valid_until,trial_end,activation_source,resources)
 SELECT o.id,p.id,p.region,'trialing',now(),now()+cfg.trial_days*interval '1 day',now()+cfg.trial_days*interval '1 day','migration',
 jsonb_build_object('assistants',(SELECT count(*) FROM assistants WHERE organization_id=o.id),'sites',(SELECT count(*) FROM sites WHERE organization_id=o.id),'members',(SELECT count(*) FROM organization_members WHERE organization_id=o.id),'knowledge_sources',(SELECT count(*) FROM knowledge_sources WHERE organization_id=o.id),'knowledge_bytes',0,'reserved_bytes',0)
 FROM organizations o CROSS JOIN billing_settings cfg JOIN billing_plans p ON p.code='trial' AND p.version=1 AND p.region=cfg.region;
INSERT INTO billing_periods(organization_id,plan_id,starts_at,ends_at,limits) SELECT s.organization_id,s.plan_id,s.started_at,s.valid_until,billing_plan_limits(p) FROM billing_subscriptions s JOIN billing_plans p ON p.id=s.plan_id;

CREATE FUNCTION billing_ensure(org uuid) RETURNS uuid LANGUAGE plpgsql AS $$
DECLARE sub billing_subscriptions;per billing_periods;p billing_plans;next_end timestamptz;
BEGIN SELECT * INTO STRICT sub FROM billing_subscriptions WHERE organization_id=org FOR UPDATE;
 IF sub.valid_until<=now() AND sub.status<>'suspended' THEN UPDATE billing_subscriptions SET status='expired',updated_at=now() WHERE organization_id=org;sub.status:='expired';END IF;
 SELECT * INTO STRICT per FROM billing_periods WHERE organization_id=org ORDER BY starts_at DESC LIMIT 1;
 IF sub.status='active' THEN
  WHILE per.ends_at<=now() AND per.ends_at<sub.valid_until LOOP
   SELECT * INTO STRICT p FROM billing_plans WHERE id=sub.plan_id;
   next_end:=LEAST(per.ends_at+p.period_days*interval '1 day',sub.valid_until);
   INSERT INTO billing_periods(organization_id,plan_id,starts_at,ends_at,limits) VALUES(org,p.id,per.ends_at,next_end,billing_plan_limits(p)) RETURNING * INTO per;
  END LOOP;
 END IF; RETURN per.id;END $$;
CREATE FUNCTION billing_resource() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE org uuid;key text;sub billing_subscriptions;per uuid;cap bigint;n bigint;file billing_files;
BEGIN
 org:=CASE WHEN TG_OP='DELETE' THEN OLD.organization_id ELSE NEW.organization_id END;
 key:=CASE TG_TABLE_NAME WHEN 'assistants' THEN 'assistants' WHEN 'sites' THEN 'sites' WHEN 'organization_members' THEN 'members' ELSE 'knowledge_sources' END;
 per:=billing_ensure(org);SELECT * INTO sub FROM billing_subscriptions WHERE organization_id=org;
 n:=(sub.resources->>key)::bigint+CASE WHEN TG_OP='DELETE' THEN -1 ELSE 1 END;
 IF TG_OP='INSERT' THEN SELECT (limits->>key)::bigint INTO cap FROM billing_periods WHERE id=per;IF cap IS NOT NULL AND n>cap THEN RAISE EXCEPTION USING MESSAGE='plan_limit_exceeded',DETAIL=key;END IF;END IF;
 UPDATE billing_subscriptions SET resources=jsonb_set(resources,ARRAY[key],to_jsonb(GREATEST(0,n))) WHERE organization_id=org;
 IF TG_TABLE_NAME='knowledge_sources' THEN
  IF TG_OP='INSERT' AND NEW.kind='document' THEN
   SELECT * INTO file FROM billing_files WHERE id=NEW.id AND organization_id=org FOR UPDATE;
   IF FOUND THEN
    IF file.state<>'reserved' THEN RAISE EXCEPTION 'storage_reservation_invalid';END IF;
    UPDATE billing_subscriptions SET resources=jsonb_set(jsonb_set(resources,'{reserved_bytes}',to_jsonb((resources->>'reserved_bytes')::bigint-file.bytes)),'{knowledge_bytes}',to_jsonb((resources->>'knowledge_bytes')::bigint+file.bytes)) WHERE organization_id=org;
    UPDATE billing_files SET state='attached',accounting_bucket='used' WHERE id=file.id;
   END IF;
  ELSIF TG_OP='DELETE' THEN UPDATE billing_files SET state='deleting',lease_until=now() WHERE id=OLD.id AND organization_id=org AND state='attached';END IF;
 END IF;
 RETURN NULL;END $$;
CREATE TRIGGER assistants_plan AFTER INSERT OR DELETE ON assistants FOR EACH ROW EXECUTE FUNCTION billing_resource();
CREATE TRIGGER sites_plan AFTER INSERT OR DELETE ON sites FOR EACH ROW EXECUTE FUNCTION billing_resource();
CREATE TRIGGER members_plan AFTER INSERT OR DELETE ON organization_members FOR EACH ROW EXECUTE FUNCTION billing_resource();
CREATE TRIGGER knowledge_plan AFTER INSERT OR DELETE ON knowledge_sources FOR EACH ROW EXECUTE FUNCTION billing_resource();

CREATE FUNCTION billing_reserve(org uuid,execution uuid) RETURNS void LANGUAGE plpgsql AS $$
DECLARE row_e ai_executions;row_c conversations;per_id uuid;per billing_periods;sub billing_subscriptions;bucket_name text;cap bigint;used bigint;held bigint;concurrent_cap bigint;running bigint;
BEGIN
 SELECT * INTO row_e FROM ai_executions WHERE id=execution AND organization_id=org;IF NOT FOUND THEN RAISE EXCEPTION 'ai_canceled';END IF;
 PERFORM id FROM conversations WHERE id=row_e.conversation_id AND organization_id=org FOR UPDATE;IF NOT FOUND THEN RAISE EXCEPTION 'ai_canceled';END IF;
 SELECT * INTO STRICT row_e FROM ai_executions WHERE id=execution AND organization_id=org FOR UPDATE;SELECT * INTO STRICT row_c FROM conversations WHERE id=row_e.conversation_id AND organization_id=org;
 IF row_e.status<>'processing' OR (row_c.kind='website' AND (row_c.control_mode<>'ai_active' OR row_c.state_version<>row_e.state_version)) THEN RAISE EXCEPTION 'ai_canceled';END IF;
 IF EXISTS(SELECT 1 FROM billing_reservations WHERE execution_id=execution AND state='reserved') THEN RETURN;END IF;
 per_id:=billing_ensure(org);SELECT * INTO STRICT per FROM billing_periods WHERE id=per_id;SELECT * INTO sub FROM billing_subscriptions WHERE organization_id=org;
 IF sub.status NOT IN ('trialing','active') OR sub.valid_until<=now() THEN RAISE EXCEPTION 'subscription_inactive';END IF;
 bucket_name:=CASE WHEN row_e.provider='mock' THEN 'demo' ELSE 'ai' END;
 IF bucket_name='demo' THEN cap:=(per.limits->>'demo_responses')::bigint;used:=per.demo_used;held:=per.demo_reserved;ELSE cap:=(per.limits->>'ai_responses')::bigint;used:=per.ai_used;held:=per.ai_reserved;END IF;
 IF cap IS NOT NULL AND used+held>=cap THEN RAISE EXCEPTION 'plan_limit_exceeded';END IF;
 concurrent_cap:=(per.limits->>'ai_concurrent')::bigint;SELECT count(*) INTO running FROM billing_reservations WHERE organization_id=org AND state='reserved';
 IF concurrent_cap IS NOT NULL AND running>=concurrent_cap THEN RAISE EXCEPTION 'plan_limit_exceeded';END IF;
 INSERT INTO billing_reservations(execution_id,organization_id,period_id,bucket,state) VALUES(execution,org,per.id,bucket_name,'reserved');
 UPDATE billing_periods SET ai_reserved=ai_reserved+CASE WHEN bucket_name='ai' THEN 1 ELSE 0 END,demo_reserved=demo_reserved+CASE WHEN bucket_name='demo' THEN 1 ELSE 0 END WHERE id=per.id;
 UPDATE ai_executions SET provider_invoked=true WHERE id=execution AND organization_id=org;
END $$;
CREATE FUNCTION billing_settle() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r billing_reservations;success boolean;
BEGIN
 IF TG_OP='UPDATE' AND OLD.status<>'processing' THEN RETURN NEW;END IF;
 IF TG_OP='UPDATE' AND NEW.status='processing' THEN RETURN NEW;END IF;
 PERFORM organization_id FROM billing_subscriptions WHERE organization_id=OLD.organization_id FOR UPDATE;
 SELECT * INTO r FROM billing_reservations WHERE execution_id=OLD.id FOR UPDATE;
 IF FOUND AND r.state='reserved' THEN
  success:=TG_OP='UPDATE' AND NEW.status='completed' AND NEW.provider_invoked;
  UPDATE billing_periods SET ai_reserved=ai_reserved-CASE WHEN r.bucket='ai' THEN 1 ELSE 0 END,demo_reserved=demo_reserved-CASE WHEN r.bucket='demo' THEN 1 ELSE 0 END,
   ai_used=ai_used+CASE WHEN success AND r.bucket='ai' THEN 1 ELSE 0 END,demo_used=demo_used+CASE WHEN success AND r.bucket='demo' THEN 1 ELSE 0 END WHERE id=r.period_id;
  UPDATE billing_reservations SET state=CASE WHEN success THEN 'used' ELSE 'released' END,settled_at=now() WHERE execution_id=r.execution_id;
  IF TG_OP='UPDATE' THEN NEW.commercial_counted:=success AND r.bucket='ai';END IF;
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD;ELSE RETURN NEW;END IF;END $$;
CREATE TRIGGER execution_billing BEFORE UPDATE OF status OR DELETE ON ai_executions FOR EACH ROW EXECUTE FUNCTION billing_settle();
