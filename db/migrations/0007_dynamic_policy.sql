-- R2A extends existing feature/plugin/plan catalogs. Policy definitions are immutable versions.
INSERT INTO platform_permissions(key) VALUES
 ('platform.access_policies.read'),('platform.access_policies.manage'),
 ('platform.commercial.read'),('platform.commercial.manage'),('platform.plugins.read'),
 ('platform.offers.manage'),('platform.safety_limits.read');
-- Existing admins receive no new authority automatically; configured owners get explicit grants at login.
CREATE TABLE policy_versions (
 key text NOT NULL, version bigint NOT NULL CHECK(version>0),
 kind text NOT NULL CHECK(kind IN('TRIAL','QUOTA','RESOURCE','GRANT','PRICE','OFFER','PAYWALL')),
 feature_key text REFERENCES features(key), plugin_key text REFERENCES plugins(key),
 plan_key text REFERENCES plans(key), workspace_id uuid REFERENCES workspaces(id),
 enabled boolean NOT NULL, definition jsonb NOT NULL CHECK(jsonb_typeof(definition)='object'),
 effective_from timestamptz NOT NULL, effective_until timestamptz,
 actor_id uuid REFERENCES users(id), reason text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(),
 CHECK(effective_until IS NULL OR effective_until>effective_from), PRIMARY KEY(key,version)
);
ALTER TABLE policy_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE policy_versions FORCE ROW LEVEL SECURITY;
CREATE POLICY policy_scope ON policy_versions USING(workspace_id IS NULL OR workspace_id::text=current_setting('app.workspace_id',true)) WITH CHECK(workspace_id IS NULL OR workspace_id::text=current_setting('app.workspace_id',true));
CREATE INDEX policy_resolution ON policy_versions(kind,feature_key,plan_key,effective_from);
CREATE TRIGGER policy_versions_immutable BEFORE UPDATE OR DELETE ON policy_versions FOR EACH ROW EXECUTE FUNCTION reject_mutation();
CREATE TABLE identity_entries (
 user_id uuid NOT NULL REFERENCES users(id), source text NOT NULL CHECK(source IN('MAIN_BOT','GROUP_BOT','DOWNLOADER_BOT','MINIAPP')),
 first_seen_at timestamptz NOT NULL DEFAULT now(), last_seen_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(user_id,source)
);
-- Global uniqueness prevents a second workspace claiming an existing Telegram group. No settings live here.
CREATE TABLE group_claims (
 telegram_chat_id bigint PRIMARY KEY CHECK(telegram_chat_id<0), workspace_id uuid NOT NULL REFERENCES workspaces(id),
 group_id uuid NOT NULL, claimed_by uuid NOT NULL REFERENCES users(id), created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(group_id,workspace_id) REFERENCES groups(id,workspace_id)
);
ALTER TABLE group_claims ENABLE ROW LEVEL SECURITY;
ALTER TABLE group_claims FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope ON group_claims USING(workspace_id::text=current_setting('app.workspace_id',true)) WITH CHECK(workspace_id::text=current_setting('app.workspace_id',true));
CREATE TRIGGER group_claims_immutable BEFORE UPDATE OR DELETE ON group_claims FOR EACH ROW EXECUTE FUNCTION reject_mutation();
CREATE TABLE trial_enrollments (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), policy_key text NOT NULL, policy_version bigint NOT NULL,
 workspace_id uuid NOT NULL REFERENCES workspaces(id), user_id uuid NOT NULL REFERENCES users(id),
 scope text NOT NULL CHECK(scope IN('PER_USER','PER_WORKSPACE','PER_GROUP','PER_INSTALLATION')),
 subject_id text NOT NULL, cycle_key text NOT NULL, source text NOT NULL,
 started_at timestamptz NOT NULL, ends_at timestamptz NOT NULL, snapshot jsonb NOT NULL,
 FOREIGN KEY(policy_key,policy_version) REFERENCES policy_versions(key,version), CHECK(ends_at>started_at),
 UNIQUE(policy_key,scope,subject_id,cycle_key)
);
CREATE INDEX trial_active ON trial_enrollments(user_id,ends_at);
CREATE TRIGGER trial_enrollments_immutable BEFORE UPDATE OR DELETE ON trial_enrollments FOR EACH ROW EXECUTE FUNCTION reject_mutation();
ALTER TABLE trial_enrollments ENABLE ROW LEVEL SECURITY;
ALTER TABLE trial_enrollments FORCE ROW LEVEL SECURITY;
CREATE POLICY trial_scope ON trial_enrollments USING (
 workspace_id::text=current_setting('app.workspace_id',true) OR
 user_id::text=current_setting('app.user_id',true)
) WITH CHECK(workspace_id::text=current_setting('app.workspace_id',true) AND user_id::text=current_setting('app.user_id',true));
CREATE TABLE plugin_capabilities (
 plugin_key text REFERENCES plugins(key), key text PRIMARY KEY, permission_key text NOT NULL REFERENCES permissions(key)
);
INSERT INTO plugin_capabilities SELECT 'uploader',k,'uploads.create' FROM unnest(ARRAY[
 'uploader.use','uploader.large_files','uploader.object_storage','uploader.telegram_storage',
 'uploader.approval_workflow','uploader.custom_workflow','uploader.ffmpeg']) k;
CREATE TABLE infrastructure_safety_caps (
 metric text PRIMARY KEY, limit_value bigint NOT NULL CHECK(limit_value>=0), unit text NOT NULL,
 description text NOT NULL
);
INSERT INTO infrastructure_safety_caps VALUES
 ('uploader.max_file_bytes',5000000000,'BYTES','absolute file size'),
 ('uploader.concurrent_uploads',4,'CONCURRENT_JOBS','absolute workspace upload concurrency'),
 ('uploader.concurrent_processing',2,'CONCURRENT_JOBS','absolute processing concurrency');
CREATE TABLE quota_usage (
 workspace_id uuid NOT NULL REFERENCES workspaces(id), metric text NOT NULL, bucket text NOT NULL,
 used bigint NOT NULL DEFAULT 0 CHECK(used>=0), updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(workspace_id,metric,bucket)
);
CREATE TABLE quota_reservations (
 workspace_id uuid NOT NULL REFERENCES workspaces(id), operation_key text NOT NULL, metric text NOT NULL,
 fingerprint text NOT NULL, amount bigint NOT NULL CHECK(amount>0), bucket text NOT NULL,
 policy_key text NOT NULL, policy_version bigint NOT NULL,
 state text NOT NULL CHECK(state IN('RESERVED','COMMITTED','RELEASED')), concurrent boolean NOT NULL,
 count_failure boolean NOT NULL DEFAULT false,
 expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), settled_at timestamptz,
 FOREIGN KEY(policy_key,policy_version) REFERENCES policy_versions(key,version),
 PRIMARY KEY(workspace_id,operation_key,metric)
);
CREATE INDEX quota_history ON quota_reservations(workspace_id,metric,created_at,settled_at);
CREATE INDEX quota_pending ON quota_reservations(workspace_id,metric,expires_at) WHERE state='RESERVED';
DO $$ DECLARE t text; BEGIN
 FOREACH t IN ARRAY ARRAY['quota_usage','quota_reservations'] LOOP
 EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',t);
 EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',t);
 EXECUTE format('CREATE POLICY tenant_scope ON %I USING(workspace_id::text=current_setting(''app.workspace_id'',true)) WITH CHECK(workspace_id::text=current_setting(''app.workspace_id'',true))',t);
 END LOOP;
END $$;
-- Editable seeds. Resolver code contains none of these commercial amounts/durations.
INSERT INTO policy_versions(key,version,kind,enabled,definition,effective_from) VALUES
 ('global_customer_trial',1,'TRIAL',true,'{"scope":"PER_USER","duration_seconds":604800,"eligibility":"NEW_USER","trigger":"DISCOVERY","once":true,"stacking":"PARALLEL","fallback":"FREE_LIMITED","grant_kind":"GLOBAL_TRIAL","features":["tickets","downloads","channels","groups","bots","broadcasts","wallet","analytics","uploader","team","ai","gates"],"plugins":["support","faq","sales","payments","subscription","affiliate","booking","uploader"],"capabilities":["uploader.use","uploader.telegram_storage"],"limits":{}}',now()),
 ('group_product_trial',1,'TRIAL',true,'{"scope":"PER_GROUP","duration_seconds":604800,"eligibility":"ANY","trigger":"GROUP_CLAIM","once":true,"stacking":"KEEP_FIRST","fallback":"ENTITLEMENT_SUSPENDED","grant_kind":"PRODUCT_TRIAL","features":["groups"],"plugins":[],"capabilities":[],"limits":{},"max_subjects_per_user":3}',now());
INSERT INTO policy_versions(key,version,kind,feature_key,plan_key,enabled,definition,effective_from) VALUES
 ('downloader_free',1,'QUOTA','downloads','FREE',true,'{"metric":"downloader.successes","unit":"JOBS","window":"CALENDAR_DAY","timezone":"Asia/Tehran","limit":5,"mode":"USAGE","reservation_seconds":900,"count_failure":false}',now()),
 ('downloader_pro',1,'QUOTA','downloads','PRO',true,'{"metric":"downloader.successes","unit":"JOBS","window":"CALENDAR_DAY","timezone":"Asia/Tehran","limit":100,"mode":"USAGE","reservation_seconds":900,"count_failure":false}',now()),
 ('downloader_business',1,'QUOTA','downloads','BUSINESS',true,'{"metric":"downloader.successes","unit":"JOBS","window":"CALENDAR_DAY","timezone":"Asia/Tehran","limit":1000,"mode":"USAGE","reservation_seconds":900,"count_failure":false}',now());
UPDATE plan_entitlements SET enabled=true WHERE plan_key='FREE' AND feature_key='downloads';
INSERT INTO policy_versions(key,version,kind,feature_key,plugin_key,plan_key,enabled,definition,effective_from)
SELECT 'uploader_'||p||'_'||m,1,'RESOURCE','uploader','uploader',p,true,
 jsonb_build_object('metric',m,'unit',u,'window',CASE WHEN m IN('uploader.files_per_period','uploader.bytes_uploaded','uploader.processing_seconds') THEN 'CALENDAR_MONTH' ELSE 'NONE' END,'timezone','UTC','limit',n,'mode',mde,'strategy','REPLACE','source','PLAN','reservation_seconds',900,'count_failure',false,'capabilities',CASE WHEN p IN('PRO','BUSINESS') AND m='uploader.max_file_bytes' THEN '["uploader.use","uploader.large_files","uploader.object_storage","uploader.telegram_storage","uploader.approval_workflow","uploader.custom_workflow","uploader.ffmpeg"]'::jsonb ELSE '[]'::jsonb END),now()
FROM (VALUES ('FREE'),('PRO'),('BUSINESS')) plans(p) CROSS JOIN (VALUES
 ('uploader.max_file_bytes','BYTES',2000000000::bigint,'ITEM'),
 ('uploader.storage_bytes','STORAGE_BYTES',20000000000::bigint,'CAPACITY'),
 ('uploader.bytes_uploaded','BYTES',100000000000::bigint,'USAGE'),
 ('uploader.active_files','COUNT',1000::bigint,'CAPACITY'),
 ('uploader.files_per_period','COUNT',1000::bigint,'USAGE'),
 ('uploader.concurrent_uploads','CONCURRENT_JOBS',2::bigint,'CONCURRENT'),
 ('uploader.concurrent_processing','CONCURRENT_JOBS',1::bigint,'CONCURRENT'),
 ('uploader.processing_seconds','SECONDS',3600::bigint,'USAGE'),
 ('uploader.retention_seconds','SECONDS',2592000::bigint,'ITEM')) metrics(m,u,n,mde);
INSERT INTO policy_versions(key,version,kind,feature_key,enabled,definition,effective_from) VALUES
 ('downloader_upgrade_price',1,'PRICE','downloads',true,'{"asset":"USD","amount_atomic":"500","scale":2}',now()),
 ('downloader_upgrade',1,'OFFER','downloads',true,'{"destination":"PRO","price_key":"downloader_upgrade_price","price_version":1,"campaign":"initial_seed"}',now()),
 ('downloader_free_limit',1,'PAYWALL','downloads',true,'{"reasons":["QUOTA_EXCEEDED","FEATURE_DISABLED","TRIAL_EXPIRED"],"offers":["downloader_upgrade"]}',now());
