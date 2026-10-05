-- Entry sets are sealed at commit: even balanced pairs cannot amend an old posting.
CREATE OR REPLACE FUNCTION lock_ledger_account() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM ledger_transactions WHERE id=NEW.transaction_id AND xmin::text=pg_current_xact_id()::text) THEN
  RAISE EXCEPTION 'posted ledger entry set is sealed';
 END IF;
 PERFORM 1 FROM wallet_accounts WHERE id=NEW.account_id FOR UPDATE;
 RETURN NEW;
END $$;
CREATE TRIGGER wallet_accounts_no_mutation BEFORE UPDATE OR DELETE ON wallet_accounts FOR EACH ROW EXECUTE FUNCTION reject_mutation();
