# go-vite-swarm-starter

**Digital foundation:** your application running on infrastructure you own,
versioned in Git, without depending on an expensive PaaS.

A Go API and a Vite frontend deployed to a single VPS with Docker Swarm:

- Automated deploy per project (Jenkins CI/CD, health checks and rollback)
- Automatic HTTPS on every domain
- Secrets isolated per system
- PostgreSQL, S3-compatible object storage and n8n included

> **Status:** under construction — see milestone
> [v0.1.0 — Digital foundation](https://github.com/mare-analitica/go-vite-swarm-starter/milestone/1).
> Operational practices follow
> [vps-gitops-harness](https://github.com/mare-analitica/vps-gitops-harness).

## Local development

Requirements: Docker with Compose, Go 1.25+, Node 22, `make`.

```bash
make dev    # PostgreSQL, MinIO, n8n and the API on http://localhost:8080
make web    # frontend on http://localhost:5173
```

| Service | URL | Notes |
| --- | --- | --- |
| Frontend | <http://localhost:5173> | Vite dev server, proxies `/api` |
| API | <http://localhost:8080/readyz> | Rebuilt by `make dev` |
| MinIO console | <http://localhost:9001> | `dev-minio-root` / `dev-minio-root-password` |
| n8n | <http://localhost:5678> | Workflow "Starter: note created" imported and active |

All credentials in `compose.dev.yaml` are **development-only and public**.
`make down` stops the environment; `make clean` also deletes its data.

## License

Copyright 2026 Paulo. Licensed under the [Apache License 2.0](LICENSE);
see [NOTICE](NOTICE).
