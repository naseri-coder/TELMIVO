-- Opt-in plugin runtime. Legacy customer_bot_modules and catalog remain unchanged.
CREATE TABLE plugin_installations (
 workspace_id uuid NOT NULL, bot_id uuid NOT NULL,
 plugin_id text NOT NULL CHECK(plugin_id ~ '^[a-z][a-z0-9_.-]{0,99}$'),
 plugin_version text NOT NULL, data_version integer NOT NULL CHECK(data_version>0),
 state text NOT NULL CHECK(state IN('INSTALLED','ACTIVE','DISABLED','UNINSTALL_PENDING','UNINSTALLED')),
 configuration jsonb NOT NULL CHECK(jsonb_typeof(configuration)='object'),
 data jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(data)='object'),
 capabilities jsonb NOT NULL DEFAULT '[]' CHECK(jsonb_typeof(capabilities)='array'),
 license_scope text NOT NULL CHECK(license_scope IN('PER_BOT','PER_ACCOUNT')),
 policy_revision text NOT NULL DEFAULT '', revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 PRIMARY KEY(bot_id,plugin_id),
 FOREIGN KEY(bot_id,workspace_id) REFERENCES customer_bots(id,workspace_id)
);
CREATE INDEX plugin_installations_workspace ON plugin_installations(workspace_id);
ALTER TABLE plugin_installations ENABLE ROW LEVEL SECURITY;
ALTER TABLE plugin_installations FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope ON plugin_installations
 USING(workspace_id::text=current_setting('app.workspace_id',true))
 WITH CHECK(workspace_id::text=current_setting('app.workspace_id',true));

-- Tombstones and retained data are durable. No implicit purge, upgrade or reinstall.
CREATE FUNCTION guard_plugin_installation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'plugin retention requires explicit future policy'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.state<>'INSTALLED' OR NEW.revision<>1 THEN RAISE EXCEPTION 'invalid initial plugin state'; END IF;
  RETURN NEW;
 END IF;
 IF NEW.workspace_id<>OLD.workspace_id OR NEW.bot_id<>OLD.bot_id OR NEW.plugin_id<>OLD.plugin_id
 OR NEW.plugin_version<>OLD.plugin_version OR NEW.data_version<>OLD.data_version

 OR NEW.revision<>OLD.revision+1 THEN RAISE EXCEPTION 'immutable plugin identity/data or stale revision'; END IF;
 -- Future Core-managed configuration/data writes may use the same state and revision fence.
 IF NEW.state=OLD.state AND OLD.state IN('INSTALLED','ACTIVE','DISABLED') THEN RETURN NEW; END IF;
 IF NEW.configuration IS DISTINCT FROM OLD.configuration OR NEW.data IS DISTINCT FROM OLD.data
 THEN RAISE EXCEPTION 'plugin lifecycle must retain configuration and data'; END IF;
 IF NOT ((OLD.state='INSTALLED' AND NEW.state IN('ACTIVE','DISABLED','UNINSTALL_PENDING'))
 OR (OLD.state='ACTIVE' AND NEW.state='DISABLED')
 OR (OLD.state='DISABLED' AND NEW.state IN('ACTIVE','UNINSTALL_PENDING'))
 OR (OLD.state='UNINSTALL_PENDING' AND NEW.state='UNINSTALLED'))
 THEN RAISE EXCEPTION 'invalid plugin lifecycle transition'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER plugin_installation_guard BEFORE INSERT OR UPDATE OR DELETE ON plugin_installations
 FOR EACH ROW EXECUTE FUNCTION guard_plugin_installation();
