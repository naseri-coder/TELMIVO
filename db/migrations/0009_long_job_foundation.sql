-- Extend existing media_jobs; PostgreSQL remains the ownership authority.
CREATE TABLE job_leases (
 workspace_id uuid NOT NULL, job_id uuid NOT NULL, owner text NOT NULL,
 generation bigint NOT NULL CHECK(generation>0), acquired_at timestamptz NOT NULL,
 expires_at timestamptz NOT NULL, deadline timestamptz NOT NULL,
 reservation_key text NOT NULL, terminal_state text,
 CHECK(length(owner) BETWEEN 1 AND 128), CHECK(expires_at<=deadline),
 CHECK(terminal_state IS NULL OR terminal_state IN('COMPLETED','FAILED','CANCELLED')),
 PRIMARY KEY(job_id), FOREIGN KEY(job_id,workspace_id) REFERENCES media_jobs(id,workspace_id)
);
CREATE TABLE quota_lease_allocations (
 workspace_id uuid NOT NULL, operation_key text NOT NULL, metric text NOT NULL,
 job_id uuid NOT NULL, expires_at timestamptz NOT NULL, window_end timestamptz,
 PRIMARY KEY(workspace_id,operation_key,metric),
 FOREIGN KEY(workspace_id,operation_key,metric) REFERENCES quota_reservations(workspace_id,operation_key,metric),
 FOREIGN KEY(job_id,workspace_id) REFERENCES media_jobs(id,workspace_id),
 CHECK(window_end IS NULL OR expires_at<=window_end)
);
DO $$ DECLARE t text; BEGIN
 FOREACH t IN ARRAY ARRAY['job_leases','quota_lease_allocations'] LOOP
 EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',t);
 EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',t);
 EXECUTE format('CREATE POLICY tenant_scope ON %I USING(workspace_id::text=current_setting(''app.workspace_id'',true)) WITH CHECK(workspace_id::text=current_setting(''app.workspace_id'',true))',t);
 END LOOP;
END $$;
CREATE FUNCTION guard_job_lease() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' OR OLD.terminal_state IS NOT NULL OR NEW.job_id<>OLD.job_id OR NEW.workspace_id<>OLD.workspace_id OR NEW.deadline<>OLD.deadline OR NEW.reservation_key<>OLD.reservation_key OR NEW.generation<OLD.generation
 THEN RAISE EXCEPTION 'immutable job ownership history'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER job_lease_history BEFORE UPDATE OR DELETE ON job_leases FOR EACH ROW EXECUTE FUNCTION guard_job_lease();
CREATE FUNCTION guard_media_terminal() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF OLD.state IN('COMPLETED','FAILED','CANCELLED') AND NEW IS DISTINCT FROM OLD THEN RAISE EXCEPTION 'immutable job terminal result'; END IF;
 IF NEW.state IN('COMPLETED','FAILED','CANCELLED') AND NEW.state<>OLD.state AND EXISTS(SELECT 1 FROM job_leases WHERE job_id=NEW.id) THEN
  IF NOT EXISTS(SELECT 1 FROM job_leases WHERE job_id=NEW.id AND owner=current_setting('app.lease_owner',true) AND generation::text=current_setting('app.lease_generation',true) AND expires_at>clock_timestamp() AND terminal_state IS NULL) THEN RAISE EXCEPTION 'stale job fencing'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER media_terminal_fencing BEFORE UPDATE ON media_jobs FOR EACH ROW EXECUTE FUNCTION guard_media_terminal();
CREATE FUNCTION guard_quota_lease() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'durable allocation history'; END IF;
 IF TG_OP='UPDATE' AND (NEW.workspace_id<>OLD.workspace_id OR NEW.operation_key<>OLD.operation_key OR NEW.metric<>OLD.metric OR NEW.job_id<>OLD.job_id OR NEW.window_end IS DISTINCT FROM OLD.window_end OR NEW.expires_at<OLD.expires_at) THEN RAISE EXCEPTION 'immutable allocation identity'; END IF;
 IF NOT EXISTS(SELECT 1 FROM job_leases j WHERE j.job_id=NEW.job_id AND j.owner=current_setting('app.lease_owner',true) AND j.generation::text=current_setting('app.lease_generation',true) AND j.expires_at>clock_timestamp() AND j.terminal_state IS NULL AND NEW.expires_at<=j.deadline) THEN RAISE EXCEPTION 'stale allocation fencing'; END IF;
 IF NOT EXISTS(SELECT 1 FROM quota_reservations WHERE workspace_id=NEW.workspace_id AND operation_key=NEW.operation_key AND metric=NEW.metric AND state='RESERVED') THEN RAISE EXCEPTION 'terminal allocation'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER quota_lease_fencing BEFORE INSERT OR UPDATE OR DELETE ON quota_lease_allocations FOR EACH ROW EXECUTE FUNCTION guard_quota_lease();
-- Recovery emits one database-derived wakeup per abandoned generation. Events
-- remain in the existing outbox, even if JetStream transport state was lost.
CREATE TABLE job_recovery_events (
 workspace_id uuid NOT NULL, job_id uuid NOT NULL, generation bigint NOT NULL CHECK(generation>=0),
 event_id uuid NOT NULL REFERENCES outbox_events(id), created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(job_id,generation), FOREIGN KEY(job_id,workspace_id) REFERENCES media_jobs(id,workspace_id)
);
ALTER TABLE job_recovery_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE job_recovery_events FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope ON job_recovery_events USING(workspace_id::text=current_setting('app.workspace_id',true)) WITH CHECK(workspace_id::text=current_setting('app.workspace_id',true));
CREATE TRIGGER job_recovery_immutable BEFORE UPDATE OR DELETE ON job_recovery_events FOR EACH ROW EXECUTE FUNCTION reject_mutation();
