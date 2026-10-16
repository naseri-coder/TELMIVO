"""Phase26 design-only contract tests. No socket, Go, DB or real effect is executed."""
import copy
import json
import unittest
from pathlib import Path

P = Path(__file__).with_name("phase26_single_effect_owner_contract.json")

class Rejected(ValueError):
    pass

def decode(raw):
    def unique(pairs):
        d = {}
        for k, v in pairs:
            if k in d:
                raise Rejected("DUPLICATE_JSON_KEY")
            d[k] = v
        return d
    return json.loads(raw, object_pairs_hook=unique)

def require(ok, msg):
    if not ok:
        raise Rejected(msg)

def validate(x):
    require(x["schema"] == "TELMIVO_LIC_A9_PHASE26_SINGLE_EFFECT_OWNER_CONTRACT_V1", "SCHEMA")
    require(x["scope"] == "DESIGN_ONLY_NONPRODUCTION", "SCOPE")
    require(x["choice"] == "ONE_LICENSED_EFFECT_OWNER_PER_INSTALLATION_WITH_AUTHENTICATED_LOCAL_COMMAND_DISPATCH", "CHOICE")
    require(x["source_pins"]["product_parent"] == "153021a4ecf19686075633b10fddf6d3ff0f3299", "PRODUCT_PIN")
    require(x["source_pins"]["authority_review"] == "d6960982e322d4458c7d963bb153f205a1f8fb59", "AUTHORITY_PIN")
    a = x["architecture"]
    for key, val in {
        "effect_owner_count": 1,
        "worker_may_hold_lease": False,
        "worker_may_hold_installation_private_key": False,
        "worker_may_call_manager_effect": False,
        "effect_owner_must_use_real_manager_effect": True,
        "shared_lease_bypass": False,
        "remote_tcp_listener": False,
        "peer_uid_alone_is_authentication": False,
        "untyped_callbacks": False,
        "allowed_transport": "UNIX_DOMAIN_SOCKET_ONLY",
        "owner_instance_on_restart": "NEW_RUNTIME_SESSION_AND_FRESH_AUTHORITY_SIGNED_LEASE",
    }.items():
        require(type(a.get(key)) is type(val) and a[key] == val, "ARCHITECTURE_" + key)
    iso = x["isolation"]
    for key, val in {
        "separate_worker_unix_identities": True,
        "shared_writable_owner_directory": False,
        "fd_relative_path_resolution_required": True,
        "symlink_ancestry_accepted": False,
        "peer_os_credentials_required": True,
        "client_key_pop_required": True,
        "server_side_worker_identity_map": True,
    }.items():
        require(iso.get(key) is val, "ISOLATION_" + key)
    req = x["request"]
    require(req["protocol"] == "TELMIVO_LIC_A9_EFFECT_DISPATCH_V1", "IPC_PROTOCOL")
    require(req["max_frame_bytes"] == 16384 and type(req["max_frame_bytes"]) is int, "FRAME_LIMIT")
    require(req["canonical_strict_json"] is True, "CANONICAL_JSON")
    for key in ("duplicate_json_fields", "unknown_json_fields", "nonce_reuse", "arbitrary_shell_or_callback"):
        require(req[key] == "DENY", "IPC_" + key)
    require(req["operations"] == "FINITE_VERSIONED_ALLOWLIST", "OPERATIONS")
    require(req["authentication"] == "PER_WORKER_PROOF_OF_POSSESSION", "WORKER_AUTH")
    fields = {"protocol", "request_id", "worker_id", "installation_id", "workspace_id", "actor_id",
              "operation", "resource_id", "payload_sha256", "nonce", "expires_at_unix_ms", "authentication"}
    require(set(req["required_fields"]) == fields and len(req["required_fields"]) == len(fields), "REQUIRED_FIELDS")
    gates = {"PEER_IDENTITY", "WORKER_PROOF_OF_POSSESSION", "ENROLLED_WORKER_ROLE", "TENANT_WORKSPACE_BINDING",
             "ACTOR_RBAC", "PLAN_AND_PLUGIN_ENTITLEMENT", "DOMAIN_QUOTAS", "DOMAIN_RESOURCE_POLICY",
             "FRESH_SIGNED_OWNER_SESSION", "REAL_MANAGER_EFFECT_AT_FINAL_EFFECT_POINT",
             "DURABLE_INTENT_AND_OUTCOME_POLICY", "CURRENT_OWNER_FENCING_TOKEN"}
    require(set(x["gates"]) == gates and len(x["gates"]) == len(gates), "EFFECT_GATES")
    life = x["lifecycle"]
    states = set(life["states"])
    edges = {tuple(edge) for edge in life["transitions"]}
    require(states == {"STARTING", "ACTIVE", "DRAINING", "FENCED", "RECOVERING"}, "STATES")
    require(edges.issubset({(a, b) for a in states for b in states}), "EDGE_UNKNOWN")
    require(("FENCED", "ACTIVE") not in edges, "FENCED_BYPASS")
    require(("FENCED", "RECOVERING") in edges and ("RECOVERING", "ACTIVE") in edges, "NO_RECOVERY")
    for key, expected in {
        "allow_active_without_signed_session": False,
        "leader_takeover_requires_new_signed_lease": True,
        "lease_generation_must_increase": True,
        "stale_owner_effect": "DENY",
        "blocked_follower_direct_effect": "DENY",
    }.items():
        require(life[key] == expected, "LIFECYCLE_" + key)
    eff = x["effects"]
    for key, expected in {
        "durable_intent_before_external_effect": True,
        "domain_ledger_required": True,
        "idempotency_key_required": True,
        "ambiguous_result": "RECONCILE_REQUIRED",
        "ambiguous_automatic_retry": False,
        "exactly_once_claim": False,
        "fencing_epoch_check_before_effect": True,
        "unsupported_provider": "PAUSE_AND_RECONCILE_BEFORE_NEW_EFFECTS",
    }.items():
        require(eff[key] == expected, "EFFECT_" + key)
    require(all(v is False for v in x["permissions"].values()), "OPERATIONAL_PERMISSION")
    require(x["evidence"] == {
        "real_product_go127": "NOT_RUN", "real_crossrepo_pg16": "NOT_RUN",
        "multiworker_live_integration": "NOT_RUN", "broker_security_qualified": "NOT_RUN",
        "recovery": "NOT_VERIFIED", "release": "NO_GO",
    }, "FALSE_EVIDENCE")
    return "MODEL_CONSISTENT_RELEASE_BLOCKED"

def idealized_admission(**change):
    """Synthetic intended gate model, NOT an implementation of signed verification."""
    flags = {k: True for k in ("peer", "worker_pop", "worker_enrolled", "tenant", "rbac",
        "entitlement", "quota", "domain", "signed_session", "owner_epoch", "active_owner",
        "final_manager_effect", "durable_intent", "allowlisted_operation")}
    flags.update(change)
    for name in flags:
        if name in change and change[name] is not True:
            return "DENY_" + name
    if not all(flags.values()):
        return "DENY"
    return "ADMISSIBLE_IN_SYNTHETIC_MODEL_ONLY"

class Phase26Tests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.original = decode(P.read_text(encoding="utf-8"))

    def test_baseline_is_no_go(self):
        self.assertEqual(validate(self.original), "MODEL_CONSISTENT_RELEASE_BLOCKED")

    def test_prohibited_architecture_relaxations(self):
        cases = [
            ("architecture", "effect_owner_count", 2),
            ("architecture", "shared_lease_bypass", True),
            ("architecture", "worker_may_hold_lease", True),
            ("architecture", "worker_may_hold_installation_private_key", True),
            ("architecture", "worker_may_call_manager_effect", True),
            ("architecture", "effect_owner_must_use_real_manager_effect", False),
            ("architecture", "allowed_transport", "TCP"),
            ("architecture", "peer_uid_alone_is_authentication", True),
            ("architecture", "untyped_callbacks", True),
            ("isolation", "shared_writable_owner_directory", True),
            ("isolation", "fd_relative_path_resolution_required", False),
            ("isolation", "client_key_pop_required", False),
            ("request", "nonce_reuse", "ALLOW"),
            ("request", "unknown_json_fields", "ALLOW"),
            ("request", "arbitrary_shell_or_callback", "ALLOW"),
            ("lifecycle", "allow_active_without_signed_session", True),
            ("lifecycle", "leader_takeover_requires_new_signed_lease", False),
            ("lifecycle", "stale_owner_effect", "ALLOW"),
            ("effects", "ambiguous_automatic_retry", True),
            ("effects", "exactly_once_claim", True),
            ("effects", "durable_intent_before_external_effect", False),
            ("effects", "unsupported_provider", "RUN_ANYWAY"),
        ]
        for section, key, value in cases:
            with self.subTest(section=section, key=key):
                x = copy.deepcopy(self.original)
                x[section][key] = value
                with self.assertRaises(Rejected):
                    validate(x)

    def test_metadata_and_fake_pass_rejected(self):
        cases = [
            lambda x: x["source_pins"].update(product_parent="0"*40),
            lambda x: x["source_pins"].update(authority_review="0"*40),
            lambda x: x["permissions"].update(merge_main=True),
            lambda x: x["permissions"].update(deploy=True),
            lambda x: x["evidence"].update(real_product_go127="PASS"),
            lambda x: x["evidence"].update(real_crossrepo_pg16="PASS"),
            lambda x: x["evidence"].update(release="GO"),
            lambda x: x["gates"].remove("ACTOR_RBAC"),
            lambda x: x["gates"].remove("FRESH_SIGNED_OWNER_SESSION"),
            lambda x: x["request"]["required_fields"].remove("workspace_id"),
            lambda x: x["lifecycle"]["transitions"].append(["FENCED", "ACTIVE"]),
            lambda x: x["request"].update(max_frame_bytes=65536),
        ]
        for j, fn in enumerate(cases):
            with self.subTest(index=j):
                x = copy.deepcopy(self.original)
                fn(x)
                with self.assertRaises(Rejected):
                    validate(x)

    def test_duplicate_json_key_rejected(self):
        with self.assertRaises(Rejected):
            decode('{"schema":"ok","schema":"forged"}')

    def test_idealized_model_rejects_individual_failed_gates(self):
        for k in ("peer", "worker_pop", "worker_enrolled", "tenant", "rbac", "entitlement",
                  "quota", "domain", "signed_session", "owner_epoch", "active_owner",
                  "final_manager_effect", "durable_intent", "allowlisted_operation"):
            with self.subTest(gate=k):
                self.assertEqual(idealized_admission(**{k: False}), "DENY_" + k)

    def test_idealized_model_admission_is_not_runtime_pass(self):
        self.assertEqual(idealized_admission(), "ADMISSIBLE_IN_SYNTHETIC_MODEL_ONLY")

if __name__ == "__main__":
    unittest.main()
