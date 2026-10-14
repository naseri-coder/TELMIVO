"""Tests for fail-closed two-repository source pin and preflight orchestration.

All effects mocked: no Github, PostgreSQL, Go command or production host.
"""
import contextlib
import io
import pathlib
import subprocess
import tempfile
import unittest
from unittest import mock

import run_source_preflight as subject

PRODUCT_SHA = "a" * 40
OTHER_SHA = "b" * 40
ARGS = ["--authority", "/tmp/lic-a9-test-authority",
        "--product", "/tmp/lic-a9-test-product",
        "--expected-product-sha", PRODUCT_SHA]


def checked(authority=subject.AUTHORITY_SHA, product=PRODUCT_SHA,
            authority_clean=True, product_clean=True, command_error=False):
    """Execute the real argument and gate code with mocked system access."""
    with (mock.patch.object(subject, "repository_sha", side_effect=[authority, product]) as hashes,
          mock.patch.object(subject, "is_clean", side_effect=[authority_clean, product_clean]) as clean,
          mock.patch.object(subject, "run") as commands,
          contextlib.redirect_stderr(io.StringIO()),
          contextlib.redirect_stdout(io.StringIO())):
        if command_error:
            commands.side_effect = subprocess.CalledProcessError(1, ["go", "test"])
        status = subject.main(ARGS)
        return status, hashes.call_count, clean.call_count, commands.call_args_list


class SourcePreflightTests(unittest.TestCase):
    def test_valid_pinned_clean_sources_execute_all_six_steps(self):
        status, hashes, clean, commands = checked()
        self.assertEqual(status, 0)
        self.assertEqual((hashes, clean, len(commands)), (2, 2, 6))
        self.assertEqual(commands[-1].args[0][:2], ["go", "test"])
        self.assertTrue(all("postgres" not in repr(c).lower() for c in commands))

    def test_authority_mismatch_refuses_all_commands(self):
        status, _, _, commands = checked(authority=OTHER_SHA)
        self.assertEqual((status, len(commands)), (3, 0))

    def test_product_mismatch_refuses_all_commands(self):
        status, hashes, _, commands = checked(product=OTHER_SHA)
        self.assertEqual((status, hashes, len(commands)), (3, 2, 0))

    def test_dirty_authority_refuses_all_commands(self):
        status, _, _, commands = checked(authority_clean=False)
        self.assertEqual((status, len(commands)), (3, 0))

    def test_dirty_product_refuses_all_commands(self):
        status, _, clean, commands = checked(product_clean=False)
        self.assertEqual((status, clean, len(commands)), (3, 2, 0))

    def test_nonzero_child_test_refuses_success(self):
        status, _, _, commands = checked(command_error=True)
        self.assertEqual(status, 3)
        self.assertEqual(len(commands), 1)

    def test_bad_pin_is_rejected_before_git(self):
        with (mock.patch.object(subject, "repository_sha") as git,
              contextlib.redirect_stderr(io.StringIO())):
            result = subject.main(ARGS[:-1] + ["main"])
        self.assertEqual(result, 3)
        git.assert_not_called()

    def test_separate_checkout_required(self):
        params = ["--authority", "/tmp/same", "--product", "/tmp/same",
                  "--expected-product-sha", PRODUCT_SHA]
        with (mock.patch.object(subject, "repository_sha") as git,
              contextlib.redirect_stderr(io.StringIO())):
            self.assertEqual(subject.main(params), 3)
        git.assert_not_called()

    def test_repo_command_failure_is_not_pass(self):
        with (mock.patch.object(subject, "repository_sha",
                                side_effect=subprocess.CalledProcessError(1, ["git"])),
              contextlib.redirect_stderr(io.StringIO())):
            self.assertEqual(subject.main(ARGS), 3)

    def test_dirty_git_checkout_is_detected(self):
        with mock.patch.object(subject.subprocess, "run",
                               return_value=mock.Mock(stdout=" M source.go\n")):
            self.assertFalse(subject.is_clean(pathlib.Path("/tmp/no-repo")))

    def test_clean_git_checkout_is_detected(self):
        with mock.patch.object(subject.subprocess, "run",
                               return_value=mock.Mock(stdout="")):
            self.assertTrue(subject.is_clean(pathlib.Path("/tmp/no-repo")))


    def test_real_git_checkout_detects_edits_and_untracked(self):
        with tempfile.TemporaryDirectory(prefix="lic-a9-git-fixture-") as temp:
            folder = pathlib.Path(temp)
            subprocess.run(["git", "init", "-q", str(folder)], check=True)
            tracked = folder / "sample.txt"
            tracked.write_text("known contents\n")
            subprocess.run(["git", "-C", str(folder), "add", "sample.txt"], check=True)
            subprocess.run(["git", "-C", str(folder), "-c", "user.name=Test",
                            "-c", "user.email=test@example.invalid", "commit", "-qm", "fixture"],
                           check=True)
            self.assertRegex(subject.repository_sha(folder), r"^[0-9a-f]{40}$")
            self.assertTrue(subject.is_clean(folder))
            tracked.write_text("unexpected change\n")
            self.assertFalse(subject.is_clean(folder))
            tracked.write_text("known contents\n")
            self.assertTrue(subject.is_clean(folder))
            (folder / "not-tracked.txt").write_text("injected")
            self.assertFalse(subject.is_clean(folder))

if __name__ == "__main__":
    unittest.main()
