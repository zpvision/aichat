CREATE TABLE users (
 id uuid PRIMARY KEY, email text NOT NULL UNIQUE, password_hash text NOT NULL,
 name text NOT NULL DEFAULT '', preferred_locale text NOT NULL CHECK (preferred_locale IN ('ru','en','cs','de')),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE sessions (
 token_hash text PRIMARY KEY, user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sessions_user_idx ON sessions(user_id);
CREATE INDEX sessions_expiry_idx ON sessions(expires_at);
CREATE TABLE organizations (
 id uuid PRIMARY KEY, name text NOT NULL, country text NOT NULL CHECK (country ~ '^[A-Z]{2}$'),
 timezone text NOT NULL, default_locale text NOT NULL CHECK (default_locale IN ('ru','en','cs','de')),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE organization_members (
 organization_id uuid NOT NULL REFERENCES organizations(id), user_id uuid NOT NULL REFERENCES users(id),
 role text NOT NULL CHECK (role IN ('owner','admin','operator')), created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (organization_id,user_id)
);
CREATE INDEX organization_members_user_idx ON organization_members(user_id);
CREATE TABLE assistants (
 id uuid PRIMARY KEY, organization_id uuid NOT NULL REFERENCES organizations(id), name text NOT NULL,
 description text NOT NULL DEFAULT '', default_locale text NOT NULL CHECK (default_locale IN ('ru','en','cs','de')),
 language_policy text NOT NULL CHECK (language_policy IN ('visitor','fixed')),
 status text NOT NULL DEFAULT 'draft' CHECK (status = 'draft'),
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE (organization_id,id)
);
CREATE TABLE sites (
 id uuid PRIMARY KEY, organization_id uuid NOT NULL REFERENCES organizations(id), assistant_id uuid NOT NULL,
 name text NOT NULL, domain text NOT NULL, status text NOT NULL DEFAULT 'unverified' CHECK (status = 'unverified'),
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(organization_id,domain),
 FOREIGN KEY (organization_id,assistant_id) REFERENCES assistants(organization_id,id)
);
CREATE INDEX sites_assistant_idx ON sites(organization_id,assistant_id);
