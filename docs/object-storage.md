# Object storage: MinIO status and alternatives

> **Warning.** The MinIO community edition is **no longer maintained as a
> distributable product**: its admin console features were removed from the
> community build in 2025, its public Docker Hub images and binaries stopped
> being published, and it does not receive regular security updates. This
> starter pins the last community image by digest
> (`quay.io/minio/minio:RELEASE.2025-09-07T16-13-09Z.hotfix…`).

## Why it is still here

- It is a single container that speaks the S3 API, which keeps the starter
  self-contained and cheap for demos, prototypes and internal tools.
- The application does not depend on MinIO: it uses the S3 API through
  `minio-go` and only needs an endpoint, a bucket and keys.

## Risk if you keep it

- Unpatched vulnerabilities over time. The S3 API is **public**
  (`S3_DOMAIN`), because browsers upload and download with presigned URLs.
- The console at `MINIO_CONSOLE_DOMAIN` is an object browser; administration
  is done with `mc` (the hooks use it) and the console is restricted to
  `ADMIN_CIDRS`.

Reasonable for: demos, staging, internal tools, non-critical files you also
back up elsewhere. For customer documents or anything regulated, move to one
of the options below.

## Moving to another S3 provider

The API needs these settings in the `app` stack:

| Setting | Meaning |
| --- | --- |
| `S3_ENDPOINT` | host:port the API connects to |
| `S3_USE_SSL` | `true` for a provider over HTTPS |
| `S3_PUBLIC_ENDPOINT` | URL the browser uses in presigned URLs (usually the same host with `https://`) |
| `S3_REGION` | provider region (default `us-east-1`) |
| `S3_BUCKET` | bucket name |
| `S3_ACCESS_KEY_FILE`, `S3_SECRET_KEY_FILE` | credentials limited to that bucket |

Steps:

1. Create the bucket and credentials limited to it in the provider.
2. Configure CORS on the bucket for `https://<APP_DOMAIN>` (methods `GET`,
   `POST`).
3. Store the keys with `require_secret` (written by hand, see
   [secrets](secrets.md)) and replace `ensure_bucket_user` in the hook.
4. Point the settings above to the provider, commit, `deploy stack app`.
5. Copy existing objects (`mc mirror` or `rclone sync`).
6. Remove `minio` from `platform.yml` when nothing uses it.

Uploads use **presigned POST** (a policy with an exact key and a size limit).
Check that the provider supports it; some S3-compatible services implement
only presigned PUT (for example, Cloudflare R2 at the time of writing), which
would require changing `PresignUpload` in `backend/internal/storage`.

## Options

| Option | Type | Notes |
| --- | --- | --- |
| AWS S3, Backblaze B2, Wasabi, provider object storage | managed | No server to patch; pay per GB and egress |
| Cloudflare R2 | managed | No egress fees; presigned POST not supported (see above) |
| Garage | self-hosted, AGPL-3.0 | Lightweight, designed for small clusters |
| SeaweedFS | self-hosted, Apache-2.0 | S3 gateway over its own storage |
| Ceph RGW | self-hosted, LGPL | Robust but heavy for one small VPS |

Evaluate license, S3 feature coverage (presigned POST, CORS, bucket policies)
and maintenance activity before choosing.
