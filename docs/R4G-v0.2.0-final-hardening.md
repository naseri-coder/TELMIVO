# R4G — v0.2.0 Final Hardening

R4G is the final TELMIVO R4 / v0.2.0 source-hardening stage. It is stacked on the R4F
AI Assistant 2.0 checkpoint and introduces no new product feature.

## Release boundary

R4G freezes the expected migration chain at 0001–0023. No 0024 migration is introduced
because no new schema requirement is part of final hardening. A new migration is permitted
only if validation identifies a concrete schema defect.

The R4 release matrix is:

- R4A — Customer Bot Runtime Convergence
- R4B — Channel Manager
- R4C — Uploader & Storage Platform
- R4D — Commerce Checkout & Payments
- R4E — Automation 2.0
- R4F — AI Assistant 2.0
- R4G — v0.2.0 Final Hardening

## Final gates

R4G adds a fail-closed static release auditor and an integration recovery matrix. The
release candidate must preserve:

- 23 contiguous checksummed migrations ending at 0023_ai_assistant_2.sql;
- FORCE RLS on new R4 tenant tables;
- existing plugin/commercial compatibility, including Automation 2.0 and AI Assistant 2.0
  manifest/data versions and the AI cost quota;
- browser acceptance for Channel Manager, Uploader, Payments, Automation 2.0 and
  AI Assistant 2.0;
- PostgreSQL 17 fresh migration, race/integration tests and backup/restore;
- security source scan, Go vulnerability scan and npm high-severity audit;
- image builds, non-root backend runtime and synthetic Compose readiness;
- production configuration validation without a standalone customer-bot runtime.

## Cross-feature recovery

The final restored database is checked in one matrix for R4B channel-post state, R4C upload
relational state, R4D paid/confirmed payment state, R4E recurring schedule state and R4F
indexed knowledge state. Individual phase recovery tests remain in place; R4G adds a final
cross-feature assertion instead of replacing them.

## Release status semantics

A source-complete R4G branch is not a v0.2.0 PASS by itself. Final PASS requires every
mandatory exact-head GitHub Actions gate to execute and succeed. A job that fails before
its first step, remains unallocated, or is skipped without an approved policy is not
positive validation evidence.

R4G does not merge branches, create a tag, publish a GitHub Release, deploy production,
run a production migration or access customer data. Those actions remain separate and
require explicit authorization.
