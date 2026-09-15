# Contributing

Thanks for helping keep small deployments simple and safe.

## Ground rules

- **Never include real infrastructure data**: hostnames, domains, IP
  addresses, customer or company names, credentials, tokens or private keys.
  Use `example.com` and the documentation ranges `192.0.2.0/24`,
  `198.51.100.0/24` and `203.0.113.0/24` (RFC 5737).
- Every change goes through a pull request. `main` is protected: direct pushes
  are rejected and all CI checks must pass.
- One logical change per pull request, linked to an issue.

## Workflow

1. Open or pick an issue. For anything non-trivial, agree on the approach in
   the issue first.
2. Create a branch from `main`:

   | Prefix | Use |
   | --- | --- |
   | `feat/` | New capabilities |
   | `fix/` | Corrections |
   | `docs/` | Documentation only |
   | `ci/` | Workflows and automation |
   | `chore/` | Repository maintenance |

3. Commit using [Conventional Commits](https://www.conventionalcommits.org/):
   `type(scope): imperative summary`, for example
   `fix(deploy): wait for jobs before reporting success`. Scopes: `backend`,
   `frontend`, `platform`, `ci`, `docs`, `dev`. Explain **why** in the body
   when it is not obvious.
4. Open a pull request with `Closes #<issue>`, fill in the template and wait
   for CI.
5. Pull requests are **squash merged**; the pull request title becomes the
   commit subject, so keep it in Conventional Commits format.

## Local checks

```bash
make lint && make test
```

The same pinned tools CI uses:

```bash
# Shell scripts
docker run --rm -v "$PWD:/mnt:ro" -w /mnt koalaman/shellcheck:v0.11.0 --external-sources \
  $(git ls-files '*.sh' 'deploy/bin/*')

# Dockerfiles
for f in $(git ls-files '*Dockerfile*'); do docker run --rm -i hadolint/hadolint:v2.15.1 hadolint --failure-threshold warning - < "$f"; done

# Markdown
docker run --rm -v "$PWD:/workdir:ro" davidanson/markdownlint-cli2:v0.23.2 "**/*.md"

# Secrets in the full history
docker run --rm -v "$PWD:/repo:ro" --entrypoint sh ghcr.io/gitleaks/gitleaks:v8.30.1 -c \
  'git config --global --add safe.directory /repo && gitleaks git /repo --redact --no-banner'
```

On Windows Git Bash, prefix commands with `MSYS_NO_PATHCONV=1`.

## Changing the platform

- Test stack, hook and `deploy` changes on a disposable single-node Swarm
  (a spare VM, or `docker:dind`) before opening the pull request, and describe
  what you verified.
- Pin every image by digest (`image:tag@sha256:…`); upgrades are their own
  pull request.
- Secrets never go into stack files, environment variables or arguments: use
  the helpers in `deploy/lib` and `*_FILE` settings.
- Data services use `stop-first`; stateless services use `start-first`; every
  service has `failure_action: rollback` and a health check.
- Shell: `set -euo pipefail`, shellcheck clean, `--help`, no hardcoded hosts.
  Do not call functions that must fail loudly inside `if`, `!`, `&&` or `||`:
  bash disables `set -e` there.

## Changing the application

- Backend: `gofmt`, `go vet`, `golangci-lint` and tests for new behavior.
- Frontend: `npm run lint` and `npm run build` clean.
- New configuration: document it in the stack file and, if needed, in
  `deploy/config/example.env` and the docs.
