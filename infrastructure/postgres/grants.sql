GRANT USAGE ON SCHEMA public TO telmivo_runtime;
GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA public TO telmivo_runtime;
REVOKE ALL ON schema_migrations FROM telmivo_runtime;
REVOKE INSERT,UPDATE,DELETE ON infrastructure_safety_caps FROM telmivo_runtime;
GRANT EXECUTE ON FUNCTION platform_plan_in_use(uuid,text) TO telmivo_runtime;

REVOKE ALL ON FUNCTION telegram_group_route(bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION telegram_group_route(bigint) TO telmivo_runtime;
