-- R5B: Consumer Apple Account Assistance.
-- This stage stores only TELMIVO workflow metadata. Passwords, OTPs, verification codes,
-- Apple tokens and account credentials remain exclusively inside Apple-controlled flows.

DO $
BEGIN
 IF EXISTS (SELECT 1 FROM apple_service_operations WHERE kind='CONSUMER_ACCOUNT') THEN
  RAISE EXCEPTION 'R5B requires zero legacy generic CONSUMER_ACCOUNT operations before migration';
 END IF;
END $;

CREATE UNIQUE INDEX apple_service_operation_kind_identity
 ON apple_service_operations(id,workspace_id,kind);

CREATE TABLE apple_consumer_assistance(
 operation_id uuid PRIMARY KEY,
 workspace_id uuid NOT NULL,
 operation_kind text NOT NULL DEFAULT 'CONSUMER_ACCOUNT' CHECK(operation_kind='CONSUMER_ACCOUNT'),
 goal text NOT NULL CHECK(goal IN('CREATE_ACCOUNT','RECOVER_ACCESS','CHECK_EXISTING')),
 channel text NOT NULL CHECK(channel IN('WEB','IPHONE_IPAD','MAC','ANDROID_WINDOWS','OTHER')),
 locale text NOT NULL DEFAULT 'en' CHECK(length(locale) BETWEEN 2 AND 35),
 region text NOT NULL DEFAULT '' CHECK(region='' OR region ~ '^[A-Z]{2}$'),
 guidance_version text NOT NULL DEFAULT 'APPLE_CONSUMER_V1' CHECK(guidance_version='APPLE_CONSUMER_V1'),
 completion_source text NOT NULL DEFAULT 'NONE' CHECK(completion_source IN('NONE','USER_ATTESTED')),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK(updated_at>=created_at),
 UNIQUE(operation_id,workspace_id),
 FOREIGN KEY(operation_id,workspace_id,operation_kind)
   REFERENCES apple_service_operations(id,workspace_id,kind)
);
CREATE INDEX apple_consumer_assistance_workspace
 ON apple_consumer_assistance(workspace_id,created_at DESC);

CREATE FUNCTION guard_apple_consumer_assistance() RETURNS trigger LANGUAGE plpgsql AS $ BEGIN
 IF TG_OP='DELETE' THEN
  RAISE EXCEPTION 'consumer assistance history cannot be deleted' USING ERRCODE='23514';
 END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.completion_source<>'NONE' THEN
   RAISE EXCEPTION 'consumer assistance must begin without completion attestation' USING ERRCODE='23514';
  END IF;
  PERFORM 1 FROM apple_service_operations
   WHERE id=NEW.operation_id AND workspace_id=NEW.workspace_id
     AND kind='CONSUMER_ACCOUNT' AND state='REQUESTED';
  IF NOT FOUND THEN
   RAISE EXCEPTION 'consumer assistance requires REQUESTED consumer operation' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
 END IF;
 IF NEW.operation_id<>OLD.operation_id OR NEW.workspace_id<>OLD.workspace_id
 OR NEW.operation_kind<>OLD.operation_kind OR NEW.goal<>OLD.goal OR NEW.channel<>OLD.channel
 OR NEW.locale<>OLD.locale OR NEW.region<>OLD.region
 OR NEW.guidance_version<>OLD.guidance_version OR NEW.created_at<>OLD.created_at THEN
  RAISE EXCEPTION 'consumer assistance identity is immutable' USING ERRCODE='23514';
 END IF;
 IF NEW.updated_at<OLD.updated_at THEN
  RAISE EXCEPTION 'consumer assistance updated_at cannot move backwards' USING ERRCODE='23514';
 END IF;
 IF OLD.completion_source='USER_ATTESTED' AND NEW.completion_source<>'USER_ATTESTED' THEN
  RAISE EXCEPTION 'consumer completion attestation is irreversible' USING ERRCODE='23514';
 END IF;
 IF OLD.completion_source='NONE' AND NEW.completion_source='USER_ATTESTED' THEN
  PERFORM 1 FROM apple_service_operations
   WHERE id=NEW.operation_id AND workspace_id=NEW.workspace_id AND state='READY';
  IF NOT FOUND THEN
   RAISE EXCEPTION 'consumer completion attestation requires READY operation' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NEW;
END $;

CREATE TRIGGER apple_consumer_assistance_guard
 BEFORE INSERT OR UPDATE OR DELETE ON apple_consumer_assistance
 FOR EACH ROW EXECUTE FUNCTION guard_apple_consumer_assistance();

ALTER TABLE apple_consumer_assistance ENABLE ROW LEVEL SECURITY;
ALTER TABLE apple_consumer_assistance FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope ON apple_consumer_assistance
 USING(workspace_id::text=current_setting('app.workspace_id',true))
 WITH CHECK(workspace_id::text=current_setting('app.workspace_id',true));
