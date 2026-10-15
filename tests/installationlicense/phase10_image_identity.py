"""Strict, offline-checkable Docker image identity gate for synthetic E2E.

An image *tag* is a mutable label, not an approved content identity. Operator
pins must come from a separately reviewed release/registry provenance record.
Never automatically pull an image, and never generate a trusted pin from the
same local image being admitted.
"""
import re


HEX_SHA256 = re.compile(r'sha256:[0-9a-f]{64}\Z')
HEX_COMMIT = re.compile(r'[0-9a-f]{40}\Z')
GO_TAG = 'golang:1.27.2-bookworm'
POSTGRES_TAG = 'postgres:16'


def reviewed_image_id(value):
    """Return a strict sha256 Docker image ID; never coerce arbitrary values."""
    if type(value) is not str or HEX_SHA256.fullmatch(value) is None:
        raise ValueError('independently approved Docker sha256 image ID required')
    return value


def reviewed_product_head(value):
    """Require an exact reviewed final checkout SHA provided out-of-band."""
    if type(value) is not str or HEX_COMMIT.fullmatch(value) is None:
        raise ValueError('independently reviewed exact TELMIVO commit SHA required')
    return value


def inspect_pinned_image(tag, expected_id, execute):
    """Check a *local* immutable Docker image ID without pulling or launching.

    execute is the caller's injectable no-shell subprocess wrapper. The returned
    immutable ID is used directly with docker run --pull=never, preventing tag
    retargeting between this check and container creation.
    """
    if tag not in (GO_TAG, POSTGRES_TAG):
        raise ValueError('unapproved Docker image tag')
    pinned = reviewed_image_id(expected_id)
    result = execute(['docker', 'image', 'inspect', '--format', '{{.Id}}', tag],
                     output=True, timeout=30)
    actual = getattr(result, 'stdout', None)
    if type(actual) is not str or actual.strip() != pinned:
        raise ValueError('reviewed Docker image ID does not match installed local image')
    return pinned
