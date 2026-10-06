from pathlib import Path
import re
import unittest

ROOT = Path(__file__).resolve().parents[2]

class UploadProxyContractTests(unittest.TestCase):
    def test_binary_route_has_distinct_bounded_limit(self):
        nginx = (ROOT / 'deployments/reverse-proxy/nginx.production.conf.template').read_text()
        self.assertIn('client_max_body_size 2m;', nginx)
        self.assertRegex(nginx, r'location ~ \^/api/v1/uploader/uploads/\[\^/\]\+/content\$')
        self.assertIn('client_max_body_size 49m;', nginx)
        route = re.compile(r'^/api/v1/uploader/uploads/[^/]+/content$')
        self.assertIsNotNone(route.match('/api/v1/uploader/uploads/synthetic/content'))
        self.assertIsNone(route.match('/api/v1/uploader/uploads/synthetic'))

    def test_native_checker_path_resolves_from_workflow_directory(self):
        workflow = (ROOT / '.github/workflows/migration-check.yml').read_text()
        self.assertIn('python3 ../../scripts/ci/check-native-selection.py --enumerate', workflow)
        self.assertTrue((ROOT / 'web/miniapp/../../scripts/ci/check-native-selection.py').resolve().is_file())

if __name__ == '__main__':
    unittest.main()
