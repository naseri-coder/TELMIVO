"""Actual local Python tests of immutable image/commit provenance controls.

These tests do not have Docker or PostgreSQL and must never be described as
real Go+PG16 integration tests.
"""
import subprocess
import unittest
from unittest import mock

import phase10_image_identity as source

GO = 'sha256:' + 'a' * 64
PG = 'sha256:' + 'b' * 64
HEAD = 'c' * 40


class ImageIdentityTests(unittest.TestCase):
    def test_canonical_approved_image_ids(self):
        self.assertEqual(source.reviewed_image_id(GO), GO)
        self.assertEqual(source.reviewed_image_id(PG), PG)

    def test_non_sha_or_uppercase_refused(self):
        for invalid in (None, 0, True, '', 'sha256:' + 'A' * 64,
                        'sha256:' + 'a' * 63, 'sha256:' + 'a' * 65,
                        'postgres:16', 'golang:1.27.2-bookworm',
                        'sha256:' + 'a' * 64 + '\n', 'sha256:' + 'a' * 64 + ';rm -rf /'):
            with self.subTest(invalid=invalid), self.assertRaises(ValueError):
                source.reviewed_image_id(invalid)

    def test_canonical_reviewed_product_commit(self):
        self.assertEqual(source.reviewed_product_head(HEAD), HEAD)

    def test_noncommit_product_refs_refused(self):
        for invalid in (None, False, '', 'main', 'latest', 'c' * 39,
                        'c' * 41, 'C' * 40, 'c' * 40 + '\n', 'sha256:' + 'c'*64):
            with self.subTest(invalid=invalid), self.assertRaises(ValueError):
                source.reviewed_product_head(invalid)

    def test_correct_go_local_id(self):
        execute = mock.Mock(return_value=subprocess.CompletedProcess([], 0, stdout=GO+'\n'))
        self.assertEqual(source.inspect_pinned_image(source.GO_TAG, GO, execute), GO)
        execute.assert_called_once_with(
            ['docker', 'image', 'inspect', '--format', '{{.Id}}', source.GO_TAG],
            output=True, timeout=30)

    def test_correct_postgres_local_id(self):
        execute = mock.Mock(return_value=subprocess.CompletedProcess([], 0, stdout=PG))
        self.assertEqual(source.inspect_pinned_image(source.POSTGRES_TAG, PG, execute), PG)

    def test_modified_tag_refused(self):
        execute = mock.Mock()
        with self.assertRaises(ValueError):
            source.inspect_pinned_image('postgres:latest', PG, execute)
        execute.assert_not_called()

    def test_wrong_installed_id_refused(self):
        execute = mock.Mock(return_value=subprocess.CompletedProcess([], 0, stdout=GO))
        with self.assertRaisesRegex(ValueError, 'does not match'):
            source.inspect_pinned_image(source.POSTGRES_TAG, PG, execute)

    def test_unavailable_image_output_refused(self):
        for output in ('', None, 'sha256:' + 'f' * 64, GO + '\n' + GO):
            with self.subTest(output=output), self.assertRaises(ValueError):
                source.inspect_pinned_image(source.GO_TAG, GO,
                    mock.Mock(return_value=subprocess.CompletedProcess([], 0, stdout=output)))

    def test_nonstring_untrusted_image_id_never_invokes_docker(self):
        execute = mock.Mock()
        with self.assertRaises(ValueError):
            source.inspect_pinned_image(source.GO_TAG, ['sha256', GO], execute)
        execute.assert_not_called()

    def test_inspect_called_as_argv_not_shell(self):
        argv = []
        def execute(cmd, **kwargs):
            argv.append((cmd, kwargs))
            return subprocess.CompletedProcess(cmd, 0, stdout=GO)
        source.inspect_pinned_image(source.GO_TAG, GO, execute)
        self.assertEqual(len(argv), 1)
        self.assertEqual(argv[0][0][:3], ['docker', 'image', 'inspect'])
        self.assertNotIn('shell', argv[0][1])

    def test_image_absence_raises_without_implicit_pull(self):
        execute = mock.Mock(side_effect=subprocess.CalledProcessError(1, ['docker', 'image', 'inspect']))
        with self.assertRaises(subprocess.CalledProcessError):
            source.inspect_pinned_image(source.GO_TAG, GO, execute)
        self.assertEqual(execute.call_count, 1)


if __name__ == '__main__':
    unittest.main()
