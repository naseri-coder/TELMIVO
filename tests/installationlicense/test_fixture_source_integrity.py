"""Detect corrupted fixture preparer and out-of-date pinned Authority source."""
import ast
import pathlib
import unittest


class FixtureIntegrityTests(unittest.TestCase):
    def test_fixture_python_syntax(self):
        path = pathlib.Path(__file__).with_name("prepare_authority_fixture.py")
        ast.parse(path.read_text(), filename=str(path))

    def test_fixture_pins_selected_source_once(self):
        path = pathlib.Path(__file__).with_name("prepare_authority_fixture.py")
        module = ast.parse(path.read_text())
        assignments = [node for node in module.body if isinstance(node, ast.Assign)
                       and any(isinstance(target, ast.Name) and target.id == "sha"
                               for target in node.targets)]
        self.assertEqual(len(assignments), 1)
        self.assertEqual(ast.literal_eval(assignments[0].value),
                         "68cdb6996f85981c36dd20ecb7435ec010351e9b")

    def test_no_fixture_execution_on_import(self):
        path = pathlib.Path(__file__).with_name("prepare_authority_fixture.py")
        tree = ast.parse(path.read_text())
        self.assertTrue(any(isinstance(node, ast.If)
                            and isinstance(node.test, ast.Compare)
                            and isinstance(node.test.left, ast.Name)
                            and node.test.left.id == "__name__"
                            for node in tree.body))


if __name__ == "__main__":
    unittest.main()
