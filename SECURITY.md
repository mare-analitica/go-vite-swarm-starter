# Security policy

## Reporting a vulnerability

**Do not open a public issue.** Report privately through
[GitHub Security Advisories](https://github.com/mare-analitica/go-vite-swarm-starter/security/advisories/new).

Include:

- What is affected (file, script, stack, documented procedure) and the impact.
- Steps to reproduce, with **all real hostnames, IPs, credentials and customer
  data redacted**.
- Any suggested fix.

You can expect an acknowledgement within 5 business days and a status update
within 15 business days.

## Scope

In scope:

- The example application (`backend/`, `frontend/`).
- Deploy tooling: `deploy/bin`, hooks, helpers, `bootstrap.sh`, stack files,
  Jenkins configuration and `Jenkinsfile` (for example, a way for the CI key to
  do more than `deploy app`, a secret reaching an environment variable or log,
  a firewall gap).
- Documented procedures that would lead operators to leak credentials, lose
  data or lock themselves out.
- CI configuration of this repository.

Out of scope:

- Vulnerabilities in third-party software (Traefik, PostgreSQL, MinIO, n8n,
  Jenkins, base images). Report them upstream; open an issue here if a pinned
  version needs an upgrade.
- The known status of the MinIO community edition
  ([docs/object-storage.md](docs/object-storage.md)).
- Findings on servers you do not own or are not authorized to test.

## Supported versions

| Version | Supported |
| --- | --- |
| Latest release on `main` | Yes |
| Older releases | No |

## If you committed a secret by mistake

Treat the secret as compromised: **revoke and rotate it first**, then remove
it. Deleting a commit or force-pushing does not undo exposure, because forks,
clones and caches may already hold it.
