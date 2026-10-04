-- Plugin-owned records: scoped to the existing bot, retained across lifecycle changes.
CREATE TABLE content_access_policies (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), workspace_id uuid NOT NULL, bot_id uuid NOT NULL,
 definition jsonb NOT NULL CHECK(jsonb_typeof(definition)='object'),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0), operation_key text NOT NULL, fingerprint text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(id,workspace_id,bot_id), UNIQUE(bot_id,operation_key),
 FOREIGN KEY(bot_id,workspace_id) REFERENCES customer_bots(id,workspace_id)
);
CREATE TABLE content_files (
 reference text PRIMARY KEY CHECK(reference ~ '^[A-Za-z0-9_-]{32}$'),workspace_id uuid NOT NULL,bot_id uuid NOT NULL,
 telegram_file_id text NOT NULL,telegram_file_unique_id text NOT NULL,type text NOT NULL CHECK(type IN('document','video','audio','photo')),
 definition jsonb NOT NULL CHECK(jsonb_typeof(definition)='object'),access_policy_id uuid,
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),operation_key text NOT NULL,fingerprint text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(reference,workspace_id,bot_id),UNIQUE(bot_id,operation_key),UNIQUE(bot_id,telegram_file_unique_id),
 FOREIGN KEY(bot_id,workspace_id) REFERENCES customer_bots(id,workspace_id),
 FOREIGN KEY(access_policy_id,workspace_id,bot_id) REFERENCES content_access_policies(id,workspace_id,bot_id)
);
CREATE TABLE content_delivery_events (
 workspace_id uuid NOT NULL,bot_id uuid NOT NULL,request_key text NOT NULL,file_reference text NOT NULL,
 requester_digest text NOT NULL,fingerprint text NOT NULL,state text NOT NULL CHECK(state IN('SENDING','SENT','DENIED','FAILED','UNKNOWN')),
 result jsonb NOT NULL DEFAULT '{}',created_at timestamptz NOT NULL DEFAULT now(),completed_at timestamptz,
 PRIMARY KEY(bot_id,request_key),
 FOREIGN KEY(file_reference,workspace_id,bot_id) REFERENCES content_files(reference,workspace_id,bot_id)
);
CREATE INDEX content_files_list ON content_files(workspace_id,bot_id,created_at);
CREATE FUNCTION guard_content_record() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'content retention requires explicit policy'; END IF;
 IF (to_jsonb(NEW)-ARRAY['definition','access_policy_id','revision','updated_at']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['definition','access_policy_id','revision','updated_at']) OR NEW.revision<>OLD.revision+1 THEN RAISE EXCEPTION 'immutable content identity or stale revision'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER content_policy_guard BEFORE UPDATE OR DELETE ON content_access_policies FOR EACH ROW EXECUTE FUNCTION guard_content_record();
CREATE TRIGGER content_file_guard BEFORE UPDATE OR DELETE ON content_files FOR EACH ROW EXECUTE FUNCTION guard_content_record();
CREATE FUNCTION guard_content_event() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' OR OLD.state<>'SENDING' OR NEW.state='SENDING' OR (to_jsonb(NEW)-ARRAY['state','result','completed_at']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['state','result','completed_at']) THEN RAISE EXCEPTION 'delivery event is immutable or terminal'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER content_event_guard BEFORE UPDATE OR DELETE ON content_delivery_events FOR EACH ROW EXECUTE FUNCTION guard_content_event();
ALTER TABLE content_access_policies ENABLE ROW LEVEL SECURITY;
ALTER TABLE content_access_policies FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope ON content_access_policies USING(workspace_id::text=current_setting('app.workspace_id',true)) WITH CHECK(workspace_id::text=current_setting('app.workspace_id',true));
ALTER TABLE content_files ENABLE ROW LEVEL SECURITY;
ALTER TABLE content_files FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope ON content_files USING(workspace_id::text=current_setting('app.workspace_id',true)) WITH CHECK(workspace_id::text=current_setting('app.workspace_id',true));
ALTER TABLE content_delivery_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE content_delivery_events FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope ON content_delivery_events USING(workspace_id::text=current_setting('app.workspace_id',true)) WITH CHECK(workspace_id::text=current_setting('app.workspace_id',true));
-- Administrative placeholders, no invented launch price/inclusion/trial or entitlement.
INSERT INTO commercial_catalog(kind,key,version,enabled,effective_at,definition,reason)
 SELECT 'PLUGIN',key,1,true,'2020-01-01T00:00:00Z',jsonb_build_object('plugin',jsonb_build_object('capabilities',caps,'sale_enabled',false,'scope','PER_BOT','prices',jsonb_build_object('daily',jsonb_build_object('unit','USD','scale',2,'amount','0'),'monthly',jsonb_build_object('unit','USD','scale',2,'amount','0')),'trial',jsonb_build_object('enabled',false,'duration_seconds',0),'grace_seconds',0)), 'R3D-4 inactive commercial placeholder; configure before sale'
 FROM (VALUES('access-gate','["telegram.read_member_status"]'::jsonb),('file-manager','["files.read","files.write","telegram.send"]'::jsonb)) p(key,caps);
INSERT INTO commercial_catalog(kind,key,version,enabled,effective_at,definition,reason)
 SELECT 'QUOTA',key,1,true,'2020-01-01T00:00:00Z',jsonb_build_object('quota',jsonb_build_object('scope','PER_BOT','reset',period,'default',jsonb_build_object('value','0','unlimited',true))), 'R3D-4 configurable quota; unlimited initial policy'
 FROM (VALUES('files.count','NONE'),('downloads.monthly','MONTHLY'),('membership_checks.monthly','MONTHLY')) q(key,period);
