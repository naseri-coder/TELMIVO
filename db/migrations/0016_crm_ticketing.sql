-- Customer-bot support data is distinct from legacy TELMIVO platform-support tickets.
CREATE TABLE crm_contacts(id uuid PRIMARY KEY DEFAULT gen_random_uuid(),workspace_id uuid NOT NULL,bot_id uuid NOT NULL,definition jsonb NOT NULL,revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),first_seen timestamptz NOT NULL,last_seen timestamptz NOT NULL,UNIQUE(id,workspace_id,bot_id),FOREIGN KEY(bot_id,workspace_id) REFERENCES customer_bots(id,workspace_id));
CREATE INDEX crm_contact_list ON crm_contacts(workspace_id,bot_id,first_seen DESC,id);
CREATE INDEX crm_contact_tags ON crm_contacts USING gin((definition->'tags'));
CREATE TABLE crm_identities(workspace_id uuid NOT NULL,bot_id uuid NOT NULL,contact_id uuid NOT NULL,provider text NOT NULL,external_key text NOT NULL,PRIMARY KEY(bot_id,provider,external_key),FOREIGN KEY(contact_id,workspace_id,bot_id) REFERENCES crm_contacts(id,workspace_id,bot_id));
CREATE TABLE support_departments(id uuid PRIMARY KEY DEFAULT gen_random_uuid(),workspace_id uuid NOT NULL,bot_id uuid NOT NULL,definition jsonb NOT NULL,revision bigint NOT NULL DEFAULT 1,operation_key text NOT NULL,fingerprint text NOT NULL,UNIQUE(id,workspace_id,bot_id),UNIQUE(bot_id,operation_key),FOREIGN KEY(bot_id,workspace_id) REFERENCES customer_bots(id,workspace_id));
CREATE TABLE support_tickets(reference text PRIMARY KEY CHECK(reference~'^[A-Za-z0-9_-]{32}$'),workspace_id uuid NOT NULL,bot_id uuid NOT NULL,contact_id uuid NOT NULL,department_id uuid NOT NULL,subject text NOT NULL CHECK(length(subject) BETWEEN 1 AND 200),status text NOT NULL CHECK(status IN('OPEN','PENDING_CUSTOMER','PENDING_AGENT','RESOLVED','CLOSED')),priority text NOT NULL CHECK(priority IN('LOW','NORMAL','HIGH','URGENT')),assignee_id uuid,tags jsonb NOT NULL DEFAULT '[]',revision bigint NOT NULL DEFAULT 1,created_at timestamptz NOT NULL,updated_at timestamptz NOT NULL,closed_at timestamptz,first_response_at timestamptz,resolved_at timestamptz,response_due_at timestamptz,resolution_due_at timestamptz,UNIQUE(reference,workspace_id,bot_id),FOREIGN KEY(contact_id,workspace_id,bot_id) REFERENCES crm_contacts(id,workspace_id,bot_id),FOREIGN KEY(department_id,workspace_id,bot_id) REFERENCES support_departments(id,workspace_id,bot_id),FOREIGN KEY(workspace_id,assignee_id) REFERENCES workspace_members(workspace_id,user_id));
CREATE UNIQUE INDEX support_one_active ON support_tickets(bot_id,contact_id,department_id) WHERE status IN('OPEN','PENDING_CUSTOMER','PENDING_AGENT');
CREATE INDEX support_inbox ON support_tickets(workspace_id,bot_id,created_at DESC,reference);
CREATE INDEX support_contact_history ON support_tickets(workspace_id,bot_id,contact_id,created_at DESC);
CREATE TABLE support_messages(id uuid PRIMARY KEY DEFAULT gen_random_uuid(),workspace_id uuid NOT NULL,bot_id uuid NOT NULL,ticket_reference text NOT NULL,kind text NOT NULL CHECK(kind IN('CUSTOMER','AGENT','SYSTEM','INTERNAL')),body text NOT NULL CHECK(length(body) BETWEEN 1 AND 4000),attachments jsonb NOT NULL DEFAULT '[]',actor_id uuid,operation_key text NOT NULL,fingerprint text NOT NULL,delivery text NOT NULL CHECK(delivery IN('NOT_APPLICABLE','SENDING','SENT','FAILED','UNKNOWN')),created_at timestamptz NOT NULL,UNIQUE(bot_id,operation_key),FOREIGN KEY(ticket_reference,workspace_id,bot_id) REFERENCES support_tickets(reference,workspace_id,bot_id),FOREIGN KEY(workspace_id,actor_id) REFERENCES workspace_members(workspace_id,user_id),CHECK(kind='AGENT' OR delivery='NOT_APPLICABLE'));
CREATE INDEX support_conversation ON support_messages(workspace_id,bot_id,ticket_reference,created_at,id);
CREATE TABLE support_templates(id uuid PRIMARY KEY DEFAULT gen_random_uuid(),workspace_id uuid NOT NULL,bot_id uuid NOT NULL,definition jsonb NOT NULL,revision bigint NOT NULL DEFAULT 1,operation_key text NOT NULL,fingerprint text NOT NULL,UNIQUE(bot_id,operation_key),FOREIGN KEY(bot_id,workspace_id) REFERENCES customer_bots(id,workspace_id));
CREATE FUNCTION guard_support_record() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'support retention requires explicit purge policy'; END IF;
 IF TG_TABLE_NAME='crm_identities' THEN RAISE EXCEPTION 'external identity is immutable'; END IF;
 IF TG_TABLE_NAME='support_messages' THEN
  IF OLD.delivery<>'SENDING' OR NEW.delivery NOT IN('SENT','FAILED','UNKNOWN') OR (to_jsonb(NEW)-'delivery') IS DISTINCT FROM (to_jsonb(OLD)-'delivery') THEN RAISE EXCEPTION 'message or terminal delivery is immutable'; END IF;
 ELSE
  IF NEW.revision<>OLD.revision+1 OR NEW.workspace_id<>OLD.workspace_id OR NEW.bot_id<>OLD.bot_id THEN RAISE EXCEPTION 'support scope or revision invalid'; END IF;
  IF TG_TABLE_NAME='support_tickets' THEN
   IF NEW.reference<>OLD.reference OR NEW.contact_id<>OLD.contact_id OR NEW.created_at<>OLD.created_at THEN RAISE EXCEPTION 'ticket identity immutable'; END IF;
  END IF;
 END IF;RETURN NEW;END $$;
DO $$ DECLARE t text;BEGIN FOREACH t IN ARRAY ARRAY['crm_contacts','crm_identities','support_departments','support_tickets','support_messages','support_templates'] LOOP
 EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',t);EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',t);
 EXECUTE format('CREATE POLICY tenant_scope ON %I USING(workspace_id::text=current_setting(''app.workspace_id'',true)) WITH CHECK(workspace_id::text=current_setting(''app.workspace_id'',true))',t);
 EXECUTE format('CREATE TRIGGER support_guard BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION guard_support_record()',t);
 END LOOP;END $$;
INSERT INTO permissions(key) VALUES('crm.read'),('crm.manage'),('tickets.manage'),('tickets.notes');
INSERT INTO role_permissions(role_key,permission_key) SELECT r,p FROM (VALUES('OWNER'),('ADMIN'),('SUPPORT_AGENT')) a(r) CROSS JOIN (VALUES('crm.read'),('crm.manage'),('tickets.manage'),('tickets.notes')) b(p);
INSERT INTO role_permissions VALUES('SALES_AGENT','crm.read');
INSERT INTO commercial_catalog(kind,key,version,enabled,effective_at,definition,reason)
 SELECT 'PLUGIN',key,1,true,'2020-01-01T00:00:00Z',jsonb_build_object('plugin',jsonb_build_object('capabilities',caps,'sale_enabled',false,'scope','PER_BOT','prices',jsonb_build_object('daily',jsonb_build_object('unit','USD','scale',2,'amount','0'),'monthly',jsonb_build_object('unit','USD','scale',2,'amount','0')),'trial',jsonb_build_object('enabled',false,'duration_seconds',0),'grace_seconds',0)), 'R3D-5 inactive commercial placeholder; configure before sale'
 FROM (VALUES('crm','["crm.read","crm.write"]'::jsonb),('ticketing','["tickets.read","tickets.write","telegram.send"]'::jsonb)) p(key,caps);
INSERT INTO commercial_catalog(kind,key,version,enabled,effective_at,definition,reason)
 SELECT 'QUOTA',key,1,true,'2020-01-01T00:00:00Z',jsonb_build_object('quota',jsonb_build_object('scope','PER_BOT','reset',period,'default',jsonb_build_object('value','0','unlimited',true))), 'R3D-5 configurable quota; unlimited initial policy'
 FROM (VALUES('crm.contacts.count','NONE'),('ticketing.active_tickets.count','NONE'),('ticketing.messages.monthly','MONTHLY'),('ticketing.agents.count','NONE')) q(key,period);

CREATE INDEX crm_name_prefix ON crm_contacts USING spgist((lower(definition->>'name')));
CREATE INDEX crm_username_prefix ON crm_contacts USING spgist((lower(definition->>'username')));
CREATE INDEX crm_identity_prefix ON crm_identities USING spgist(external_key);
CREATE INDEX support_subject_prefix ON support_tickets USING spgist((lower(subject)));
CREATE INDEX support_reference_prefix ON support_tickets USING spgist(reference);
