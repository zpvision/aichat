CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public;
CREATE TABLE knowledge_sources (
 id uuid PRIMARY KEY, organization_id uuid NOT NULL, assistant_id uuid NOT NULL,
 kind text NOT NULL CHECK(kind IN ('article','document','web')), title text NOT NULL,
 language text NOT NULL DEFAULT '', url text NOT NULL DEFAULT '', file_key text NOT NULL DEFAULT '', format text NOT NULL DEFAULT '',
 enabled boolean NOT NULL DEFAULT true, content text NOT NULL DEFAULT '', checksum text NOT NULL DEFAULT '',
 crawl boolean NOT NULL DEFAULT false, imported_pages integer NOT NULL DEFAULT 0,
 revision integer NOT NULL DEFAULT 1, active_revision integer,
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','processing','ready','failed')),
 error_code text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,assistant_id,id),
 FOREIGN KEY(organization_id,assistant_id) REFERENCES assistants(organization_id,id)
);
CREATE UNIQUE INDEX knowledge_web_url ON knowledge_sources(organization_id,assistant_id,url) WHERE kind='web';
CREATE UNIQUE INDEX knowledge_upload_checksum ON knowledge_sources(organization_id,assistant_id,checksum) WHERE kind='document';
CREATE TABLE knowledge_jobs (
 id uuid PRIMARY KEY, organization_id uuid NOT NULL, assistant_id uuid NOT NULL, source_id uuid NOT NULL,
 revision integer NOT NULL, state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','processing','ready','failed')),
 attempts integer NOT NULL DEFAULT 0, available_at timestamptz NOT NULL DEFAULT now(), lease_until timestamptz,
 lease_token uuid, error_code text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(source_id,revision),
 FOREIGN KEY(organization_id,assistant_id,source_id) REFERENCES knowledge_sources(organization_id,assistant_id,id) ON DELETE CASCADE
);
CREATE INDEX knowledge_job_queue ON knowledge_jobs(state,available_at);
CREATE TABLE knowledge_chunks (
 id uuid PRIMARY KEY, organization_id uuid NOT NULL, assistant_id uuid NOT NULL, source_id uuid NOT NULL,
 revision integer NOT NULL, position integer NOT NULL, page integer, content text NOT NULL,
 search tsvector GENERATED ALWAYS AS (to_tsvector('simple',content)) STORED,
 embedding public.vector NOT NULL, model text NOT NULL, model_version text NOT NULL, dimensions integer NOT NULL CHECK(dimensions>0),
 CHECK(public.vector_dims(embedding)=dimensions), UNIQUE(source_id,revision,position),
 FOREIGN KEY(organization_id,assistant_id,source_id) REFERENCES knowledge_sources(organization_id,assistant_id,id) ON DELETE CASCADE
);
CREATE INDEX knowledge_chunks_scope ON knowledge_chunks(organization_id,assistant_id,source_id,revision);
CREATE INDEX knowledge_chunks_text ON knowledge_chunks USING gin(search);
