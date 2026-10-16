"""Offline production-build Go toolchain compatibility and security regression."""
import pathlib
import re
import unittest

from run_joint_docker_only_e2e import GO_IMAGE

ROOT = pathlib.Path(__file__).resolve().parents[2]
DOCKERFILE = ROOT / 'deployments/docker/backend.Dockerfile'
MODULE = ROOT / 'go.mod'
PATCHED_IMAGE = 'golang:1.27.2-bookworm'


def inspect_builder(dockerfile: str, module: str, e2e_image: str) -> None:
    """Reject toolchain downgrade, an unexpected builder, or E2E/build drift."""
    builders = re.findall(r'^FROM\s+(golang:[^\s]+)\s+AS\s+build\s*$', dockerfile, re.M)
    if builders != [PATCHED_IMAGE]:
        raise ValueError('backend Go build stage must use reviewed patched toolchain')
    if e2e_image != PATCHED_IMAGE:
        raise ValueError('builder and joint E2E Go images must match exact patch release')
    go_lines = re.findall(r'^go\s+(\d+\.\d+(?:\.\d+)?)\s*$', module, re.M)
    if go_lines != ['1.27.0']:
        raise ValueError('unexpected application Go language/toolchain contract')
    if dockerfile.count('FROM ') != 2:
        raise ValueError('unreviewed additional Docker build stage')


class BackendGoBuildSecurityTests(unittest.TestCase):
    def test_source_file_uses_patched_go_and_matches_joint_runner(self):
        inspect_builder(DOCKERFILE.read_text(), MODULE.read_text(), GO_IMAGE)

    def test_rejects_security_downgrade_and_e2e_builder_mismatch(self):
        contents = DOCKERFILE.read_text()
        module = MODULE.read_text()
        for old in ['1.27.0', '1.27.1', '1.26.9', '1.27']:
            with self.subTest(old=old), self.assertRaises(ValueError):
                inspect_builder(contents.replace(PATCHED_IMAGE, f'golang:{old}-bookworm'),
                                module, GO_IMAGE)
        with self.assertRaises(ValueError):
            inspect_builder(contents, module, 'golang:1.27.1-bookworm')

    def test_rejects_ambiguous_or_unreviewed_build_stages(self):
        contents = DOCKERFILE.read_text()
        with self.assertRaises(ValueError):
            inspect_builder(contents + '\nFROM golang:1.27.1-bookworm AS build\n',
                            MODULE.read_text(), GO_IMAGE)
        with self.assertRaises(ValueError):
            inspect_builder(contents.replace(' AS build', ' AS compiler'),
                            MODULE.read_text(), GO_IMAGE)

    def test_rejects_unreviewed_go_mod_change(self):
        contents = DOCKERFILE.read_text()
        with self.assertRaises(ValueError):
            inspect_builder(contents, 'module app\ngo 1.28.0\n', GO_IMAGE)


if __name__ == '__main__':
    unittest.main()
