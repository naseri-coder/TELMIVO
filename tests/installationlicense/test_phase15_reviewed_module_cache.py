"""Tests of exactly staged module bytes, with zero Go/Docker/network interaction."""
import os
from pathlib import Path
import tempfile
import unittest
from unittest import mock

from phase15_reviewed_module_cache import (
    approved_module_cache_digest, reviewed_module_pin,
    stage_approved_module_cache,
)


class ReviewedModuleCacheTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix='lic-a9-phase15-')
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.cache = self.root / 'reviewed'
        self.cache.mkdir(mode=0o700)
        dep = self.cache / 'github.com' / 'jackc' / 'pgx' / 'v5@v5.11.0'
        dep.mkdir(parents=True)
        (dep / 'go.mod').write_text('module github.com/jackc/pgx/v5\n')
        (dep / 'pgx.go').write_text('package pgx\n')
        self.stage = self.root / 'go_cache' / 'mod'
        self.stage.parent.mkdir(mode=0o700)
        self.pin = approved_module_cache_digest(self.cache)

    def test_valid_digest_format(self):
        self.assertEqual(reviewed_module_pin(self.pin), self.pin)
        self.assertTrue(self.pin.startswith('sha256:'))
        self.assertEqual(len(self.pin), 71)

    def test_correct_preapproved_cache_is_staged(self):
        report = stage_approved_module_cache(self.cache, self.stage, self.pin)
        self.assertEqual(report['pin'], self.pin)
        self.assertEqual(report['staged_file_count'], 2)
        self.assertGreater(report['staged_bytes'], 0)
        self.assertFalse(report['network_used'])
        self.assertFalse(report['production_accessed'])
        self.assertEqual(self.pin, approved_module_cache_digest(self.stage))
        self.assertTrue((self.stage / 'github.com/jackc/pgx/v5@v5.11.0/go.mod').exists())

    def test_missing_approved_pin_fails_before_copy(self):
        for bad in ('', None, 0, False, 'main', 'sha256:' + 'A'*64,
                    'sha256:'+'a'*63, 'sha256:'+'a'*65, 'sha256:'+'a'*64+'\n'):
            with self.subTest(bad=bad), self.assertRaises(ValueError):
                stage_approved_module_cache(self.cache, self.stage, bad)
            self.assertFalse(self.stage.exists())

    def test_wrong_pin_refuses_and_does_not_copy(self):
        with self.assertRaisesRegex(ValueError, 'unapproved or mutated'):
            stage_approved_module_cache(self.cache, self.stage, 'sha256:'+'f'*64)
        self.assertFalse(self.stage.exists())

    def test_tampered_module_rejects_original_pin(self):
        (self.cache / 'github.com/jackc/pgx/v5@v5.11.0/go.mod').write_text('malicious revision')
        with self.assertRaises(ValueError):
            stage_approved_module_cache(self.cache, self.stage, self.pin)
        self.assertFalse(self.stage.exists())

    def test_file_path_and_contents_both_authenticated(self):
        path = self.cache / 'github.com/jackc/pgx/v5@v5.11.0/pgx.go'
        path.rename(path.with_name('pgx2.go'))
        self.assertNotEqual(self.pin, approved_module_cache_digest(self.cache))

    def test_scan_is_independent_of_creation_order(self):
        other = self.root / 'other'
        (other / 'github.com/jackc/pgx/v5@v5.11.0').mkdir(parents=True)
        for name in ('pgx.go', 'go.mod'):
            p = self.cache / 'github.com/jackc/pgx/v5@v5.11.0' / name
            (other / 'github.com/jackc/pgx/v5@v5.11.0' / name).write_bytes(p.read_bytes())
        self.assertEqual(self.pin, approved_module_cache_digest(other))

    def test_symlinked_module_rejected_without_reading_target(self):
        external = self.root / 'sensitive-test-only'
        external.write_text('DO NOT COPY')
        alias = self.cache / 'secret_alias.mod'
        alias.symlink_to(external)
        with self.assertRaises(ValueError):
            approved_module_cache_digest(self.cache)
        self.assertFalse(self.stage.exists())

    def test_symlinked_directory_rejected(self):
        (self.cache / 'extra').symlink_to(self.root, target_is_directory=True)
        with self.assertRaises(ValueError):
            approved_module_cache_digest(self.cache)

    def test_hardlink_rejected(self):
        p = self.cache / 'github.com/jackc/pgx/v5@v5.11.0/go.mod'
        os.link(p, self.cache / 'alias.mod')
        with self.assertRaises(ValueError):
            approved_module_cache_digest(self.cache)

    def test_unprotected_source_directory_rejected(self):
        self.cache.chmod(0o777)
        with self.assertRaises(ValueError):
            stage_approved_module_cache(self.cache, self.stage, self.pin)

    def test_unprotected_source_file_rejected(self):
        (self.cache / 'github.com/jackc/pgx/v5@v5.11.0/go.mod').chmod(0o666)
        with self.assertRaises(ValueError):
            stage_approved_module_cache(self.cache, self.stage, self.pin)

    def test_relative_source_directory_rejected(self):
        with self.assertRaises(ValueError):
            approved_module_cache_digest(Path('untrusted'))

    def test_source_equal_destination_rejected(self):
        with self.assertRaises(ValueError):
            stage_approved_module_cache(self.cache, self.cache, self.pin)

    def test_destination_inside_source_rejected(self):
        with self.assertRaises(ValueError):
            stage_approved_module_cache(self.cache, self.cache / 'new', self.pin)

    def test_destination_must_not_exist(self):
        self.stage.mkdir()
        with self.assertRaises(ValueError):
            stage_approved_module_cache(self.cache, self.stage, self.pin)

    def test_source_cache_not_mutated_by_copy(self):
        before = {str(p.relative_to(self.cache)): p.read_bytes()
                  for p in self.cache.rglob('*') if p.is_file()}
        stage_approved_module_cache(self.cache, self.stage, self.pin)
        after = {str(p.relative_to(self.cache)): p.read_bytes()
                 for p in self.cache.rglob('*') if p.is_file()}
        self.assertEqual(before, after)

    def test_disconnected_side_effects_no_subprocess(self):
        with mock.patch('subprocess.run', side_effect=AssertionError('unexpected network/process')):
            stage_approved_module_cache(self.cache, self.stage, self.pin)

    def test_empty_cache_rejected(self):
        empty = self.root / 'empty'
        empty.mkdir()
        with self.assertRaises(ValueError):
            approved_module_cache_digest(empty)

    def test_non_module_directory_rejected(self):
        bogus = self.root / 'bogus'
        bogus.mkdir()
        (bogus / 'README.txt').write_text('not modules')
        with self.assertRaises(ValueError):
            approved_module_cache_digest(bogus)

    def test_stage_tampering_refused_and_deleted(self):
        import phase15_reviewed_module_cache as mod
        original = mod.shutil.copytree
        def tamper(src, dst, *args, **kwargs):
            output = original(src, dst, *args, **kwargs)
            if Path(src) != self.cache:
                return output
            p = Path(dst) / 'github.com/jackc/pgx/v5@v5.11.0/pgx.go'
            p.write_text('unexpected stage byte change')
            return output
        with mock.patch.object(mod.shutil, 'copytree', side_effect=tamper):
            with self.assertRaisesRegex(ValueError, 'staged Go module-cache'):
                stage_approved_module_cache(self.cache, self.stage, self.pin)
        self.assertFalse(self.stage.exists())

    def test_stage_symlink_injection_refused_and_deleted(self):
        import phase15_reviewed_module_cache as mod
        original = mod.shutil.copytree
        def inject(src, dst, *args, **kwargs):
            output = original(src, dst, *args, **kwargs)
            if Path(src) != self.cache:
                return output
            alias = Path(dst) / 'unreviewed.mod'
            alias.symlink_to(self.root / 'sensitive')
            return output
        with mock.patch.object(mod.shutil, 'copytree', side_effect=inject):
            with self.assertRaisesRegex(ValueError, 'symlink substituted'):
                stage_approved_module_cache(self.cache, self.stage, self.pin)
        self.assertFalse(self.stage.exists())


if __name__ == '__main__':
    unittest.main()
