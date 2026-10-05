#!/bin/sh
set -eu
python3 - <<'PYCODE'
import os,urllib.parse
for name in ['TEST_DATABASE_URL','TEST_MIGRATION_DATABASE_URL']:
 u=urllib.parse.urlparse(os.environ.get(name,''))
 if u.hostname not in ('127.0.0.1','localhost','postgres') or u.path!='/telmivo_test': raise SystemExit('Explicit isolated telmivo_test URLs are required')
PYCODE
