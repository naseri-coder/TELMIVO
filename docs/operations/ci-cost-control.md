# Private repository CI cost control

TELMIVO stays private. This change does not install a self-hosted runner, change
repository visibility, enable billing, or alter the preserved R5B remediation.

CI Go owns automatic native integration, browser and backup/restore acceptance.
The version-scoped release workflow keeps its automatic static source audit but
runs its duplicate native suite only on manual workflow_dispatch. Its automatic
branch targets are covered by CI Go; a regression test protects that coverage.
Documentation-only changes retain the static release audit where selected, while
CI Go's existing paths-ignore policy continues to omit native execution for them.

Manual release audits still run both static and complete native acceptance.
All native test selectors, assertions, recovery checks, browser specs and steps
remain unchanged. Successful release acceptance requires actual native results
from CI Go or the manual release workflow for the required source checkpoint;
a successful static audit alone is insufficient.

Web and native test artifacts expire after seven days. Download evidence that
must be kept longer before expiry. Existing run cancellation and dependency
caches remain enabled. No minute-saving percentage is claimed without measured
executed runs. This change cannot replenish minutes already consumed.

A future dedicated self-hosted runner requires separate isolated infrastructure
and runner registration. Never attach CI to a production server or production
credentials. This optimization does not move jobs to an unregistered runner.
