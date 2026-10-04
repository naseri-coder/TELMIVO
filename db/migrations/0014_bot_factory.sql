-- Existing bot identity is retained. Only missing replay/duplicate-credential
-- evidence is added; no second bot model or plaintext secret storage.
ALTER TABLE customer_bots ADD COLUMN factory_fingerprint text CHECK(factory_fingerprint IS NULL OR factory_fingerprint ~ '^[a-f0-9]{64}$');
ALTER TABLE customer_bots ADD COLUMN factory_token_digest text CHECK(factory_token_digest IS NULL OR factory_token_digest ~ '^[a-f0-9]{64}$');
CREATE UNIQUE INDEX customer_bot_factory_token ON customer_bots(workspace_id,factory_token_digest) WHERE factory_token_digest IS NOT NULL;
