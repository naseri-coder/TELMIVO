-- Version 1: canonical counts/content identities for all authoritative tables.
CREATE TEMP TABLE recovery_manifest(name text, rows bigint, digest text);
DO $$ DECLARE t record; BEGIN
 FOR t IN SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY tablename LOOP
 EXECUTE format('INSERT INTO recovery_manifest SELECT %L,count(*),md5(COALESCE(string_agg(to_jsonb(r)::text,E''\n'' ORDER BY to_jsonb(r)::text),'''')) FROM %I r',t.tablename,t.tablename);
 END LOOP;
END $$;
SELECT name,rows,digest FROM recovery_manifest ORDER BY name;
-- Ledger balance is checked independently of snapshot equality.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM ledger_entries e JOIN ledger_transactions t ON t.id=e.transaction_id GROUP BY t.id HAVING sum(e.amount_atomic)<>0) THEN RAISE EXCEPTION 'unbalanced restored ledger'; END IF;
END $$;
