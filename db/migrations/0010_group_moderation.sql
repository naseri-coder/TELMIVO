-- R3B: existing group_settings owns configuration; append-only moderation history.
-- No new identity, claim, trial, event queue or Telegram deduplication model.
ALTER TABLE group_settings ADD COLUMN revision bigint NOT NULL DEFAULT 1 CHECK(revision>0);
CREATE TABLE group_moderation_events (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), sequence bigint GENERATED ALWAYS AS IDENTITY UNIQUE, workspace_id uuid NOT NULL REFERENCES workspaces(id),
 group_id uuid NOT NULL, target_id bigint NOT NULL, actor_id uuid REFERENCES users(id),
 operation_key text NOT NULL CHECK(length(operation_key) BETWEEN 1 AND 200),
 kind text NOT NULL CHECK(kind IN('MESSAGE','DECISION','WARNING','CLEAR_WARNINGS','ACTION_RESULT','AUTHORITY')),
 action text NOT NULL, reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 1000),
 message_digest text NOT NULL DEFAULT '', message_id bigint NOT NULL DEFAULT 0,
 outcome text NOT NULL CHECK(outcome IN('RECORDED','ACTION_REQUESTED','ACTION_ACKNOWLEDGED','ACTION_FAILED','ACTION_OUTCOME_UNKNOWN')),
 correlation_id text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(group_id,workspace_id) REFERENCES groups(id,workspace_id), UNIQUE(workspace_id,group_id,operation_key,kind)
);
CREATE INDEX group_moderation_history ON group_moderation_events(workspace_id,group_id,created_at DESC,id DESC);
CREATE INDEX group_moderation_window ON group_moderation_events(workspace_id,group_id,target_id,created_at) WHERE kind='MESSAGE';
ALTER TABLE group_moderation_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE group_moderation_events FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope ON group_moderation_events USING(workspace_id::text=current_setting('app.workspace_id',true)) WITH CHECK(workspace_id::text=current_setting('app.workspace_id',true));
CREATE TRIGGER group_moderation_immutable BEFORE UPDATE OR DELETE ON group_moderation_events FOR EACH ROW EXECUTE FUNCTION reject_mutation();
-- Narrow server routing projection; ownership remains solely in immutable group_claims.
-- The migration role owns this function. It returns no group content/settings and grants
-- no tenant authority; every subsequent operation must use scoped restricted runtime SQL.
CREATE FUNCTION telegram_group_route(chat bigint) RETURNS TABLE(workspace_id uuid,group_id uuid)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,public AS $$
 SELECT c.workspace_id,c.group_id FROM public.group_claims c WHERE c.telegram_chat_id=chat
$$;
REVOKE ALL ON FUNCTION telegram_group_route(bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION telegram_group_route(bigint) TO telmivo_runtime;

-- Activate the now-implemented product; commercial entitlement remains resolver-driven.
UPDATE feature_flags SET enabled=true,rollout_status='ON' WHERE feature_key='groups';
