-- Migration owner is a dedicated BYPASSRLS role; runtime never inherits it.
CREATE TABLE platform_permissions (key text PRIMARY KEY);
INSERT INTO platform_permissions SELECT unnest(ARRAY['platform.users.read','platform.users.message','platform.plans.read','platform.plans.manage','platform.features.manage','platform.plugins.manage','platform.wallets.read','platform.wallets.adjust','platform.payments.read','platform.refunds.manage','platform.audit.read']);
CREATE TABLE platform_admin_permissions (user_id uuid REFERENCES platform_admins(user_id), permission_key text REFERENCES platform_permissions(key), PRIMARY KEY(user_id,permission_key));
-- Existing configured owners retain explicit permissions; membership alone grants nothing.
INSERT INTO platform_admin_permissions SELECT a.user_id,p.key FROM platform_admins a CROSS JOIN platform_permissions p;
CREATE FUNCTION platform_plan_in_use(actor uuid, plan text) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public SET row_security=off AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM public.platform_admin_permissions WHERE user_id=actor AND permission_key='platform.plans.manage') THEN RAISE EXCEPTION 'permission denied' USING ERRCODE='42501'; END IF;
 RETURN EXISTS(SELECT 1 FROM public.subscriptions WHERE plan_key=plan OR previous_plan_key=plan OR scheduled_plan_key=plan);
END $$;
REVOKE ALL ON FUNCTION platform_plan_in_use(uuid,text) FROM PUBLIC;
ALTER TABLE features ADD COLUMN limit_kind text NOT NULL DEFAULT 'USAGE' CHECK(limit_kind IN('USAGE','RESOURCE_COUNT','CAPACITY','CREDIT','ITEM_SIZE','NONE'));
ALTER TABLE features ADD COLUMN limit_unit text NOT NULL DEFAULT 'operation';
ALTER TABLE features ADD COLUMN reset_window text CHECK(reset_window IN('MONTH','DAY')) DEFAULT 'MONTH';
UPDATE features SET limit_kind='RESOURCE_COUNT',limit_unit='resource',reset_window=NULL WHERE key IN('channels','groups','bots','team');
UPDATE features SET limit_kind='CAPACITY',limit_unit='byte',reset_window=NULL WHERE key='uploader';
UPDATE features SET limit_kind='CREDIT',limit_unit='credit' WHERE key='ai';
UPDATE features SET limit_kind='NONE',reset_window=NULL WHERE key='wallet';
CREATE TABLE subscription_operations (workspace_id uuid NOT NULL REFERENCES workspaces(id), idempotency_key text NOT NULL, fingerprint text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(workspace_id,idempotency_key));
ALTER TABLE subscription_operations ENABLE ROW LEVEL SECURITY;
ALTER TABLE subscription_operations FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope ON subscription_operations USING(workspace_id::text=current_setting('app.workspace_id',true)) WITH CHECK(workspace_id::text=current_setting('app.workspace_id',true));
CREATE TRIGGER subscription_operations_no_mutation BEFORE UPDATE OR DELETE ON subscription_operations FOR EACH ROW EXECUTE FUNCTION reject_mutation();
ALTER TABLE outbox_events ADD COLUMN lease_owner text;
ALTER TABLE outbox_events ADD COLUMN available_at timestamptz NOT NULL DEFAULT now();
