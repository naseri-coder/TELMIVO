-- R4E: Automation 2.0 durable cron schedule state.
CREATE TABLE automation_schedule_state(
 workspace_id uuid NOT NULL,
 bot_id uuid NOT NULL,
 workflow_id uuid PRIMARY KEY,
 expression text NOT NULL DEFAULT '' CHECK(length(expression)<=128),
 timezone text NOT NULL DEFAULT 'UTC' CHECK(length(timezone) BETWEEN 1 AND 64),
 next_at timestamptz,
 last_at timestamptz,
 occurrence bigint NOT NULL DEFAULT 0 CHECK(occurrence>=0),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 updated_at timestamptz NOT NULL,
 UNIQUE(workflow_id,workspace_id,bot_id),
 FOREIGN KEY(workflow_id,workspace_id,bot_id) REFERENCES automation_workflows(id,workspace_id,bot_id)
);
CREATE INDEX automation_schedule_due ON automation_schedule_state(workspace_id,next_at,workflow_id) WHERE next_at IS NOT NULL;

ALTER TABLE automation_schedule_state ENABLE ROW LEVEL SECURITY;
ALTER TABLE automation_schedule_state FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope ON automation_schedule_state
 USING(workspace_id::text=current_setting('app.workspace_id',true))
 WITH CHECK(workspace_id::text=current_setting('app.workspace_id',true));

CREATE FUNCTION guard_automation_schedule_state() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'automation schedule state is retained'; END IF;
 IF NEW.workspace_id<>OLD.workspace_id OR NEW.bot_id<>OLD.bot_id OR NEW.workflow_id<>OLD.workflow_id THEN RAISE EXCEPTION 'automation schedule scope immutable'; END IF;
 IF NEW.revision<>OLD.revision+1 THEN RAISE EXCEPTION 'automation schedule revision invalid'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER automation_schedule_state_guard BEFORE UPDATE OR DELETE ON automation_schedule_state
 FOR EACH ROW EXECUTE FUNCTION guard_automation_schedule_state();
