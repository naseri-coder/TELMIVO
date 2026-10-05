-- R4F: AI Assistant 2.0 hybrid retrieval, source ingestion and cost accounting.

ALTER TABLE ai_knowledge DROP CONSTRAINT IF EXISTS ai_knowledge_body_check;
ALTER TABLE ai_knowledge ADD CONSTRAINT ai_knowledge_body_check CHECK(length(body) BETWEEN 1 AND 65536);
ALTER TABLE ai_knowledge
  ADD COLUMN source_kind text NOT NULL DEFAULT 'MANUAL' CHECK(source_kind IN('MANUAL','FILE_MANAGER','UPLOADER')),
  ADD COLUMN source_ref text NOT NULL DEFAULT '' CHECK(length(source_ref)<=200);
CREATE INDEX ai_knowledge_source ON ai_knowledge(workspace_id,bot_id,source_kind,source_ref);

CREATE TABLE ai_knowledge_chunks(
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 workspace_id uuid NOT NULL,
 bot_id uuid NOT NULL,
 knowledge_id uuid NOT NULL,
 ordinal int NOT NULL CHECK(ordinal>=0 AND ordinal<128),
 body text NOT NULL CHECK(length(body) BETWEEN 1 AND 2048),
 vector jsonb NOT NULL,
 vector_kind text NOT NULL DEFAULT 'HASH_NGRAM_V1',
 fingerprint text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(knowledge_id,ordinal),
 UNIQUE(id,workspace_id,bot_id),
 FOREIGN KEY(knowledge_id,workspace_id,bot_id) REFERENCES ai_knowledge(id,workspace_id,bot_id)
);
CREATE INDEX ai_knowledge_chunks_search ON ai_knowledge_chunks USING gin(to_tsvector('simple',body));
CREATE INDEX ai_knowledge_chunks_source ON ai_knowledge_chunks(workspace_id,bot_id,knowledge_id,ordinal);
ALTER TABLE ai_knowledge_chunks ENABLE ROW LEVEL SECURITY;
ALTER TABLE ai_knowledge_chunks FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope ON ai_knowledge_chunks
 USING(workspace_id::text=current_setting('app.workspace_id',true))
 WITH CHECK(workspace_id::text=current_setting('app.workspace_id',true));

ALTER TABLE ai_requests
  ADD COLUMN cost_microunits bigint NOT NULL DEFAULT 0 CHECK(cost_microunits>=0),
  ADD COLUMN retrieved_chunks int NOT NULL DEFAULT 0 CHECK(retrieved_chunks BETWEEN 0 AND 8);

INSERT INTO commercial_catalog(kind,key,version,enabled,effective_at,definition,reason)
SELECT 'QUOTA','ai.cost_microunits.monthly',1,true,'2020-01-01T00:00:00Z',
       jsonb_build_object('quota',jsonb_build_object('scope','PER_BOT','reset','MONTHLY','default',jsonb_build_object('value','0','unlimited',true))),
       'R4F configurable AI cost ceiling'
WHERE NOT EXISTS (
 SELECT 1 FROM commercial_catalog WHERE kind='QUOTA' AND key='ai.cost_microunits.monthly' AND version=1
);
