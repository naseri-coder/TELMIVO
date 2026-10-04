-- Native PostgreSQL resolves record fields before a CASE expression selects its arm.
CREATE OR REPLACE FUNCTION enforce_ledger_balance() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE tid uuid; n bigint; total numeric; bad boolean;
BEGIN
 IF TG_TABLE_NAME='ledger_transactions' THEN tid:=NEW.id; ELSE tid:=NEW.transaction_id; END IF;
 SELECT count(*),COALESCE(sum(amount_atomic),0) INTO n,total FROM ledger_entries WHERE transaction_id=tid;
 IF n<2 OR total<>0 THEN RAISE EXCEPTION 'unbalanced ledger transaction'; END IF;
 SELECT EXISTS(SELECT 1 FROM wallet_accounts a JOIN ledger_entries e ON e.account_id=a.id WHERE a.kind='WALLET' AND a.id IN(SELECT account_id FROM ledger_entries WHERE transaction_id=tid) GROUP BY a.id HAVING sum(e.amount_atomic)<0) INTO bad;
 IF bad THEN RAISE EXCEPTION 'insufficient wallet balance'; END IF;
 RETURN NULL;
END $$;
