-- R5C: Apple Identity Verification / Sign in with Apple.
-- Authorization codes, ID/access/refresh tokens and Apple private keys are never persisted.

CREATE TABLE apple_identity_verifications(
 operation_id uuid PRIMARY KEY,
 workspace_id uuid NOT NULL,
 operation_kind text NOT NULL DEFAULT 'IDENTITY_VERIFICATION' CHECK(operation_kind='IDENTITY_VERIFICATION'),
 user_id uuid NOT NULL,
 client_id text NOT NULL CHECK(length(client_id) BETWEEN 1 AND 200),
 state_hash text NOT NULL UNIQUE CHECK(length(state_hash)=64),
 nonce_hash text NOT NULL CHECK(length(nonce_hash)=64),
 code_hash text CHECK(code_hash IS NULL OR length(code_hash)=64),
 exchange_state text NOT NULL DEFAULT 'INITIATED' CHECK(exchange_state IN('INITIATED','EXCHANGING','VERIFIED','REJECTED','UNKNOWN')),
 attempt_id uuid,
 expires_at timestamptz NOT NULL,
 completed_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK(expires_at>created_at),
 CHECK(updated_at>=created_at),
 CHECK((exchange_state='INITIATED')=(attempt_id IS NULL)),
 CHECK((exchange_state='INITIATED')=(code_hash IS NULL)),
 CHECK((exchange_state IN('VERIFIED','REJECTED','UNKNOWN'))=(completed_at IS NOT NULL)),
 UNIQUE(operation_id,workspace_id),
 FOREIGN KEY(operation_id,workspace_id,operation_kind)
   REFERENCES apple_service_operations(id,workspace_id,kind),
 FOREIGN KEY(workspace_id,user_id)
   REFERENCES workspace_members(workspace_id,user_id)
);
CREATE INDEX apple_identity_verifications_user
 ON apple_identity_verifications(workspace_id,user_id,created_at DESC);

CREATE TABLE apple_identity_links(
 client_id text NOT NULL CHECK(length(client_id) BETWEEN 1 AND 200),
 subject_hash text NOT NULL CHECK(length(subject_hash)=64),
 user_id uuid NOT NULL REFERENCES users(id),
 email_hash text CHECK(email_hash IS NULL OR length(email_hash)=64),
 email_verified boolean NOT NULL DEFAULT false,
 private_email boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now(),
 last_verified_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(client_id,user_id),
 UNIQUE(client_id,subject_hash),
 CHECK(last_verified_at>=created_at)
);

CREATE FUNCTION guard_apple_identity_verification() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' THEN
  RAISE EXCEPTION 'apple identity verification history cannot be deleted' USING ERRCODE='23514';
 END IF;
 IF NEW.operation_id<>OLD.operation_id OR NEW.workspace_id<>OLD.workspace_id
 OR NEW.operation_kind<>OLD.operation_kind OR NEW.user_id<>OLD.user_id
 OR NEW.client_id<>OLD.client_id OR NEW.state_hash<>OLD.state_hash
 OR NEW.nonce_hash<>OLD.nonce_hash OR NEW.expires_at<>OLD.expires_at
 OR NEW.created_at<>OLD.created_at THEN
  RAISE EXCEPTION 'apple identity verification identity is immutable' USING ERRCODE='23514';
 END IF;
 IF NEW.updated_at<OLD.updated_at THEN
  RAISE EXCEPTION 'apple identity verification time cannot move backwards' USING ERRCODE='23514';
 END IF;
 IF OLD.exchange_state IN('VERIFIED','REJECTED','UNKNOWN') AND NEW.exchange_state<>OLD.exchange_state THEN
  RAISE EXCEPTION 'apple identity verification terminal state is immutable' USING ERRCODE='23514';
 END IF;
 IF OLD.exchange_state='INITIATED' AND NEW.exchange_state NOT IN('EXCHANGING','REJECTED') THEN
  RAISE EXCEPTION 'invalid apple identity exchange transition' USING ERRCODE='23514';
 END IF;
 IF OLD.exchange_state='EXCHANGING' AND NEW.exchange_state NOT IN('VERIFIED','REJECTED','UNKNOWN') THEN
  RAISE EXCEPTION 'invalid apple identity exchange completion' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER apple_identity_verification_guard
 BEFORE UPDATE OR DELETE ON apple_identity_verifications
 FOR EACH ROW EXECUTE FUNCTION guard_apple_identity_verification();

CREATE FUNCTION guard_apple_identity_link() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' THEN
  RAISE EXCEPTION 'apple identity links require future explicit unlink policy' USING ERRCODE='23514';
 END IF;
 IF NEW.client_id<>OLD.client_id OR NEW.subject_hash<>OLD.subject_hash
 OR NEW.user_id<>OLD.user_id OR NEW.created_at<>OLD.created_at THEN
  RAISE EXCEPTION 'apple identity link identity is immutable' USING ERRCODE='23514';
 END IF;
 IF NEW.last_verified_at<OLD.last_verified_at THEN
  RAISE EXCEPTION 'apple identity verification time cannot move backwards' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER apple_identity_link_guard
 BEFORE UPDATE OR DELETE ON apple_identity_links
 FOR EACH ROW EXECUTE FUNCTION guard_apple_identity_link();

ALTER TABLE apple_identity_verifications ENABLE ROW LEVEL SECURITY;
ALTER TABLE apple_identity_verifications FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope ON apple_identity_verifications
 USING(workspace_id::text=current_setting('app.workspace_id',true))
 WITH CHECK(workspace_id::text=current_setting('app.workspace_id',true));

ALTER TABLE apple_identity_links ENABLE ROW LEVEL SECURITY;
ALTER TABLE apple_identity_links FORCE ROW LEVEL SECURITY;
CREATE POLICY user_scope ON apple_identity_links
 USING(user_id::text=current_setting('app.user_id',true))
 WITH CHECK(user_id::text=current_setting('app.user_id',true));
