import fnmatch
import re
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def workflow(name):
    return (ROOT / '.github/workflows' / name).read_text()


def branches(source, event):
    match = re.search(r'^  ' + event + r':\n    branches: (\[[^\n]+\])', source, re.M)
    if not match:
        raise AssertionError('missing event branches: ' + event)
    # Branch lists contain unquoted GitHub branch names, not Python strings.
    return [value.strip().strip("'\"") for value in match[1][1:-1].split(',')]


class CICostContract(unittest.TestCase):
    def test_automatic_release_events_have_native_validation_in_ci_go(self):
        release = workflow('r4-release-readiness.yml')
        go = workflow('ci-go.yml')
        for event in ('push', 'pull_request'):
            targets = branches(go, event)
            for branch in branches(release, event):
                self.assertTrue(any(fnmatch.fnmatchcase(branch, pattern) for pattern in targets),
                                (event, branch))
        self.assertIn('  native:\n    uses: ./.github/workflows/migration-check.yml', go)

    def test_manual_release_audit_retains_complete_native_suite(self):
        release = workflow('r4-release-readiness.yml')
        self.assertIn('  workflow_dispatch:', release)
        self.assertRegex(release, r"  native:\n(?:    #[^\n]*\n)*    if: github.event_name == 'workflow_dispatch'\n    needs: static\n    uses: ./\.github/workflows/migration-check.yml")

    def test_test_artifacts_have_bounded_retention(self):
        for name, artifact in [('ci-web.yml', 'rtl-smoke'), ('migration-check.yml', 'native-results')]:
            self.assertIn('          name: ' + artifact + '\n          retention-days: 7', workflow(name))


if __name__ == '__main__':
    unittest.main()
