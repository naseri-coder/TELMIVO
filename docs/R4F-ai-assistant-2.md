# R4F — AI Assistant 2.0

Base checkpoint: stacked R4E Automation 2.0 source checkpoint.

R4F extends the existing bounded AI Assistant domain. It does not introduce an autonomous
unbounded agent, a second authorization path, an arbitrary code runtime or an external vector
database requirement.

## Retrieval

The assistant configuration supports `LEXICAL` and `HYBRID` retrieval. Manual knowledge and
approved imported sources are chunked durably in PostgreSQL. R4F defines a provider-neutral vector
contract and ships a deterministic local hashed word/trigram feature vector as the baseline
implementation. This keeps the source runnable without pgvector or a vendor-specific embedding
service while preserving a replaceable vector boundary for a future approved embedding provider.

Hybrid ranking combines local vector cosine similarity with lexical token overlap. The number of
retrieved chunks is bounded per request (1–8) and recorded on the request usage row.

## Knowledge ingestion

Two TELMIVO-owned source types are admitted:

- `FILE_MANAGER:<reference>`: metadata from an enabled File Manager record, after the existing
  File Manager execution authorization succeeds.
- `UPLOADER:<upload-id>`: UTF-8 text from a private stored Uploader object. Only
  `text/plain`, `text/markdown` and `application/json` sources up to 64 KiB are accepted,
  and the upload must belong to the same workspace/customer bot.

No URL fetch, shell command, arbitrary parser, arbitrary SQL or arbitrary webhook is used for
knowledge ingestion. Imported sources have stable source identity and may be refreshed explicitly
with a new idempotency key when their authoritative TELMIVO content changes.

The legacy assistant `files` allowlist remains dedicated to the `file.metadata` tool. R4F uses
a separate `sources` allowlist for retrieval corpus selection, so the older tool contract is not
redefined.

## Provider catalog and cost controls

AI model catalog entries now carry optional input/output microunit prices per one million tokens in
addition to existing tool, structured-output and token-limit capabilities. A configuration may set
a per-request cost ceiling. Before a provider call, TELMIVO computes a conservative maximum cost
from the selected model/configuration and rejects requests above the ceiling. Actual cost,
input/output tokens, tool calls, retrieved chunks and latency are persisted after completion.

The commercial quota `ai.cost_microunits.monthly` is configurable and defaults to unlimited in
this source checkpoint. Pricing metadata is operator configuration; TELMIVO does not invent or
fetch provider prices automatically.

## Agent safety

Existing write-tool rules remain authoritative. A configured write tool still requires:

- `write_enabled=true`;
- an explicitly authorized request (`confirm_writes=true`);
- a non-customer runtime context;
- a purpose other than classification.

R4F does not grant write authority from retrieval content, model output or imported files.

## Persistence and recovery

Migration 0023 widens bounded knowledge storage, adds source identity, durable knowledge chunks,
retrieval/cost accounting and the cost quota catalog entry. New chunk rows use FORCE RLS and
workspace/bot composite ownership.

Native validation covers vector behavior, hybrid retrieval, File Manager and Uploader ingestion,
cost accounting/ceiling enforcement, browser configuration and backup/restore of the indexed
corpus. Validation remains isolated and synthetic; this checkpoint does not authorize production
deployment.
