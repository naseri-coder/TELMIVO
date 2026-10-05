-- R4D: provider-neutral checkout, verified payment and refund lifecycle.
CREATE TABLE commerce_payment_intents(
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 workspace_id uuid NOT NULL,
 bot_id uuid NOT NULL,
 order_id uuid NOT NULL,
 contact_id uuid NOT NULL,
 rail text NOT NULL CHECK(rail IN('TELEGRAM_STARS','EXTERNAL')),
 provider_key text NOT NULL CHECK(provider_key~'^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$'),
 state text NOT NULL CHECK(state IN('CREATING','AWAITING_PAYMENT','PAID','FAILED','UNKNOWN','CANCELLED','REFUND_PENDING','REFUND_UNKNOWN','REFUNDED')),
 amount jsonb NOT NULL CHECK(octet_length(amount::text)<=512),
 operation_key text NOT NULL CHECK(length(operation_key) BETWEEN 1 AND 100),
 fingerprint text NOT NULL CHECK(length(fingerprint)=64),
 checkout_reference text NOT NULL DEFAULT '',
 checkout_url text NOT NULL DEFAULT '' CHECK(length(checkout_url)<=4096),
 provider_payment_id text NOT NULL DEFAULT '' CHECK(length(provider_payment_id)<=512),
 payer_telegram_id bigint CHECK(payer_telegram_id IS NULL OR payer_telegram_id>0),
 outcome_code text NOT NULL DEFAULT '',
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 paid_at timestamptz,
 refunded_at timestamptz,
 UNIQUE(id,workspace_id,bot_id),
 UNIQUE(bot_id,operation_key),
 FOREIGN KEY(order_id,workspace_id,bot_id) REFERENCES commerce_orders(id,workspace_id,bot_id),
 FOREIGN KEY(contact_id,workspace_id,bot_id) REFERENCES crm_contacts(id,workspace_id,bot_id)
);
CREATE INDEX commerce_payment_order ON commerce_payment_intents(workspace_id,bot_id,order_id,created_at DESC);
CREATE INDEX commerce_payment_state ON commerce_payment_intents(workspace_id,bot_id,state,updated_at);
CREATE UNIQUE INDEX commerce_payment_provider_identity ON commerce_payment_intents(bot_id,provider_key,provider_payment_id) WHERE provider_payment_id<>'';

CREATE TABLE commerce_payment_events(
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 workspace_id uuid NOT NULL,
 bot_id uuid NOT NULL,
 payment_id uuid NOT NULL,
 event_key text NOT NULL CHECK(length(event_key) BETWEEN 1 AND 200),
 kind text NOT NULL CHECK(kind IN('PRECHECKOUT_ACCEPTED','PAYMENT_VERIFIED','PAYMENT_REJECTED','CHECKOUT_RECONCILED','REFUND_VERIFIED','REFUND_REJECTED')),
 amount jsonb,
 external_reference text NOT NULL DEFAULT '' CHECK(length(external_reference)<=512),
 digest text NOT NULL CHECK(length(digest)=64),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(payment_id,event_key),
 FOREIGN KEY(payment_id,workspace_id,bot_id) REFERENCES commerce_payment_intents(id,workspace_id,bot_id)
);

CREATE TABLE commerce_refunds(
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 workspace_id uuid NOT NULL,
 bot_id uuid NOT NULL,
 payment_id uuid NOT NULL,
 state text NOT NULL CHECK(state IN('REQUESTED','PROCESSING','REFUNDED','FAILED','UNKNOWN')),
 amount jsonb NOT NULL CHECK(octet_length(amount::text)<=512),
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 500),
 operation_key text NOT NULL CHECK(length(operation_key) BETWEEN 1 AND 100),
 fingerprint text NOT NULL CHECK(length(fingerprint)=64),
 provider_reference text NOT NULL DEFAULT '' CHECK(length(provider_reference)<=512),
 outcome_code text NOT NULL DEFAULT '',
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 completed_at timestamptz,
 UNIQUE(id,workspace_id,bot_id),
 UNIQUE(payment_id,operation_key),
 FOREIGN KEY(payment_id,workspace_id,bot_id) REFERENCES commerce_payment_intents(id,workspace_id,bot_id)
);
CREATE INDEX commerce_refund_payment ON commerce_refunds(workspace_id,bot_id,payment_id,created_at DESC);

CREATE OR REPLACE FUNCTION guard_commerce_payment() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'payment records are retained'; END IF;
 IF TG_TABLE_NAME='commerce_payment_events' THEN RAISE EXCEPTION 'payment events are immutable'; END IF;
 IF NEW.workspace_id<>OLD.workspace_id OR NEW.bot_id<>OLD.bot_id THEN RAISE EXCEPTION 'payment scope immutable'; END IF;
 IF TG_TABLE_NAME='commerce_payment_intents' THEN
  IF NEW.order_id<>OLD.order_id OR NEW.contact_id<>OLD.contact_id OR NEW.rail<>OLD.rail OR NEW.provider_key<>OLD.provider_key OR NEW.amount<>OLD.amount OR NEW.operation_key<>OLD.operation_key OR NEW.fingerprint<>OLD.fingerprint OR NEW.payer_telegram_id IS DISTINCT FROM OLD.payer_telegram_id THEN RAISE EXCEPTION 'payment identity immutable'; END IF;
  IF NEW.revision<>OLD.revision+1 THEN RAISE EXCEPTION 'payment revision invalid'; END IF;
  IF OLD.state IN('FAILED','CANCELLED','REFUNDED') THEN RAISE EXCEPTION 'payment terminal state immutable'; END IF;
  IF NEW.state<>OLD.state AND NOT (
    (OLD.state='CREATING' AND NEW.state IN('AWAITING_PAYMENT','FAILED','UNKNOWN','CANCELLED')) OR
    (OLD.state='AWAITING_PAYMENT' AND NEW.state IN('PAID','FAILED','UNKNOWN','CANCELLED')) OR
    (OLD.state='UNKNOWN' AND NEW.state IN('AWAITING_PAYMENT','PAID','FAILED','CANCELLED','REFUND_PENDING','REFUNDED')) OR
    (OLD.state='PAID' AND NEW.state IN('REFUND_PENDING','REFUNDED')) OR
    (OLD.state='REFUND_PENDING' AND NEW.state IN('PAID','REFUND_UNKNOWN','REFUNDED')) OR
    (OLD.state='REFUND_UNKNOWN' AND NEW.state IN('PAID','REFUNDED'))
  ) THEN RAISE EXCEPTION 'payment transition invalid'; END IF;
 END IF;
 IF TG_TABLE_NAME='commerce_refunds' THEN
  IF NEW.payment_id<>OLD.payment_id OR NEW.amount<>OLD.amount OR NEW.reason<>OLD.reason OR NEW.operation_key<>OLD.operation_key OR NEW.fingerprint<>OLD.fingerprint THEN RAISE EXCEPTION 'refund identity immutable'; END IF;
  IF NEW.revision<>OLD.revision+1 THEN RAISE EXCEPTION 'refund revision invalid'; END IF;
  IF OLD.state IN('REFUNDED','FAILED','UNKNOWN') THEN RAISE EXCEPTION 'refund terminal state immutable'; END IF;
  IF NEW.state<>OLD.state AND NOT (
    (OLD.state='REQUESTED' AND NEW.state='PROCESSING') OR
    (OLD.state='PROCESSING' AND NEW.state IN('REFUNDED','FAILED','UNKNOWN'))
  ) THEN RAISE EXCEPTION 'refund transition invalid'; END IF;
 END IF;
 RETURN NEW;
END $$;

DO $$ DECLARE t text;BEGIN
 FOREACH t IN ARRAY ARRAY['commerce_payment_intents','commerce_payment_events','commerce_refunds'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',t);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',t);
  EXECUTE format('CREATE POLICY tenant_scope ON %I USING(workspace_id::text=current_setting(''app.workspace_id'',true)) WITH CHECK(workspace_id::text=current_setting(''app.workspace_id'',true))',t);
  EXECUTE format('CREATE TRIGGER commerce_payment_guard BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION guard_commerce_payment()',t);
 END LOOP;
END $$;
