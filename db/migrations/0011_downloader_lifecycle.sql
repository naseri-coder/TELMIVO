-- Missing durable contracts: bind existing download_jobs to existing leased media_jobs;
-- stable analyzed title/format expiry, cancel intent and
-- acknowledged/unknown delivery outcome cannot be safely encoded in format lists.
-- Existing updated_at is the terminal timestamp; download_assets owns asset bytes.
-- UPLOADING is the durable external-send marker. Outbox owns retry scheduling.
-- No second jobs/quota/queue/identity/storage model. Existing RLS covers new columns.
ALTER TABLE download_jobs ADD COLUMN media_job_id uuid,
 ADD COLUMN title text NOT NULL DEFAULT '',
 ADD COLUMN format_expires_at timestamptz,
 ADD COLUMN cancel_requested boolean NOT NULL DEFAULT false,
 ADD COLUMN delivery_outcome text NOT NULL DEFAULT 'DELIVERY_PENDING' CHECK(delivery_outcome IN('DELIVERY_PENDING','DELIVERY_ACKNOWLEDGED','DELIVERY_FAILED','DELIVERY_OUTCOME_UNKNOWN')),
 ADD COLUMN delivery_message_id bigint,
 ADD CONSTRAINT download_media_binding FOREIGN KEY(media_job_id,workspace_id) REFERENCES media_jobs(id,workspace_id),
 ADD CONSTRAINT download_one_media UNIQUE(media_job_id),
 ADD CONSTRAINT download_delivery_ack CHECK(media_job_id IS NULL OR
 ((state<>'COMPLETED' OR (delivery_outcome='DELIVERY_ACKNOWLEDGED' AND COALESCE(delivery_message_id,0)>0))
 AND (delivery_outcome<>'DELIVERY_ACKNOWLEDGED' OR (state='COMPLETED' AND COALESCE(delivery_message_id,0)>0))));
CREATE FUNCTION guard_downloader_product() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='INSERT' THEN
  IF NEW.media_job_id IS NOT NULL AND (NEW.state<>'QUEUED' OR NEW.format_id<>'' OR NEW.formats<>'[]'::jsonb OR NEW.cancel_requested OR NEW.delivery_outcome<>'DELIVERY_PENDING' OR NEW.delivery_message_id IS NOT NULL) THEN RAISE EXCEPTION 'invalid initial downloader state'; END IF;
  IF NEW.media_job_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM media_jobs m WHERE m.id=NEW.media_job_id AND m.workspace_id=NEW.workspace_id AND m.idempotency_key=NEW.idempotency_key AND m.operation='downloader' AND m.state='QUEUED') THEN RAISE EXCEPTION 'invalid downloader execution binding'; END IF;
  RETURN NEW;
 END IF;
 IF OLD.media_job_id IS NULL THEN
  IF NEW.media_job_id IS NOT NULL THEN RAISE EXCEPTION 'legacy download cannot be rebound'; END IF;
  RETURN NEW;
 END IF;
 IF NEW.workspace_id<>OLD.workspace_id OR NEW.creator_id<>OLD.creator_id OR NEW.source_url<>OLD.source_url OR NEW.provider<>OLD.provider OR NEW.media_job_id IS DISTINCT FROM OLD.media_job_id OR NEW.idempotency_key<>OLD.idempotency_key THEN RAISE EXCEPTION 'immutable download identity'; END IF;
 IF OLD.state IN('COMPLETED','FAILED','CANCELLED') AND NEW IS DISTINCT FROM OLD THEN RAISE EXCEPTION 'immutable download terminal'; END IF;
 IF NEW.state<>OLD.state AND NOT (
 (OLD.state='QUEUED' AND NEW.state IN('ANALYZING','FAILED','CANCELLED')) OR
 (OLD.state='ANALYZING' AND NEW.state IN('AWAITING_FORMAT','FAILED','CANCELLED')) OR
 (OLD.state='AWAITING_FORMAT' AND NEW.state IN('DOWNLOADING','FAILED','CANCELLED')) OR
 (OLD.state='DOWNLOADING' AND NEW.state IN('PROCESSING','UPLOADING','FAILED','CANCELLED')) OR
 (OLD.state='PROCESSING' AND NEW.state IN('UPLOADING','FAILED','CANCELLED')) OR
 (OLD.state='UPLOADING' AND NEW.state IN('COMPLETED','FAILED')) ) THEN RAISE EXCEPTION 'invalid download transition'; END IF;
 IF OLD.cancel_requested AND NOT NEW.cancel_requested THEN RAISE EXCEPTION 'immutable cancellation intent'; END IF;
 IF NEW.cancel_requested AND NOT OLD.cancel_requested AND OLD.state IN('UPLOADING','COMPLETED','FAILED','CANCELLED') THEN RAISE EXCEPTION 'download cannot be cancelled'; END IF;
 IF NEW.format_id<>OLD.format_id AND (OLD.format_id<>'' OR OLD.state<>'AWAITING_FORMAT' OR OLD.cancel_requested OR OLD.format_expires_at IS NULL OR OLD.format_expires_at<=clock_timestamp() OR NOT EXISTS(SELECT 1 FROM jsonb_array_elements(OLD.formats) f WHERE f->>'id'=NEW.format_id)) THEN RAISE EXCEPTION 'invalid download format selection'; END IF;
 IF NEW.state<>OLD.state OR NEW.formats<>OLD.formats OR NEW.title<>OLD.title OR NEW.format_expires_at IS DISTINCT FROM OLD.format_expires_at OR NEW.delivery_outcome<>OLD.delivery_outcome OR NEW.delivery_message_id IS DISTINCT FROM OLD.delivery_message_id THEN
  IF NOT EXISTS(SELECT 1 FROM job_leases j WHERE j.job_id=OLD.media_job_id AND j.workspace_id=OLD.workspace_id AND j.owner=current_setting('app.lease_owner',true) AND j.generation::text=current_setting('app.lease_generation',true) AND j.expires_at>clock_timestamp() AND j.terminal_state IS NULL) THEN RAISE EXCEPTION 'stale downloader fencing'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER downloader_product_guard BEFORE INSERT OR UPDATE ON download_jobs FOR EACH ROW EXECUTE FUNCTION guard_downloader_product();
-- No global SECURITY DEFINER dispatch API: worker reuses durable outbox
-- workspace coordinates and then reads jobs under transaction-local RLS scope.
UPDATE feature_flags SET enabled=true,rollout_status='ON' WHERE feature_key='downloads';
