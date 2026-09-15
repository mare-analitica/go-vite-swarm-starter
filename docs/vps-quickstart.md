# VPS quickstart

From a fresh server to the example application live on HTTPS, deployed by the
pipeline. Plan about 30 minutes, most of it waiting for image builds.

## Prerequisites

| What | Why |
| --- | --- |
| A VPS with **Debian 12/13 or Ubuntu 22.04/24.04**, 2 vCPU, **4 GB RAM** minimum (8 GB recommended), 40 GB disk | Everything runs on this one node |
| Root access over SSH with a key | The bootstrap runs as root |
| A domain and access to its DNS | Let's Encrypt needs public DNS records |
| Your public IP (for SSH and the admin consoles) | The firewall only lets this IP in |
| A fork or copy of this repository | Jenkins builds and deploys from it |

> **Provider firewall.** If your provider has its own firewall panel, allow
> TCP 22 (from your IP), 80 and 443 there as well.

## 1. DNS

Create `A` records pointing to the server's IP, for example:

| Record | Used by |
| --- | --- |
| `app.example.com` | frontend |
| `api.example.com` | API |
| `s3.example.com` | object storage (presigned URLs) |
| `minio.example.com` | MinIO console (admins only) |
| `n8n.example.com` | n8n editor (admins only) |
| `ci.example.com` | Jenkins (admins only) |

Certificates are requested on the first request to each domain, so the records
must resolve **before** you deploy.

## 2. Clone and bootstrap

```bash
ssh root@<server-ip>
apt-get update && apt-get install -y git
git clone https://github.com/<your-org>/<your-repo>.git /opt/starter
cd /opt/starter

# See what would change, then apply. Replace the CIDR with your IP.
deploy/host/bootstrap.sh --ssh-from 198.51.100.7/32 --dry-run
deploy/host/bootstrap.sh --ssh-from 198.51.100.7/32
```

The repository must live at a path owned by root (such as `/opt/starter`):
the deploy CLI and the forced SSH command run from it.

Jenkins deploys by SSH to the `deployer` user on the host. If your
`/etc/ssh/sshd_config` restricts logins with `AllowUsers` or `AllowGroups`,
add `deployer` there and reload sshd; the bootstrap does not change the SSH
server configuration.

> **Keep your current SSH session open** until you have confirmed that a new
> session connects. A wrong `--ssh-from` locks you out; the provider's web
> console is the way back in (`ufw disable`).

## 3. Configure

Edit `/etc/starter/config.env` (created from
[`deploy/config/example.env`](../deploy/config/example.env)):

- the six domains;
- `ACME_EMAIL`;
- `ADMIN_CIDRS`: your IP, e.g. `198.51.100.7/32`;
- `GIT_REPOSITORY_URL` and `GIT_BRANCH`: the repository and branch Jenkins
  deploys.

Private repository? Store a read-only token (for GitHub, a fine-grained token
with **Contents: read-only**) without echoing it:

```bash
umask 077
read -rsp 'token: ' t; printf '%s' "$t" > /etc/starter/secrets/jenkins_git_token; unset t
```

Nothing else needs to be written by hand: every other secret is generated on
the host on first use ([secrets model](secrets.md)).

## 4. Deploy the stacks

```bash
deploy stack platform     # Traefik, registry, PostgreSQL, MinIO
deploy stack automation   # n8n
deploy stack ci           # Jenkins (first run builds its image: a few minutes)
```

Each command waits until every service is healthy and rolls back on failure.
`deploy status` shows the state at any time.

To try things out without Let's Encrypt rate limits, uncomment
`ACME_CA_SERVER` (staging) in the config first; comment it again and redeploy
`platform` for trusted certificates. Staging certificates stay in the
`platform_traefik_certificates` volume, so delete `acme.json` from it when
switching.

## 5. First pipeline run

1. Open `https://ci.example.com` and sign in as `admin`. The password is on the
   host: `cat /etc/starter/secrets/jenkins_admin_password`.
2. Open the job **app** and click **Build Now**. The job also polls the branch
   every two minutes, so the first build may already have started on its own.
3. The pipeline runs the tests, builds and pushes both images, then deploys the
   API and the frontend. The first run creates the `app` stack, including its
   database, bucket and credentials.

Open `https://app.example.com`: create a note and attach a file.

## 6. Admin consoles

| Console | Access |
| --- | --- |
| n8n `https://n8n.example.com` | The first visitor creates the owner account: open it right away |
| MinIO `https://minio.example.com` | User `minio-root`, password in `/etc/starter/secrets/minio_root_password` |
| Jenkins `https://ci.example.com` | `admin`, see above |

All three answer only to `ADMIN_CIDRS`.

## What the bootstrap changed, and how to undo it

Every step is idempotent; run the script again after pulling a new version.

| Step | Change | Undo |
| --- | --- | --- |
| Packages | `git jq apache2-utils ufw ...` installed | `apt-get remove <package>` |
| Docker Engine | Docker's apt repository and packages | `apt-get purge docker-ce docker-ce-cli containerd.io docker-buildx-plugin`, remove `/etc/apt/sources.list.d/docker.list` |
| Swarm | single-node swarm, overlay networks `edge`, `data`, `ci` | `docker swarm leave --force` (**removes every stack**) |
| Directories | `/etc/starter` (config, `secrets/` 0700), `/var/lib/starter` (state, history) | Delete them (**secrets are lost**) |
| Deploy CLI | `/usr/local/sbin/deploy` and `deploy-ssh` symlinks | `rm /usr/local/sbin/deploy /usr/local/sbin/deploy-ssh` |
| Deploy user | `deployer` (locked password), `/etc/sudoers.d/starter-deploy`, `authorized_keys` with a forced command, CI key in `secrets/` | `userdel -r deployer`, remove the sudoers file and `secrets/ci_deploy_*` |
| BuildKit | `buildkitd` container, `/etc/starter/buildkitd.toml`, volume `buildkit_state` | `docker rm -f buildkitd && docker volume rm buildkit_state` |
| Firewall | UFW enabled (22 from `--ssh-from` and the Docker gateway, 80, 443); `starter-docker-user-firewall` service | `ufw disable`; `systemctl disable --now starter-docker-user-firewall`, then `iptables -D DOCKER-USER -j STARTER-DOCKER-USER` |

Next: [deploy and rollback](deploy-and-rollback.md) ·
[adding an app](adding-an-app.md) · [CI](ci.md)
