-- LIC-A9 A2-02: read-only PostgreSQL role/RLS inventory.
-- Run ONLY against a verified disposable PostgreSQL16 test database.
-- This query neither switches roles nor modifies grants, schema, or rows.
-- Snapshot must be reviewed before planning a worker API credential cutover.
SELECT
  r.rolname,
  r.rolcanlogin,
  r.rolsuper,
  r.rolbypassrls,
  r.rolinherit
FROM pg_roles r
WHERE r.rolname IN ('telmivo_runtime','telmivo_migration','telmivo_worker')
ORDER BY r.rolname;

SELECT
  c.relname AS journal,
  c.relrowsecurity AS rls_enabled,
  c.relforcerowsecurity AS rls_forced,
  EXISTS (
    SELECT 1 FROM pg_attribute a
    WHERE a.attrelid=c.oid AND a.attname='workspace_id' AND NOT a.attisdropped
  ) AS workspace_column_present,
  has_table_privilege('telmivo_runtime',c.oid,'SELECT') AS runtime_select,
  has_table_privilege('telmivo_runtime',c.oid,'INSERT') AS runtime_insert,
  has_table_privilege('telmivo_runtime',c.oid,'UPDATE') AS runtime_update,
  has_table_privilege('telmivo_runtime',c.oid,'DELETE') AS runtime_delete
FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
WHERE n.nspname='public'
  AND c.relname IN ('outbox_events','inbox_events','dead_letter_events')
ORDER BY c.relname;
