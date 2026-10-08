#!/usr/bin/env python3
"""Create TEST-ONLY LIC-A7 fixture outside both source repositories.
Usage: prepare_authority_fixture.py AUTHORITY_REPO NEW_FIXTURE_DIR
Run go test ./authority -run '^TestE2E[1-6]$' with LIC_A8_TEST_DATABASE_URL
pointing to an exclusively disposable non-production PostgreSQL database.
"""
import pathlib, subprocess, sys
sha='392fc89087d8dac001d1709d0f7973c2f28a8515'
repo=pathlib.Path(sys.argv[1]).resolve(); dst=pathlib.Path(sys.argv[2]).resolve()
product=pathlib.Path(__file__).resolve().parents[2]
if dst.exists() or dst.is_relative_to(repo) or dst.is_relative_to(product):
 raise SystemExit('fixture must be a new directory outside source worktrees')
dst.mkdir(mode=0o700); authority=dst/'authority';authority.mkdir()
archive=subprocess.run(['git','-C',str(repo),'archive',sha],check=True,stdout=subprocess.PIPE).stdout
subprocess.run(['tar','-x','-C',str(authority)],input=archive,check=True)
old='github.com/naseri-coder/TELMIVO-LICENSE-AUTHORITY';new='github.com/naseri-coder/TELMIVO/lica8-e2e/authority'
for p in authority.rglob('*.go'):
 s=p.read_text().replace(old,new)
 # Injectable lease/business clock only. Transport replay/nonce clocks stay real.
 if p.parent==authority/'internal/store/postgres' and p.name!='transport.go' and not p.name.endswith('_test.go'):
  s=s.replace('clock_timestamp()', 'lic_a8_clock()')
 p.write_text(s)
mod=(product/'go.mod').read_text().replace('module github.com/naseri-coder/TELMIVO','module github.com/naseri-coder/TELMIVO/lica8-e2e',1)
mod+='\nrequire github.com/naseri-coder/TELMIVO v0.0.0\nreplace github.com/naseri-coder/TELMIVO => '+str(product)+'\n'
(dst/'go.mod').write_text(mod);(dst/'go.sum').write_bytes((product/'go.sum').read_bytes())
(authority/'client_test.go').write_bytes((pathlib.Path(__file__).parent/'authority_e2e_test.go.template').read_bytes())
print('TEST-ONLY fixture prepared from '+sha)
