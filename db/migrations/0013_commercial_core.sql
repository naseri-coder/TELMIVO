-- R3D-2: existing plans/subscriptions remain the account plan identity and assignment.
-- Typed, immutable catalog versions cover policies/prices/products without parallel catalogs per noun.
CREATE TABLE commercial_catalog (
 kind text NOT NULL CHECK(kind IN('PLAN','PLUGIN','BUNDLE','ADDON','QUOTA')),
 key text NOT NULL CHECK(key ~ '^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,99}$'),
 version bigint NOT NULL CHECK(version>0), enabled boolean NOT NULL,
 effective_at timestamptz NOT NULL, definition jsonb NOT NULL CHECK(jsonb_typeof(definition)='object'),
 actor_id uuid REFERENCES users(id), reason text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(kind,key,version)
);
CREATE TRIGGER commercial_catalog_immutable BEFORE UPDATE OR DELETE ON commercial_catalog FOR EACH ROW EXECUTE FUNCTION reject_mutation();
CREATE INDEX commercial_catalog_effective ON commercial_catalog(kind,key,effective_at,version DESC);
CREATE TABLE commercial_agreements (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), workspace_id uuid NOT NULL REFERENCES workspaces(id),
 operation_key text NOT NULL, fingerprint text NOT NULL,
 bot_id uuid, license_scope text NOT NULL CHECK(license_scope IN('PER_ACCOUNT','PER_BOT')),
 source text NOT NULL CHECK(source IN('SUBSCRIPTION','TRIAL','BUNDLE','ADDON','ADMIN')),
 state text NOT NULL DEFAULT 'ACTIVE' CHECK(state IN('ACTIVE','GRACE','SUSPENDED','CANCELLED','EXPIRED')),
 starts_at timestamptz NOT NULL, ends_at timestamptz, grace_until timestamptz,
 catalog_kind text NOT NULL, catalog_key text NOT NULL, catalog_version bigint NOT NULL,
 terms jsonb NOT NULL CHECK(jsonb_typeof(terms)='object'), revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 actor_id uuid NOT NULL REFERENCES users(id), reason text NOT NULL,
 UNIQUE(workspace_id,operation_key), UNIQUE(id,workspace_id),
 FOREIGN KEY(bot_id,workspace_id) REFERENCES customer_bots(id,workspace_id),
 FOREIGN KEY(catalog_kind,catalog_key,catalog_version) REFERENCES commercial_catalog(kind,key,version),
 CHECK((license_scope='PER_ACCOUNT' AND bot_id IS NULL) OR (license_scope='PER_BOT' AND bot_id IS NOT NULL)),
 CHECK(ends_at IS NULL OR ends_at>starts_at), CHECK(grace_until IS NULL OR (ends_at IS NOT NULL AND grace_until>=ends_at))
);
CREATE INDEX commercial_agreements_access ON commercial_agreements(workspace_id,bot_id,state);
CREATE FUNCTION guard_commercial_agreement() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'commercial history cannot be deleted' USING ERRCODE='23514'; END IF;
 IF (to_jsonb(NEW)-ARRAY['state','revision']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['state','revision']) OR NEW.revision<>OLD.revision+1 THEN
  RAISE EXCEPTION 'commercial terms are immutable' USING ERRCODE='23514';
 END IF;
 -- Cancelled/expired agreements cannot be resurrected; restoration issues a new agreement.
 IF OLD.state IN('CANCELLED','EXPIRED') OR NEW.state NOT IN('SUSPENDED','CANCELLED','EXPIRED','ACTIVE') THEN
  RAISE EXCEPTION 'invalid commercial transition' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER commercial_agreement_guard BEFORE UPDATE OR DELETE ON commercial_agreements FOR EACH ROW EXECUTE FUNCTION guard_commercial_agreement();
CREATE TABLE commercial_trial_consumptions (
 workspace_id uuid NOT NULL REFERENCES workspaces(id), plugin_id text NOT NULL,
 license_scope text NOT NULL CHECK(license_scope IN('PER_ACCOUNT','PER_BOT')), subject_id text NOT NULL,
 agreement_id uuid NOT NULL, consumed_at timestamptz NOT NULL,
 PRIMARY KEY(workspace_id,plugin_id,license_scope,subject_id),
 FOREIGN KEY(agreement_id,workspace_id) REFERENCES commercial_agreements(id,workspace_id)
);
CREATE TRIGGER commercial_trial_immutable BEFORE UPDATE OR DELETE ON commercial_trial_consumptions FOR EACH ROW EXECUTE FUNCTION reject_mutation();
CREATE TABLE commercial_usage (
 workspace_id uuid NOT NULL REFERENCES workspaces(id), metric text NOT NULL,
 subject_id text NOT NULL, bucket text NOT NULL, used bigint NOT NULL CHECK(used>=0),
 PRIMARY KEY(workspace_id,metric,subject_id,bucket)
);
ALTER TABLE commercial_agreements ENABLE ROW LEVEL SECURITY;
ALTER TABLE commercial_agreements FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope ON commercial_agreements USING(workspace_id::text=current_setting('app.workspace_id',true)) WITH CHECK(workspace_id::text=current_setting('app.workspace_id',true));
ALTER TABLE commercial_trial_consumptions ENABLE ROW LEVEL SECURITY;
ALTER TABLE commercial_trial_consumptions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope ON commercial_trial_consumptions USING(workspace_id::text=current_setting('app.workspace_id',true)) WITH CHECK(workspace_id::text=current_setting('app.workspace_id',true));
ALTER TABLE commercial_usage ENABLE ROW LEVEL SECURITY;
ALTER TABLE commercial_usage FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope ON commercial_usage USING(workspace_id::text=current_setting('app.workspace_id',true)) WITH CHECK(workspace_id::text=current_setting('app.workspace_id',true));
-- Central initial plan labels; no plugin price or commercial inclusion is hard-coded.
INSERT INTO commercial_catalog(kind,key,version,enabled,effective_at,definition,reason)
 SELECT 'PLAN',key,1,active,'2020-01-01T00:00:00Z',jsonb_build_object('plan',jsonb_build_object('name',name,'order',CASE key WHEN 'FREE' THEN 0 WHEN 'PRO' THEN 10 ELSE 20 END,'plugins','{}'::jsonb,'limits','{}'::jsonb)),'R3D-2 initial plan policy' FROM plans WHERE key IN('FREE','PRO','BUSINESS');
