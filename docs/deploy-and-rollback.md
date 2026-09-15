# Deploy and rollback

`deploy` is the only way changes reach the Swarm. It runs on the host as root;
Jenkins reaches a restricted subset of it over SSH.

```text
deploy stack <name>             deploy/stacks/<name>.yml (+ deploy/hooks/<name>.sh)
deploy app <service> <image>    one application service, image pinned by digest
deploy status                   stacks, services, anything not converged
deploy history [count]          audit trail
```

## What every deploy does

1. Takes a global lock: two deploys never overlap; the second waits.
2. Refuses a repository with uncommitted changes (the host follows Git;
   `ALLOW_DIRTY=1` is a documented break-glass override).
3. Loads `/etc/starter/config.env`, runs the stack hook (secrets, database,
   bucket) and refuses any `${VARIABLE}` that is empty.
4. Validates the file with `docker stack config` and warns about images not
   pinned by digest.
5. Deploys with `--prune --with-registry-auth` and waits until every service
   has all replicas running **and healthy**, reading twice in a row.
6. On failure: Swarm rolls back the failed update (`failure_action: rollback`);
   `deploy` rolls back anything still mid-update and exits non-zero.
7. Appends a line to `/var/lib/starter/history.jsonl` (actor, action, target,
   image, commit, duration, result).

## Application updates

```bash
deploy app app_api 127.0.0.1:5000/app/api:<commit>@sha256:<digest>
```

- Only services listed in `APP_SERVICES` and images from the local registry
  pinned by digest are accepted.
- `start-first`: the new task must pass its health check before the old one
  stops, so a bad image never takes the site down.
- The last healthy image of each service is kept in
  `/var/lib/starter/images/`, so `deploy stack app` never reverts what the
  pipeline shipped.
- The first image of a service that does not exist yet creates its stack as
  soon as every service of that stack has an image.

## Rolling back by hand

A failed deploy already rolled back. To go back to an older version that was
healthy:

```bash
deploy history 20                               # find the image
deploy app app_api 127.0.0.1:5000/app/api:<old-commit>@sha256:<digest>
```

or rebuild the old commit in Jenkins. `docker service rollback <service>` also
works (one step back), but bypasses the lock and the history: prefer `deploy`.

For stack changes (configuration, versions), revert the commit, `git pull` on
the host and `deploy stack <name>`.

## Data services

PostgreSQL, MinIO, the registry and n8n use `stop-first`: two instances on one
data directory corrupt it, so these updates have a short downtime. **Never
change the major version of PostgreSQL** by editing the image: it needs a dump
and restore.

## Reading failures

```bash
deploy status                                  # what is not converged
docker service ps --no-trunc <service>         # task errors ("unhealthy", "no such image")
docker service logs --tail 100 <service>
```

| Symptom | Likely cause |
| --- | --- |
| `repository has uncommitted changes` | Someone edited files on the host: commit upstream and `git pull`, or `git stash` |
| `empty variables in <stack>.yml: X` | Missing key in `/etc/starter/config.env`, or a hook did not run |
| `platform_postgres is not healthy` | Deploy `platform` first, then the other stacks |
| Task `unhealthy` then rolled back | Application does not pass `/healthz` or crashes on start: read its logs |
| `no such image` / pull denied | Image not in the local registry, or registry login failed (`deploy stack platform`) |
| Certificate errors | DNS not pointing to the server yet, or staging CA enabled |
