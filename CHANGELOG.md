# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-09-15

### Added

- Notes API in Go: PostgreSQL with embedded migrations, attachments uploaded
  from the browser with presigned POST (exact key and size limit) and
  confirmed by the API, presigned downloads, `note.created` events to n8n,
  health and readiness endpoints, `healthcheck` subcommand for distroless.
- Vite + React + TypeScript frontend served by unprivileged nginx.
- Local development environment with Docker Compose: PostgreSQL, MinIO with a
  bucket-scoped user, n8n with the example workflow, and the API.
- Swarm stacks: `platform` (Traefik with automatic HTTPS, read-only socket
  proxy, registry, PostgreSQL 17, MinIO), `automation` (n8n), `ci` (Jenkins)
  and `app`; every image pinned by digest.
- `deploy` CLI: global lock, dirty-repository and empty-variable refusal,
  convergence wait, rollback, last-good images and JSONL history; SSH forced
  command for the CI.
- Per-system secrets generated on the host as versioned Docker Secrets; one
  PostgreSQL role/database and one MinIO bucket/user per system.
- Jenkins as code with rootless BuildKit and a `Jenkinsfile` that tests,
  builds, pushes and deploys the API before the frontend.
- `bootstrap.sh` for Debian/Ubuntu: Docker, single-node Swarm, networks,
  deploy user, BuildKit, UFW and DOCKER-USER firewall, with `--dry-run`.
- Guides: VPS quickstart, deploy and rollback, secrets model, CI/CD, adding
  an app, object storage (MinIO status and alternatives).
- GitHub Actions quality gates, Dependabot, community files, issue forms and
  pull request template.

### Security

- MinIO community edition is pinned to its last community build and no longer
  receives updates; see `docs/object-storage.md`.

[Unreleased]: https://github.com/mare-analitica/go-vite-swarm-starter/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/mare-analitica/go-vite-swarm-starter/releases/tag/v0.1.0
