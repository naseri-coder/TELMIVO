-- R4C: complete the bounded Uploader lifecycle on the existing R1 storage foundation.

ALTER TABLE uploads DROP CONSTRAINT uploads_state_check;
ALTER TABLE uploads ADD CONSTRAINT uploads_state_check
  CHECK(state IN('RECEIVED','VALIDATING','PENDING_APPROVAL','PROCESSING','STORED','REJECTED','FAILED','EXPIRED'));

ALTER TABLE uploads
  ADD COLUMN workflow_id uuid,
  ADD COLUMN operation_key text,
  ADD COLUMN fingerprint text,
  ADD COLUMN original_name text,
  ADD COLUMN content_type text,
  ADD COLUMN size_bytes bigint CHECK(size_bytes IS NULL OR size_bytes>0),
  ADD COLUMN storage_key text,
  ADD COLUMN sha256 text,
  ADD COLUMN expires_at timestamptz,
  ADD COLUMN error_code text,
  ADD COLUMN revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
  ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE uploads
  ADD CONSTRAINT uploads_workflow_workspace_fkey
  FOREIGN KEY(workflow_id,workspace_id) REFERENCES upload_workflows(id,workspace_id);

CREATE UNIQUE INDEX uploads_operation_identity
  ON uploads(workspace_id,operation_key) WHERE operation_key IS NOT NULL;
CREATE UNIQUE INDEX uploads_storage_identity
  ON uploads(workspace_id,storage_key) WHERE storage_key IS NOT NULL;
CREATE INDEX uploads_lifecycle
  ON uploads(workspace_id,state,expires_at,created_at DESC);

ALTER TABLE upload_files
  ADD COLUMN sha256 text,
  ADD COLUMN expires_at timestamptz,
  ADD COLUMN created_at timestamptz NOT NULL DEFAULT now();
CREATE UNIQUE INDEX upload_files_one_object ON upload_files(upload_id);

ALTER TABLE upload_workflows
  ADD COLUMN creator_id uuid REFERENCES users(id),
  ADD COLUMN enabled boolean NOT NULL DEFAULT true,
  ADD COLUMN operation_key text,
  ADD COLUMN fingerprint text,
  ADD COLUMN revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
  ADD COLUMN created_at timestamptz NOT NULL DEFAULT now(),
  ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();
CREATE UNIQUE INDEX upload_workflows_operation_identity
  ON upload_workflows(workspace_id,operation_key) WHERE operation_key IS NOT NULL;

CREATE OR REPLACE FUNCTION guard_upload_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.workspace_id<>OLD.workspace_id OR NEW.creator_id<>OLD.creator_id OR
     NEW.workflow_id IS DISTINCT FROM OLD.workflow_id OR NEW.operation_key IS DISTINCT FROM OLD.operation_key OR
     NEW.fingerprint IS DISTINCT FROM OLD.fingerprint OR NEW.original_name IS DISTINCT FROM OLD.original_name OR
     NEW.content_type IS DISTINCT FROM OLD.content_type OR NEW.size_bytes IS DISTINCT FROM OLD.size_bytes OR
     NEW.storage_key IS DISTINCT FROM OLD.storage_key THEN
    RAISE EXCEPTION 'upload identity is immutable';
  END IF;
  IF NEW.revision<>OLD.revision+1 THEN
    RAISE EXCEPTION 'upload revision must advance exactly once';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER upload_identity_guard
  BEFORE UPDATE ON uploads FOR EACH ROW EXECUTE FUNCTION guard_upload_identity();

CREATE OR REPLACE FUNCTION guard_upload_workflow_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.workspace_id<>OLD.workspace_id OR NEW.creator_id IS DISTINCT FROM OLD.creator_id OR
     NEW.operation_key IS DISTINCT FROM OLD.operation_key OR NEW.fingerprint IS DISTINCT FROM OLD.fingerprint THEN
    RAISE EXCEPTION 'upload workflow identity is immutable';
  END IF;
  IF NEW.revision<>OLD.revision+1 THEN
    RAISE EXCEPTION 'upload workflow revision must advance exactly once';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER upload_workflow_identity_guard
  BEFORE UPDATE ON upload_workflows FOR EACH ROW EXECUTE FUNCTION guard_upload_workflow_identity();

UPDATE feature_flags SET enabled=true,rollout_status='ON' WHERE feature_key='uploader';
UPDATE plugins SET status='ENABLED',version='0.2.0' WHERE key='uploader';
UPDATE plan_plugin_entitlements SET enabled=true
 WHERE plugin_key='uploader' AND plan_key IN('PRO','BUSINESS');
INSERT INTO role_permissions(role_key,permission_key) VALUES
 ('CONTENT_MANAGER','uploads.read'),('CONTENT_MANAGER','uploads.create')
ON CONFLICT DO NOTHING;
