import unittest
from go_toolchain_gate import assess, installed_version, required_version

class ToolchainGateTests(unittest.TestCase):
    def test_parse_declared(self):
        self.assertEqual(required_version('module a\n\ngo 1.27.0\n'), (1,27))
    def test_old_go_rejected(self):
        good, msg = assess('go 1.27.0\n','go version go1.23.2 linux/amd64\n')
        self.assertFalse(good)
        self.assertIn('BLOCKED',msg)
    def test_equal_go_accepted(self):
        self.assertTrue(assess('go 1.27.0\n','go version go1.27.0 linux/amd64\n')[0])
    def test_future_go_accepted(self):
        self.assertTrue(assess('go 1.27.0\n','go version go1.28.1 linux/amd64\n')[0])
    def test_missing_declared_fails(self):
        with self.assertRaises(ValueError): required_version('module x\n')
    def test_duplicate_declared_fails(self):
        with self.assertRaises(ValueError): required_version('go 1.27\ngo 1.26\n')
    def test_corrupt_version_fails(self):
        with self.assertRaises(ValueError): installed_version('go version unknown')

if __name__ == '__main__': unittest.main()
