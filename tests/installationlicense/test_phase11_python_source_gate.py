"""Standalone negative tests of source-integrity gate, without Docker or Go."""
import importlib.util
from pathlib import Path
import json
import tempfile
import unittest


MODFILE = Path(__file__).with_name('phase11_python_source_gate.py')
SPEC = importlib.util.spec_from_file_location('lic_a9_phase11_gate', MODFILE)
gate = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(gate)


class Phase11SourceGateTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='lic-a9-phase11-syntax-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        for name in gate.REQUIRED:
            (self.root / name).write_text('value = 1\n', encoding='utf-8')
        self.runner = self.root / gate.RUNNER
        self.runner.write_text("def main():\n    return 0\n"
                               "if __name__ == '__main__':\n"
                               "    raise SystemExit(main())\n")

    def test_good_source_inventory_is_syntax_only(self):
        result = gate.audit(self.root)
        self.assertEqual(result['verdict'], 'SYNTAX_AND_RUNNER_ENTRYPOINT_PASS_ONLY')
        self.assertEqual(result['inspected_files'], len(gate.REQUIRED))
        self.assertFalse(result['go_pg16_e2e_executed'])
        self.assertFalse(result['production_authorized'])

    def test_main_guard_that_only_passes_is_denied(self):
        self.runner.write_text("def main():\n    return 0\n"
                               "if __name__ == '__main__':\n    pass\n")
        with self.assertRaisesRegex(gate.SourceDenied, 'RUNNER_MAIN_ENTRYPOINT_NOT_EXECUTABLE'):
            gate.audit(self.root)

    def test_main_guard_that_only_exits_zero_is_denied(self):
        self.runner.write_text("def main():\n    return 0\n"
                               "if __name__ == '__main__':\n    raise SystemExit(0)\n")
        with self.assertRaisesRegex(gate.SourceDenied, 'RUNNER_MAIN_ENTRYPOINT_NOT_EXECUTABLE'):
            gate.audit(self.root)

    def test_main_guard_wrong_function_name_denied(self):
        self.runner.write_text("def main():\n    return 0\n"
                               "if __name__ == '__main__':\n    raise SystemExit(skip_checks())\n")
        with self.assertRaisesRegex(gate.SourceDenied, 'RUNNER_MAIN_ENTRYPOINT_NOT_EXECUTABLE'):
            gate.audit(self.root)

    def test_main_guard_additional_statement_denied(self):
        self.runner.write_text("def main():\n    return 0\n"
                               "if __name__ == '__main__':\n"
                               "    log_ahead()\n    raise SystemExit(main())\n")
        with self.assertRaisesRegex(gate.SourceDenied, 'RUNNER_MAIN_ENTRYPOINT_NOT_EXECUTABLE'):
            gate.audit(self.root)

    def test_main_guard_else_branch_denied(self):
        self.runner.write_text("def main():\n    return 0\n"
                               "if __name__ == '__main__':\n"
                               "    raise SystemExit(main())\nelse:\n    unapproved()\n")
        with self.assertRaisesRegex(gate.SourceDenied, 'RUNNER_MAIN_ENTRYPOINT_NOT_EXECUTABLE'):
            gate.audit(self.root)

    def test_main_guard_calls_main_with_fake_args_denied(self):
        self.runner.write_text("def main():\n    return 0\n"
                               "if __name__ == '__main__':\n"
                               "    raise SystemExit(main('--skip-e2e'))\n")
        with self.assertRaisesRegex(gate.SourceDenied, 'RUNNER_MAIN_ENTRYPOINT_NOT_EXECUTABLE'):
            gate.audit(self.root)

    def test_no_main_definition_denied(self):
        self.runner.write_text("if __name__ == '__main__':\n"
                               "    raise SystemExit(main())\n")
        with self.assertRaisesRegex(gate.SourceDenied, 'RUNNER_MAIN_ENTRYPOINT_NOT_EXECUTABLE'):
            gate.audit(self.root)

    def test_stray_duplicate_invalid_tail_is_rejected(self):
        with self.runner.open('a') as f:
            f.write(", './authority'],\n    image=go_image)\n")
        with self.assertRaisesRegex(gate.SourceDenied, 'PYTHON_SYNTAX_INVALID'):
            gate.audit(self.root)

    def test_syntactically_valid_post_main_statements_are_rejected(self):
        with self.runner.open('a') as f:
            f.write('unapproved_operation()\n')
        with self.assertRaisesRegex(gate.SourceDenied, 'RUNNER_MAIN_GUARD_NOT_UNIQUE_OR_TERMINAL'):
            gate.audit(self.root)

    def test_duplicate_main_guards_are_rejected(self):
        with self.runner.open('a') as f:
            f.write("if __name__ == '__main__':\n    value = 3\n")
        with self.assertRaisesRegex(gate.SourceDenied, 'RUNNER_MAIN_GUARD_NOT_UNIQUE_OR_TERMINAL'):
            gate.audit(self.root)

    def test_missing_main_guard_is_rejected(self):
        self.runner.write_text('value = 2\n')
        with self.assertRaisesRegex(gate.SourceDenied, 'RUNNER_MAIN_GUARD_NOT_UNIQUE_OR_TERMINAL'):
            gate.audit(self.root)

    def test_unrelated_test_file_syntax_is_checked(self):
        (self.root / 'test_joint_runner.py').write_text('def broken(:\n')
        with self.assertRaisesRegex(gate.SourceDenied, 'PYTHON_SYNTAX_INVALID:test_joint_runner.py'):
            gate.audit(self.root)

    def test_new_unlisted_python_file_also_checked(self):
        (self.root / 'other_test.py').write_text('class Invalid(\n')
        with self.assertRaisesRegex(gate.SourceDenied, 'PYTHON_SYNTAX_INVALID:other_test.py'):
            gate.audit(self.root)

    def test_missing_required_file_rejected(self):
        (self.root / 'prepare_authority_fixture.py').unlink()
        with self.assertRaisesRegex(gate.SourceDenied, 'PYTHON_SOURCE_INVENTORY_INCOMPLETE'):
            gate.audit(self.root)

    def test_symlink_source_file_rejected(self):
        other = self.root / 'real.py'
        other.write_text('pass\n')
        alias = self.root / 'test_phase08_refreeze.py'
        alias.unlink()
        alias.symlink_to(other)
        with self.assertRaisesRegex(gate.SourceDenied, 'PYTHON_SOURCE_PATH_UNSAFE'):
            gate.audit(self.root)

    def test_missing_directory_rejected(self):
        with self.assertRaisesRegex(gate.SourceDenied, 'SOURCE_DIRECTORY_UNSAFE_OR_MISSING'):
            gate.audit(self.root / 'missing')

    def test_non_ascii_unicode_valid_python(self):
        (self.root / 'test_joint_runner.py').write_text("نام = 'آزمون'\n", encoding='utf-8')
        self.assertEqual(gate.audit(self.root)['inspected_files'], len(gate.REQUIRED))

    def test_cli_success_in_synthetic_fixture(self):
        self.assertEqual(gate.main(['--test-source-dir', str(self.root)]), 0)

    def test_cli_failure_does_not_authorize_release(self):
        self.runner.write_text('def invalid(:\n')
        self.assertEqual(gate.main(['--test-source-dir', str(self.root)]), 3)


if __name__ == '__main__':
    unittest.main()
