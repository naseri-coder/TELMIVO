#!/usr/bin/env python3
"""Opt-in NONPRODUCTION Go-cache integration probe with a *synthetic* Go module.

No Docker, PostgreSQL, TELMIVO/Authority source, GitHub or network requests.
The dependency is staged from a local file:// module proxy; the final Go test
runs with GOPROXY=off. Its SHA is SELF-GENERATED TEST EVIDENCE, never a
production-approved trust pin. Requires installed local Go >=1.23 and Python.
"""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import zipfile

from phase15_reviewed_module_cache import (
    approved_module_cache_digest, stage_approved_module_cache,
)


def _go_env(module_cache: Path, build_cache: Path, proxy: str):
    # Do not propagate production credentials or cloud/Docker target variables.
    minimal = {k: os.environ[k] for k in ('PATH', 'HOME', 'LANG', 'TMPDIR')
               if k in os.environ}
    minimal.update({'GOMODCACHE': str(module_cache), 'GOCACHE': str(build_cache),
                    'GOPROXY': proxy, 'GOSUMDB': 'off', 'GOTOOLCHAIN': 'local',
                    'GOWORK': 'off', 'GOTELEMETRY': 'off'})
    return minimal


def _call(args, cwd: Path, env, expected_ok=True):
    output = subprocess.run(args, cwd=cwd, env=env, text=True,
                            capture_output=True, timeout=90)
    if (output.returncode == 0) != expected_ok:
        raise ValueError(f'LOCAL_GO_PROBE_UNEXPECTED_EXIT_{output.returncode}')
    return output


def probe():
    with tempfile.TemporaryDirectory(prefix='lic-a9-phase16-real-go-') as name:
        root = Path(name)
        proxy = root / 'file_proxy' / 'example.org' / 'lic-a9-local-dep' / '@v'
        proxy.mkdir(parents=True)
        module = b'module example.org/lic-a9-local-dep\n\ngo 1.23\n'
        go = b'package localdep\nfunc Value() string { return "OFFLINE_MODULE_CACHE_OK" }\n'
        prefix = 'example.org/lic-a9-local-dep@v1.0.0/'
        (proxy/'v1.0.0.mod').write_bytes(module)
        (proxy/'v1.0.0.info').write_text(json.dumps(
            {'Version': 'v1.0.0', 'Time': '2026-10-15T00:00:00Z'}))
        (proxy/'list').write_text('v1.0.0\n')
        with zipfile.ZipFile(proxy/'v1.0.0.zip', 'w', zipfile.ZIP_DEFLATED) as z:
            z.writestr(prefix + 'go.mod', module)
            z.writestr(prefix + 'dep.go', go)
        source = root / 'reviewed_test_only_cache'
        source.mkdir(mode=0o700)
        consumer = root / 'test_consumer'
        consumer.mkdir(mode=0o700)
        (consumer/'go.mod').write_text(
            'module example.org/lic-a9-offline-probe\n\ngo 1.23\n\n'
            'require example.org/lic-a9-local-dep v1.0.0\n')
        (consumer/'integration_test.go').write_text(
            'package probe\nimport ("testing"; dep "example.org/lic-a9-local-dep")\n'
            'func TestOfflineCache(t *testing.T) { if dep.Value() != '
            '"OFFLINE_MODULE_CACHE_OK" { t.Fatal("module mismatch") } }\n')
        before = _go_env(source, root/'build_before', (root/'file_proxy').as_uri())
        _call(['go', 'mod', 'tidy'], consumer, before)
        pin = approved_module_cache_digest(source)
        stage = root / 'stage'
        stage.mkdir(mode=0o700)
        staged = stage / 'mod'
        evidence = stage_approved_module_cache(source, staged, pin)
        if approved_module_cache_digest(staged) != pin:
            raise ValueError('staged cache failed independent rehash')
        offline = _go_env(staged, root/'build_staged', 'off')
        result = _call(['go', 'test', '-mod=readonly', '-count=1', '-v', '.'],
                       consumer, offline)
        if '--- PASS: TestOfflineCache' not in result.stdout:
            raise ValueError('real Go test pass was not observed')
        empty = root/'empty'
        empty.mkdir(mode=0o700)
        blocked = _go_env(empty, root/'build_empty', 'off')
        failure = _call(['go', 'test', '-mod=readonly', '-count=1', '.'],
                        consumer, blocked, expected_ok=False)
        return {'verdict': 'SYNTHETIC_LOCAL_GO_CACHE_INTEGRATION_PASS',
                'positive_test': 'TestOfflineCache',
                'staged_files': evidence['staged_file_count'],
                'stage_digest_is_test_only': True,
                'empty_cache_blocked': failure.returncode != 0,
                'real_authority_e2e_run': False, 'postgres16_run': False,
                'production_authorized': False}


if __name__ == '__main__':
    try:
        print(json.dumps(probe(), sort_keys=True))
    except (OSError, ValueError, subprocess.TimeoutExpired) as exc:
        # Local diagnostic only, no raw Go output, paths or credentials.
        print(json.dumps({'verdict': 'LOCAL_GO_CACHE_PROBE_FAILED',
                          'reason_type': type(exc).__name__,
                          'production_authorized': False}, sort_keys=True))
        raise SystemExit(3)
