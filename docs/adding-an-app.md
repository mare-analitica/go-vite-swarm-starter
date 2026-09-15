# Adding an app

Each system gets its own stack, database, bucket, credentials, images and job.
Example: a second system called `crm`.

## 1. Stack file

Copy [`deploy/stacks/app.yml`](../deploy/stacks/app.yml) to
`deploy/stacks/crm.yml` and change:

- image variables: `${CRM_API_IMAGE:?no image yet, run the pipeline first}`
  and the same for `CRM_WEB_IMAGE` (the variable is the service name in upper
  case plus `_IMAGE`). Keep `-` out of that message: Docker's interpolation
  misreads it and renders the text as the image name;
- `DB_NAME`, `DB_USER`, `S3_BUCKET` → `crm`;
- secret names: `crm_db_password`, `crm_s3_access_key`, `crm_s3_secret_key`,
  and the matching `${CRM_DB_PASSWORD_SECRET}` variables;
- Traefik router and service names (`crm-api`, `crm-web`) and the `Host()`
  rules: add `CRM_DOMAIN` / `CRM_API_DOMAIN` to `/etc/starter/config.env`;
- the `N8N_WEBHOOK_URL`, if it uses n8n.

Router names must be unique across all stacks, or Traefik merges them.

## 2. Hook

`deploy/hooks/crm.sh`:

```bash
# shellcheck shell=bash
ensure_pg_database crm crm crm_db_password
ensure_bucket_user crm crm_s3_access_key crm_s3_secret_key
registry_login
```

## 3. Allow the CI to deploy it

In `/etc/starter/config.env`:

```bash
APP_SERVICES="app_api app_web crm_api crm_web"
```

## 4. Job

Add a `pipelineJob('crm')` to
[`deploy/jenkins/casc/jobs.yaml`](../deploy/jenkins/casc/jobs.yaml) pointing to
its repository, and give that repository a `Jenkinsfile` like the root one with
`REPO_API = 'crm/api'`, `REPO_WEB = 'crm/web'` and services `crm_api` /
`crm_web`.

## 5. Ship

```bash
git commit -am "feat(crm): add crm system" && git push
# on the host
cd /opt/starter && git pull
deploy stack ci          # creates the job
```

Run the job: the first images create the `crm` stack, its database and bucket.

## Public n8n webhooks

Applications call n8n internally (`http://automation_n8n:5678/webhook/...`).
To receive webhooks from outside (payment providers, forms), add a second
router in `deploy/stacks/automation.yml` without the allowlist, limited to the
webhook paths:

```yaml
- traefik.http.routers.n8n-webhooks.rule=Host(`${N8N_DOMAIN}`) && PathPrefix(`/webhook/`)
- traefik.http.routers.n8n-webhooks.entrypoints=websecure
- traefik.http.routers.n8n-webhooks.service=n8n
```

Authenticate those webhooks in the workflow (header secret or signature).

## Capacity

A small Go API and an nginx frontend use under 100 MB together; PostgreSQL,
MinIO, n8n and Jenkins take most of the memory. On 4 GB, watch `free -m` and
`docker stats` before adding heavy systems, and set `resources.limits` for
each service.
