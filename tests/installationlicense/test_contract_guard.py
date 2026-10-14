"""Offline negative/positive tests for the cross-repository M2M source guard."""
import unittest
from contract_guard import AUTHORITY_SHA, LEGACY_SHA, check_source

DOC = "\n".join("/v1/licenses/" + s for s in ("activate", "refresh", "deactivate", "status")) + "\nTELMIVO_INSTALLATION_REQUEST_V1"
CLIENT = """
func proof() {
  b := []byte("TELMIVO_INSTALLATION_REQUEST_V1\\x00")
  for _, f := range [][]byte{[]byte(method), []byte(path), []byte(query), []byte(timestamp), []byte(nonce), []byte(key), []byte(i.ID), pk[:], bh[:]} {}
}
"X-Telmivo-Installation-ID" "X-Telmivo-Timestamp" "X-Telmivo-Nonce"
"X-Telmivo-Signature" "Idempotency-Key" "application/json"
"""
FIXTURE = ('sha = "' + AUTHORITY_SHA + '"\n'
           'archive = subprocess.run(["git", "-C", str(repo), "archive", sha], check=True)')


class ContractSourceGuardTests(unittest.TestCase):
    def test_correct_source_shape(self):
        self.assertEqual(check_source(DOC, CLIENT, FIXTURE), [])

    def test_reject_old_authority_pin(self):
        errors = check_source(DOC, CLIENT, FIXTURE.replace(AUTHORITY_SHA, LEGACY_SHA))
        self.assertTrue(any("legacy Authority" in error for error in errors))

    def test_reject_missing_route(self):
        self.assertTrue(any("status" in error for error in check_source(
            DOC.replace("/v1/licenses/status", ""), CLIENT, FIXTURE)))

    def test_reject_changed_proof_order(self):
        changed = CLIENT.replace("[]byte(nonce), []byte(key)", "[]byte(key), []byte(nonce)")
        self.assertTrue(any("fields changed" in error for error in check_source(DOC, changed, FIXTURE)))

    def test_reject_changed_domain(self):
        changed = CLIENT.replace("INSTALLATION_REQUEST_V1", "INSTALLATION_REQUEST_V2")
        self.assertTrue(any("namespace" in error for error in check_source(DOC, changed, FIXTURE)))

    def test_reject_missing_idempotency_header(self):
        changed = CLIENT.replace("Idempotency-Key", "Idempotency-Key-2")
        self.assertTrue(any("Idempotency-Key" in error for error in check_source(DOC, changed, FIXTURE)))


if __name__ == "__main__":
    unittest.main()
