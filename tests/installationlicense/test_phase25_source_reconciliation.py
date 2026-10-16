"""LIC-A9 Phase25 pinned GitHub source-graph policy tests (offline only).

This module checks a dated metadata snapshot. It DOES NOT fetch GitHub, execute
product tests, certify a merge, or release anything into Production.
"""
import copy
import unittest

PRODUCT_CHAIN = (
    (74, "a9e09c8e0029fa0d35e6e06ed6da0fe640c672b8", "d12a80d10575b3cef8ae9d6b824f89dfcdbf193f", 5),
    (75, "24ef50684148a2a434f0dbde8c81017c6c0e244c", "a9e09c8e0029fa0d35e6e06ed6da0fe640c672b8", 4),
    (76, "34a1459c3f44a7395f3044040d707c53644443fc", "24ef50684148a2a434f0dbde8c81017c6c0e244c", 6),
    (77, "255a55123015a0ad3d5efe1ab9a233e678e5db03", "34a1459c3f44a7395f3044040d707c53644443fc", 5),
)
AUTHORITY_CHAIN = (
    (55, "bfceaef06d41aa185677a796652d17b07979058a", "bce3490234224e6156c9fdfeaf28b43d189e767f", 3),
    (56, "d947d2829f9980117a1ef5dfd2082d4613151fa0", "bfceaef06d41aa185677a796652d17b07979058a", 2),
    (57, "d6960982e322d4458c7d963bb153f205a1f8fb59", "d947d2829f9980117a1ef5dfd2082d4613151fa0", 4),
)
PRODUCT_PHASE24_PATHS = frozenset((
    "tests/installationlicense/PHASE24_BROKER_OWNER_SHUTDOWN_HARDENING_20261016.md",
    "tests/installationlicense/phase23_broker_proof/broker.go",
    "tests/installationlicense/phase23_broker_proof/phase24_hardening_test.go",
    "tests/installationlicense/run_joint_postgres_e2e.py",
    "tests/installationlicense/test_phase08_refreeze.py",
))
AUTHORITY_PHASE21_PATHS = frozenset((
    "docs/release/LIC_A9_PHASE21_CROSSREPO_AUDIT_20261016.md",
    "docs/release/LIC_A9_PHASE21_SOURCE_GRAPH_SNAPSHOT_20261016.json",
    "tests/phase21_crossrepo_audit_test.py",
    "tools/lic-a9-phase21-crossrepo-audit",
))
EXPECTED_BLOCKERS = frozenset((
    "FULL_PRODUCT_GO127_NOT_RUN", "CROSS_REPO_PG16_E2E_NOT_RUN",
    "BROKER_RUNTIME_INTEGRATION_NOT_RUN", "INDEPENDENT_DR_NOT_VERIFIED",
    "SIGNED_INSTALL_NOT_RUN", "INDEPENDENT_REVIEW_PENDING",
))


def snapshot():
    return {
        "schema": "TELMIVO_LIC_A9_PHASE25_PINNED_SOURCE_GRAPH_V1",
        "product": {
            "main": "f8b7962b83760f780e1c96bc3b80872223dedaee",
            "merge_base": "a708193c75b06895c8d5bfdd8e8b585a93863a30",
            "ahead": 139, "behind": 2,
            "chain": [{"number": n, "head": h, "base": b, "paths": c,
                       "state": "open", "draft": True, "merged": False}
                      for n, h, b, c in PRODUCT_CHAIN],
            "phase24_paths": sorted(PRODUCT_PHASE24_PATHS),
            "main_only_commits": [
                "11cc000eb9e84de1ec6946ad133f674259fe3094",
                "f8b7962b83760f780e1c96bc3b80872223dedaee",
            ],
            "main_only_paths": ["tools/license-owner-offline-age-key"],
            "main_disposition": "DEFER_INDEPENDENT_OWNER_KEY_REVIEW",
        },
        "authority": {
            "main": "c078f55f55a0446d0a6ff49fb6bc0111a217ae42",
            "ahead": 166, "behind": 0,
            "chain": [{"number": n, "head": h, "base": b, "paths": c,
                       "state": "open", "draft": True, "merged": False}
                      for n, h, b, c in AUTHORITY_CHAIN],
            "phase21_paths": sorted(AUTHORITY_PHASE21_PATHS),
        },
        "blockers": sorted(EXPECTED_BLOCKERS),
        "release": "NO_GO", "main_merge_authorized": False,
        "production_deployment_authorized": False,
    }


class GraphRejected(ValueError):
    pass


def require(ok, tag):
    if not ok:
        raise GraphRejected(tag)


def validate(x):
    fixed = snapshot()
    require(isinstance(x, dict) and set(x) == set(fixed), "SCHEMA_KEYS")
    require(x.get("schema") == fixed["schema"], "SCHEMA_VERSION")
    for side, chain_def in (("product", PRODUCT_CHAIN), ("authority", AUTHORITY_CHAIN)):
        node = x[side]
        expect = fixed[side]
        require(isinstance(node, dict) and set(node) == set(expect), side + "_KEYS")
        for key in expect:
            if key != "chain":
                require(type(node[key]) is type(expect[key]) and node[key] == expect[key], side + "_" + key.upper())
        chain = node["chain"]
        require(isinstance(chain, list) and len(chain) == len(chain_def), side + "_CHAIN_LENGTH")
        for j, (num, head, base, paths) in enumerate(chain_def):
            row = chain[j]
            require(isinstance(row, dict) and set(row) == set(expect["chain"][j]), side + "_ROW_KEYS")
            require(row == {"number": num, "head": head, "base": base, "paths": paths,
                            "state": "open", "draft": True, "merged": False}, side + "_CHAIN_DRIFT")
            if j:
                require(chain[j-1]["head"] == row["base"], side + "_NONLINEAR")
    require(not set(x["product"]["main_only_paths"]) & set(x["product"]["phase24_paths"]), "MAIN_PATH_COLLISION")
    require(x["blockers"] == sorted(EXPECTED_BLOCKERS), "BLOCKER_TAMPER")
    require(x["release"] == "NO_GO", "RELEASE_UNSAFE")
    require(x["main_merge_authorized"] is False, "MERGE_UNSAFE")
    require(x["production_deployment_authorized"] is False, "DEPLOY_UNSAFE")
    return "PINNED_GRAPH_CONSISTENT_RELEASE_BLOCKED"


class Phase25PolicyTests(unittest.TestCase):
    def test_current_pin_is_consistent_but_not_release(self):
        self.assertEqual(validate(snapshot()), "PINNED_GRAPH_CONSISTENT_RELEASE_BLOCKED")

    def test_adversarial_changes_are_rejected(self):
        edits = [
            ("product-main", lambda x: x["product"].update(main="0" * 40)),
            ("product-base", lambda x: x["product"].update(merge_base="0" * 40)),
            ("ahead", lambda x: x["product"].update(ahead=138)),
            ("behind", lambda x: x["product"].update(behind=0)),
            ("reordered-main", lambda x: x["product"]["main_only_commits"].reverse()),
            ("main-tool-deleted", lambda x: x["product"].update(main_only_paths=[])),
            ("main-tool-other-file", lambda x: x["product"].update(main_only_paths=["internal/platform/installationlicense/state.go"])),
            ("main-auto-merge", lambda x: x["product"].update(main_disposition="AUTO_MERGE")),
            ("phase24-runtime-file", lambda x: x["product"]["phase24_paths"].append("internal/platform/installationlicense/state.go")),
            ("phase24-missing-test", lambda x: x["product"]["phase24_paths"].remove("tests/installationlicense/phase23_broker_proof/phase24_hardening_test.go")),
            ("phase24-sha", lambda x: x["product"]["chain"][3].update(head="0" * 40)),
            ("phase24-base", lambda x: x["product"]["chain"][3].update(base="0" * 40)),
            ("phase22-base", lambda x: x["product"]["chain"][1].update(base="0" * 40)),
            ("phase19-merged", lambda x: x["product"]["chain"][0].update(merged=True)),
            ("phase22-not-draft", lambda x: x["product"]["chain"][1].update(draft=False)),
            ("phase23-closed", lambda x: x["product"]["chain"][2].update(state="closed")),
            ("product-extra-pr", lambda x: x["product"]["chain"].append(copy.deepcopy(x["product"]["chain"][3]))),
            ("authority-main", lambda x: x["authority"].update(main="0" * 40)),
            ("authority-behind", lambda x: x["authority"].update(behind=1)),
            ("authority-pr57-sha", lambda x: x["authority"]["chain"][2].update(head="0" * 40)),
            ("authority-pr57-base", lambda x: x["authority"]["chain"][2].update(base="0" * 40)),
            ("authority-pr57-merged", lambda x: x["authority"]["chain"][2].update(merged=True)),
            ("authority-path-added", lambda x: x["authority"]["phase21_paths"].append("cmd/license-api/main.go")),
            ("release-go", lambda x: x.update(release="GO")),
            ("fake-go-tests", lambda x: x["blockers"].remove("FULL_PRODUCT_GO127_NOT_RUN")),
            ("fake-e2e", lambda x: x["blockers"].remove("CROSS_REPO_PG16_E2E_NOT_RUN")),
            ("merge-permission", lambda x: x.update(main_merge_authorized=True)),
            ("deploy-permission", lambda x: x.update(production_deployment_authorized=True)),
            ("schema-drift", lambda x: x.update(schema="V2")),
            ("unknown-field", lambda x: x.update(approval="GO")),
        ]
        for name, edit in edits:
            with self.subTest(name=name):
                x = snapshot()
                edit(x)
                with self.assertRaises(GraphRejected):
                    validate(x)


if __name__ == "__main__":
    unittest.main()
