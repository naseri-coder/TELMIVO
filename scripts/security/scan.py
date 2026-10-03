import pathlib,re,subprocess
patterns=[r'-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----',r'gh[pousr]_[A-Za-z0-9]{30,}',r'AKIA[A-Z0-9]{16}',r'\b[0-9]{8,12}:[A-Za-z0-9_-]{35}\b']
paths=subprocess.check_output(['git','ls-files'],text=True).splitlines()
bad=[]
for name in paths:
 data=pathlib.Path(name).read_text(errors='replace')
 if any(re.search(p,data) for p in patterns): bad.append(name)
if bad: raise SystemExit('Potential secrets in files: '+','.join(bad))
print('Source secret patterns: %d files, zero matches (supplementary check)' % len(paths))
