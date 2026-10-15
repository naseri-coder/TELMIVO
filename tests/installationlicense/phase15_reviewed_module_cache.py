"""Validate and stage an independently pinned, preloaded Go module cache.

Use only on an authorized disposable nonproduction test machine. The source
module-cache directory is NEVER modified. No subprocess, Docker, Go command,
network request, credentials, or Production API is used in this module.
"""
import hashlib
import os
from pathlib import Path
import re
import shutil
import stat


PIN_PATTERN = re.compile(r'sha256:[0-9a-f]{64}\Z')
MAX_FILES = 100_000
MAX_BYTES = 2 * 1024 * 1024 * 1024
DOMAIN = b'TELMIVO-LIC-A9-REVIEWED-GO-MODCACHE-V1\x00'


def reviewed_module_pin(value):
    if type(value) is not str or PIN_PATTERN.fullmatch(value) is None:
        raise ValueError('independently reviewed Go module-cache sha256 pin required')
    return value


def _directory(path):
    item = Path(path)
    if not item.is_absolute() or item.is_symlink() or item.resolve() != item:
        raise ValueError('module-cache must be a nonsymlinked absolute path')
    st = item.lstat()
    if (not stat.S_ISDIR(st.st_mode) or st.st_uid != os.geteuid()
            or (st.st_mode & 0o022)):
        raise ValueError('module-cache directory must be owned and protected')
    return item


def _inventory(source):
    root = _directory(source)
    file_count = 0
    total = 0
    data = hashlib.sha256()
    data.update(DOMAIN)
    candidate_module_metadata = False
    for top, dirs, files in os.walk(root, topdown=True, followlinks=False):
        parent = _directory(Path(top))
        for name in sorted(dirs):
            _directory(parent / name)
        dirs.sort()
        for name in sorted(files):
            path = parent / name
            st = path.lstat()
            if (not stat.S_ISREG(st.st_mode) or st.st_uid != os.geteuid()
                    or st.st_nlink != 1 or (st.st_mode & 0o022)):
                raise ValueError('unsafe regular file, mode, hardlink or symlink in module-cache')
            file_count += 1
            total += st.st_size
            if file_count > MAX_FILES or total > MAX_BYTES:
                raise ValueError('Go module-cache inventory exceeds limits')
            relative = path.relative_to(root).as_posix().encode('utf-8')
            if name in ('go.mod',) or name.endswith(('.mod', '.zip')):
                candidate_module_metadata = True
            digest = hashlib.sha256()
            fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC)
            try:
                snapshot = os.fstat(fd)
                if (snapshot.st_ino != st.st_ino or snapshot.st_dev != st.st_dev
                        or snapshot.st_size != st.st_size or snapshot.st_nlink != 1):
                    raise ValueError('Go module-cache file changed during scan')
                with os.fdopen(fd, 'rb', closefd=False) as stream:
                    while True:
                        chunk = stream.read(1024 * 1024)
                        if not chunk:
                            break
                        digest.update(chunk)
                if os.fstat(fd).st_size != st.st_size:
                    raise ValueError('Go module-cache file mutated during scan')
            finally:
                os.close(fd)
            data.update(len(relative).to_bytes(4, 'big'))
            data.update(relative)
            data.update(st.st_size.to_bytes(8, 'big'))
            data.update(digest.digest())
    if not file_count or not candidate_module_metadata:
        raise ValueError('not a populated Go module-cache')
    return 'sha256:' + data.hexdigest(), file_count, total


def approved_module_cache_digest(source):
    """Content-address the full path/file inventory, independent of filesystem order."""
    return _inventory(source)[0]


def stage_approved_module_cache(source, destination, expected_pin):
    """Fail closed and copy only approved bytes into a disposable writable cache.

    If validation fails after copying, delete the temporary destination. Never
    change the supplied source. The caller must supply a *separately reviewed*
    digest, not a digest it just calculated from this source itself.
    """
    reviewed_module_pin(expected_pin)
    src = _directory(source)
    dst = Path(destination)
    if not dst.is_absolute() or dst.exists() or dst.is_symlink():
        raise ValueError('new absolute disposable cache destination required')
    if dst.is_relative_to(src) or src.is_relative_to(dst):
        raise ValueError('module source and staged directory must be disjoint')
    if approved_module_cache_digest(src) != expected_pin:
        raise ValueError('unapproved or mutated Go module-cache source')
    try:
        shutil.copytree(src, dst, symlinks=True)
        # The final temporary cache is writable only by this user's Go
        # container UID, allowing Go's local unpack/index work without writes
        # to independently reviewed input or any module download.
        for top, dirs, files in os.walk(dst, topdown=True, followlinks=False):
            os.chmod(top, 0o700)
            for name in files:
                path = Path(top) / name
                if path.is_symlink():
                    raise ValueError('symlink substituted during Go module-cache staging')
                os.chmod(path, 0o600)
        if approved_module_cache_digest(dst) != expected_pin:
            raise ValueError('staged Go module-cache contents do not match reviewed hash')
    except Exception:
        if dst.exists():
            shutil.rmtree(dst)
        raise
    return {'pin': expected_pin, 'staged_file_count': _inventory(dst)[1],
            'staged_bytes': _inventory(dst)[2], 'network_used': False,
            'production_accessed': False}
