CREATE TABLE object_files(
 id uuid PRIMARY KEY,organization_id uuid NOT NULL REFERENCES organizations(id),purpose text NOT NULL CHECK(purpose IN ('knowledge','logos')),
 provider text NOT NULL CHECK(provider IN ('local','s3')),object_key text NOT NULL,identity text NOT NULL DEFAULT '',
 bytes bigint CHECK(bytes>=0),sha256 text NOT NULL DEFAULT '',state text NOT NULL CHECK(state IN ('pending','ready','deleted')),
 created_at timestamptz NOT NULL DEFAULT now(),UNIQUE(organization_id,id),UNIQUE(provider,identity,object_key,purpose)
);
INSERT INTO object_files(id,organization_id,purpose,provider,object_key,sha256,state)
 SELECT file_key::uuid,organization_id,'knowledge','local',file_key,checksum,'ready' FROM knowledge_sources WHERE kind='document';
INSERT INTO object_files(id,organization_id,purpose,provider,object_key,state)
 SELECT logo_key::uuid,organization_id,'logos','local',logo_key,'ready' FROM widgets WHERE logo_key<>'' ON CONFLICT(id) DO NOTHING;
