-- Immutable ingestion identities are separate from mutable knowledge versions.
CREATE TABLE ai_ingestion_receipts (
 workspace_id uuid NOT NULL,
 bot_id uuid NOT NULL,
 operation_key text NOT NULL CHECK(length(operation_key) BETWEEN 1 AND 100),
 fingerprint text NOT NULL,
 knowledge_id uuid NOT NULL,
 result jsonb NOT NULL,
 legacy_ambiguous boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(bot_id,operation_key),
 FOREIGN KEY(knowledge_id,workspace_id,bot_id) REFERENCES ai_knowledge(id,workspace_id,bot_id)
);
-- Older mutable fingerprints cannot prove the original ingestion request.
-- Reserve those keys fail-closed; re-ingestion must use a fresh key.
INSERT INTO ai_ingestion_receipts(workspace_id,bot_id,operation_key,fingerprint,knowledge_id,result,legacy_ambiguous)
 SELECT workspace_id,bot_id,operation_key,fingerprint,id,'{}'::jsonb,true
 FROM ai_knowledge WHERE source_kind<>'MANUAL';
ALTER TABLE ai_ingestion_receipts ENABLE ROW LEVEL SECURITY;
ALTER TABLE ai_ingestion_receipts FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope ON ai_ingestion_receipts
 USING(workspace_id::text=current_setting('app.workspace_id',true))
 WITH CHECK(workspace_id::text=current_setting('app.workspace_id',true));
CREATE FUNCTION guard_ai_ingestion_receipt() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'AI ingestion receipts are immutable';
END $$;
CREATE TRIGGER ai_ingestion_receipt_guard BEFORE UPDATE OR DELETE ON ai_ingestion_receipts
 FOR EACH ROW EXECUTE FUNCTION guard_ai_ingestion_receipt();
