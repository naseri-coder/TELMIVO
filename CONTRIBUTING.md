# Development

Private proprietary repository. Source identifiers and commits use English; user-facing content uses locale keys, Persian and RTL by default. No open-source source license is granted.

Continue R1 only on `feature/platform-bootstrap-v0.1`. PR #1 targets `develop` and stays Draft until owner authorization. Never force push, rewrite published migrations, merge automatically or deploy. Add new checksummed SQL migrations for released schema changes.

Use `make bootstrap` then `make up` with Docker for synthetic local development. See `docs/operations/development.md`. Run targeted checks during changes; full relevant CI at a checkpoint. Native tests require an explicitly isolated `telmivo_test` database and restricted runtime role. Never point tests at production. Commit and push logical safe checkpoints.
