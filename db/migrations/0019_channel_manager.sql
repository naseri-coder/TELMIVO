-- R4B: channel manager and safe scheduled publishing.
CREATE UNIQUE INDEX channels_telegram_global ON channels(telegram_chat_id);

CREATE TABLE channel_posts(
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 workspace_id uuid NOT NULL,
 channel_id uuid NOT NULL,
 actor_id uuid NOT NULL,
 body text NOT NULL CHECK(length(body) BETWEEN 1 AND 4096),
 state text NOT NULL DEFAULT 'DRAFT' CHECK(state IN('DRAFT','SCHEDULED','SENDING','SENT','FAILED','UNKNOWN','CANCELLED')),
 scheduled_at timestamptz,
 attempted_at timestamptz,
 delivered_at timestamptz,
 telegram_message_id bigint,
 outcome_code text NOT NULL DEFAULT '',
 operation_key text NOT NULL CHECK(length(operation_key) BETWEEN 1 AND 100),
 fingerprint text NOT NULL CHECK(length(fingerprint)=64),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(channel_id,operation_key),
 UNIQUE(id,workspace_id,channel_id),
 FOREIGN KEY(channel_id,workspace_id) REFERENCES channels(id,workspace_id),
 FOREIGN KEY(workspace_id,actor_id) REFERENCES workspace_members(workspace_id,user_id),
 CHECK((state='DRAFT' AND scheduled_at IS NULL AND attempted_at IS NULL AND delivered_at IS NULL AND telegram_message_id IS NULL)
    OR (state='SCHEDULED' AND scheduled_at IS NOT NULL AND attempted_at IS NULL AND delivered_at IS NULL AND telegram_message_id IS NULL)
    OR (state='SENDING' AND scheduled_at IS NOT NULL AND attempted_at IS NOT NULL AND delivered_at IS NULL AND telegram_message_id IS NULL)
    OR (state IN('SENT','FAILED','UNKNOWN') AND scheduled_at IS NOT NULL AND attempted_at IS NOT NULL AND delivered_at IS NOT NULL)
    OR (state='CANCELLED' AND attempted_at IS NULL AND delivered_at IS NULL AND telegram_message_id IS NULL))
);
CREATE INDEX channel_posts_due ON channel_posts(workspace_id,state,scheduled_at,attempted_at) WHERE state IN('SCHEDULED','SENDING');

CREATE TABLE channel_post_history(
 workspace_id uuid NOT NULL,
 channel_id uuid NOT NULL,
 post_id uuid NOT NULL,
 revision bigint NOT NULL CHECK(revision>0),
 state text NOT NULL,
 outcome_code text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(post_id,revision),
 FOREIGN KEY(post_id,workspace_id,channel_id) REFERENCES channel_posts(id,workspace_id,channel_id)
);

ALTER TABLE channel_posts ENABLE ROW LEVEL SECURITY;
ALTER TABLE channel_posts FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope ON channel_posts USING(workspace_id::text=current_setting('app.workspace_id',true)) WITH CHECK(workspace_id::text=current_setting('app.workspace_id',true));
ALTER TABLE channel_post_history ENABLE ROW LEVEL SECURITY;
ALTER TABLE channel_post_history FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope ON channel_post_history USING(workspace_id::text=current_setting('app.workspace_id',true)) WITH CHECK(workspace_id::text=current_setting('app.workspace_id',true));

CREATE FUNCTION guard_channel_post() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'channel post history retained'; END IF;
 IF NEW.workspace_id<>OLD.workspace_id OR NEW.channel_id<>OLD.channel_id OR NEW.actor_id<>OLD.actor_id OR NEW.operation_key<>OLD.operation_key OR NEW.fingerprint<>OLD.fingerprint THEN RAISE EXCEPTION 'channel post identity immutable'; END IF;
 IF NEW.revision<>OLD.revision+1 THEN RAISE EXCEPTION 'channel post revision invalid'; END IF;
 IF OLD.state IN('SENT','FAILED','UNKNOWN','CANCELLED') THEN RAISE EXCEPTION 'channel post terminal result immutable'; END IF;
 IF OLD.state<>'DRAFT' AND NEW.body<>OLD.body THEN RAISE EXCEPTION 'scheduled channel body immutable'; END IF;
 IF NOT (
   (OLD.state='DRAFT' AND NEW.state IN('DRAFT','SCHEDULED','CANCELLED')) OR
   (OLD.state='SCHEDULED' AND NEW.state IN('SCHEDULED','SENDING','CANCELLED')) OR
   (OLD.state='SENDING' AND NEW.state IN('SENT','FAILED','UNKNOWN'))
 ) THEN RAISE EXCEPTION 'channel post transition invalid'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER channel_post_guard BEFORE UPDATE OR DELETE ON channel_posts FOR EACH ROW EXECUTE FUNCTION guard_channel_post();
CREATE TRIGGER channel_post_history_immutable BEFORE UPDATE OR DELETE ON channel_post_history FOR EACH ROW EXECUTE FUNCTION reject_mutation();

UPDATE feature_flags SET enabled=true,rollout_status='ON',updated_at=now() WHERE feature_key='channels';
