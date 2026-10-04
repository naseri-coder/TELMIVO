-- Existing catalog controls participate in policy revision audit without changing prior migrations.
ALTER TABLE plugins ADD COLUMN configuration_version bigint NOT NULL DEFAULT 1 CHECK(configuration_version>0);
ALTER TABLE feature_flags ADD COLUMN configuration_version bigint NOT NULL DEFAULT 1 CHECK(configuration_version>0);
ALTER TABLE workspace_entitlement_overrides ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK(version>0);
ALTER TABLE temporary_grants ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK(version>0);
-- Reservation snapshots and terminal history cannot be edited into a new operation.
CREATE FUNCTION guard_quota_reservation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'durable reservation history'; END IF;
 IF OLD.state<>'RESERVED' OR NEW.state NOT IN('COMMITTED','RELEASED') OR
    (to_jsonb(OLD)-'state'-'settled_at')<>(to_jsonb(NEW)-'state'-'settled_at') OR NEW.settled_at IS NULL
 THEN RAISE EXCEPTION 'invalid reservation mutation'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER quota_reservation_history BEFORE UPDATE OR DELETE ON quota_reservations FOR EACH ROW EXECUTE FUNCTION guard_quota_reservation();
