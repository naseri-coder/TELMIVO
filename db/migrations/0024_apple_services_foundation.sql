-- R5A: Apple Services Foundation.
-- No Apple/IdP credentials, passwords, OTPs, tokens or external provider effects are introduced here.

INSERT INTO permissions(key) VALUES
 ('apple.services.read'),
 ('apple.services.manage'),
 ('apple.consumer.setup'),
 ('apple.verify'),
 ('apple.business.connect'),
 ('apple.provision'),
 ('apple.users'),
 ('apple.reconcile'),
 ('apple.audit.read');

INSERT INTO role_permissions(role_key,permission_key)
SELECT r,p
FROM (VALUES('OWNER'),('ADMIN')) roles(r)
CROSS JOIN (VALUES
 ('apple.services.read'),
 ('apple.services.manage'),
 ('apple.consumer.setup'),
 ('apple.verify'),
 ('apple.business.connect'),
 ('apple.provision'),
 ('apple.users'),
 ('apple.reconcile'),
 ('apple.audit.read')
) permissions(p);

INSERT INTO features(key,name_key,permission_key,limit_kind,limit_unit,reset_window)
VALUES('apple_services','features.apple_services','apple.services.read','USAGE','operation','MONTH');

INSERT INTO feature_flags(feature_key,enabled,rollout_status)
VALUES('apple_services',true,'BETA');

INSERT INTO plan_entitlements(plan_key,feature_key,enabled,limit_value)
SELECT key,'apple_services',false,NULL FROM plans;

INSERT INTO plugins(key,name_key,version,status,required_feature,configuration_schema,permissions)
VALUES(
 'apple-services',
 'plugins.apple_services',
 '0.3.0',
 'BETA',
 'apple_services',
 '{"type":"object","additionalProperties":false}'::jsonb,
 '["apple.services.read","apple.services.manage","apple.consumer.setup","apple.verify","apple.business.connect","apple.provision","apple.users","apple.reconcile","apple.audit.read"]'::jsonb
);

INSERT INTO plan_plugin_entitlements(plan_key,plugin_key,enabled)
SELECT key,'apple-services',false FROM plans;

INSERT INTO plugin_capabilities(plugin_key,key,permission_key) VALUES
 ('apple-services','apple.consumer.setup','apple.consumer.setup'),
 ('apple-services','apple.verify','apple.verify'),
 ('apple-services','apple.business.connect','apple.business.connect'),
 ('apple-services','apple.provision','apple.provision'),
 ('apple-services','apple.users','apple.users'),
 ('apple-services','apple.reconcile','apple.reconcile');

CREATE FUNCTION apple_operation_state_valid(k text, s text) RETURNS boolean
LANGUAGE sql IMMUTABLE AS $$
 SELECT CASE k
  WHEN 'CONSUMER_ACCOUNT' THEN s IN('REQUESTED','PRECHECK','APPLE_ACTION_REQUIRED','APPLE_COMPLETED','VERIFICATION_PENDING','READY','FAILED','CANCELLED','EXPIRED')
  WHEN 'IDENTITY_VERIFICATION' THEN s IN('REQUESTED','PRECHECK','APPLE_ACTION_REQUIRED','VERIFICATION_PENDING','READY','FAILED','CANCELLED','EXPIRED')
  WHEN 'BUSINESS_CONNECTION' THEN s IN('REQUESTED','PRECHECK','APPLE_ACTION_REQUIRED','VERIFICATION_PENDING','READY','RECONCILE_REQUIRED','SUSPENDED','FAILED','CANCELLED','EXPIRED')
  WHEN 'DIRECTORY_PROVISIONING' THEN s IN('REQUESTED','PRECHECK','APPLE_ACTION_REQUIRED','SYNC_PENDING','ACTIVE','RECONCILE_REQUIRED','SUSPENDED','FAILED','CANCELLED','EXPIRED')
  WHEN 'MANAGED_ACCOUNT' THEN s IN('REQUESTED','PRECHECK','SYNC_PENDING','ACTIVE','RECONCILE_REQUIRED','SUSPENDED','FAILED','CANCELLED','EXPIRED')
  WHEN 'RECONCILIATION' THEN s IN('REQUESTED','PRECHECK','RECONCILE_REQUIRED','READY','FAILED','CANCELLED','EXPIRED')
  ELSE false
 END
$$;

CREATE TABLE apple_service_profiles(
 workspace_id uuid PRIMARY KEY REFERENCES workspaces(id),
 state text NOT NULL DEFAULT 'FOUNDATION_READY' CHECK(state IN('FOUNDATION_READY','SUSPENDED')),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 created_by uuid NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(workspace_id,created_by) REFERENCES workspace_members(workspace_id,user_id)
);

CREATE TABLE apple_service_operations(
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 workspace_id uuid NOT NULL REFERENCES workspaces(id),
 kind text NOT NULL CHECK(kind IN('CONSUMER_ACCOUNT','IDENTITY_VERIFICATION','BUSINESS_CONNECTION','DIRECTORY_PROVISIONING','MANAGED_ACCOUNT','RECONCILIATION')),
 state text NOT NULL DEFAULT 'REQUESTED',
 subject_ref text NOT NULL DEFAULT '' CHECK(length(subject_ref)<=200),
 operation_key text NOT NULL CHECK(length(operation_key) BETWEEN 1 AND 100 AND operation_key=btrim(operation_key)),
 fingerprint text NOT NULL CHECK(length(fingerprint)=64),
 actor_id uuid NOT NULL,
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 expires_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(workspace_id,operation_key),
 UNIQUE(id,workspace_id),
 FOREIGN KEY(workspace_id,actor_id) REFERENCES workspace_members(workspace_id,user_id),
 CHECK(apple_operation_state_valid(kind,state)),
 CHECK(expires_at IS NULL OR expires_at>created_at)
);
CREATE INDEX apple_service_operations_state ON apple_service_operations(workspace_id,kind,state,created_at DESC);

CREATE TABLE apple_service_operation_events(
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 workspace_id uuid NOT NULL,
 operation_id uuid NOT NULL,
 sequence bigint NOT NULL CHECK(sequence>0),
 actor_id uuid NOT NULL,
 from_state text NOT NULL CHECK(length(from_state)<=40),
 to_state text NOT NULL CHECK(length(to_state) BETWEEN 1 AND 40),
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 500),
 correlation_id text NOT NULL CHECK(length(correlation_id) BETWEEN 1 AND 150),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(operation_id,sequence),
 FOREIGN KEY(operation_id,workspace_id) REFERENCES apple_service_operations(id,workspace_id),
 FOREIGN KEY(workspace_id,actor_id) REFERENCES workspace_members(workspace_id,user_id)
);
CREATE INDEX apple_service_operation_events_history ON apple_service_operation_events(workspace_id,operation_id,sequence);

CREATE FUNCTION guard_apple_service_profile() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' THEN
  RAISE EXCEPTION 'apple service profile retention requires explicit future policy' USING ERRCODE='23514';
 END IF;
 IF NEW.workspace_id<>OLD.workspace_id OR NEW.created_by<>OLD.created_by OR NEW.revision<>OLD.revision+1 THEN
  RAISE EXCEPTION 'apple service profile identity/revision invalid' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER apple_service_profile_guard
 BEFORE UPDATE OR DELETE ON apple_service_profiles
 FOR EACH ROW EXECUTE FUNCTION guard_apple_service_profile();

CREATE FUNCTION guard_apple_service_operation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' THEN
  RAISE EXCEPTION 'apple service operation history cannot be deleted' USING ERRCODE='23514';
 END IF;
 IF NEW.workspace_id<>OLD.workspace_id OR NEW.id<>OLD.id OR NEW.kind<>OLD.kind
 OR NEW.subject_ref<>OLD.subject_ref OR NEW.operation_key<>OLD.operation_key
 OR NEW.fingerprint<>OLD.fingerprint OR NEW.actor_id<>OLD.actor_id
 OR NEW.expires_at IS DISTINCT FROM OLD.expires_at
 OR NEW.revision<>OLD.revision+1 THEN
  RAISE EXCEPTION 'apple service operation identity/revision invalid' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER apple_service_operation_guard
 BEFORE UPDATE OR DELETE ON apple_service_operations
 FOR EACH ROW EXECUTE FUNCTION guard_apple_service_operation();

CREATE TRIGGER apple_service_operation_events_immutable
 BEFORE UPDATE OR DELETE ON apple_service_operation_events
 FOR EACH ROW EXECUTE FUNCTION reject_mutation();

DO $$ DECLARE t text; BEGIN
 FOREACH t IN ARRAY ARRAY['apple_service_profiles','apple_service_operations','apple_service_operation_events'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',t);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',t);
  EXECUTE format('CREATE POLICY tenant_scope ON %I USING(workspace_id::text=current_setting(''app.workspace_id'',true)) WITH CHECK(workspace_id::text=current_setting(''app.workspace_id'',true))',t);
 END LOOP;
END $$;
