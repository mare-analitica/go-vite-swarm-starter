#!/usr/bin/env bash
# bootstrap.sh - prepares a fresh Debian/Ubuntu VPS to run the starter.
#
#   sudo deploy/host/bootstrap.sh --ssh-from <your-ip>/32 [--dry-run]
#
# Idempotent: every step checks the current state first, so running it again
# changes nothing (or only what drifted). With --dry-run nothing is changed and
# the actions that would run are printed. docs/vps-quickstart.md explains each
# step and how to undo it.
set -Eeuo pipefail

BUILDKIT_IMAGE="moby/buildkit:v0.33.0-rootless@sha256:80b15f0735e87bab7bf59ec4d695dfb4a7cfb25521cf56dc75d6f256285b63ef"
REPO_DIR="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")/../.." && pwd)"
CONF_DIR=/etc/starter
SECRETS_DIR=$CONF_DIR/secrets
STATE_DIR=/var/lib/starter
DEPLOY_USER=deployer

DRY_RUN=0
SSH_FROM=""

usage() {
    cat <<'EOF'
Usage: sudo deploy/host/bootstrap.sh --ssh-from <cidr>[,<cidr>...] [--dry-run]

  --ssh-from   CIDRs allowed to reach SSH (port 22), e.g. your IP as 198.51.100.7/32.
               Use "any" only if your provider's firewall already restricts SSH.
  --dry-run    print what would change, change nothing

Steps: packages, Docker Engine, single-node Swarm, overlay networks, directories
and host config, deploy CLI, deploy user with forced command, CI deploy key,
rootless BuildKit, firewall (UFW + DOCKER-USER rule).
EOF
}

while [ $# -gt 0 ]; do
    case "$1" in
        --ssh-from) SSH_FROM="${2:-}"; shift 2 ;;
        --dry-run) DRY_RUN=1; shift ;;
        -h|--help) usage; exit 0 ;;
        *) usage >&2; exit 2 ;;
    esac
done

log()  { printf '\033[1m==> %s\033[0m\n' "$*"; }
info() { printf '    %s\n' "$*"; }
die()  { printf 'error: %s\n' "$*" >&2; exit 1; }
# run: execute, or only print in dry-run mode.
run() {
    if [ "$DRY_RUN" = 1 ]; then printf '    [dry-run] %s\n' "$*"; else "$@"; fi
}
# write_file <path> <mode>: content from stdin, replaced only when it differs.
write_file() {
    local path="$1" mode="$2" tmp
    tmp="$(mktemp)"
    cat > "$tmp"
    if [ -f "$path" ] && cmp -s "$tmp" "$path"; then
        rm -f "$tmp"
        return 0
    fi
    if [ "$DRY_RUN" = 1 ]; then
        printf '    [dry-run] write %s (mode %s)\n' "$path" "$mode"
        rm -f "$tmp"
    else
        install -D -m "$mode" "$tmp" "$path"
        rm -f "$tmp"
        info "wrote $path"
    fi
}

preflight() {
    [ "$(id -u)" -eq 0 ] || die "run as root (sudo)"
    [ -n "$SSH_FROM" ] || { usage >&2; die "--ssh-from is required (avoid locking yourself out)"; }
    local cidr_re='^([0-9]{1,3}\.){3}[0-9]{1,3}/[0-9]{1,2}$'
    if [ "$SSH_FROM" != any ]; then
        local c
        IFS=',' read -r -a cidrs <<< "$SSH_FROM"
        for c in "${cidrs[@]}"; do [[ "$c" =~ $cidr_re ]] || die "invalid CIDR: $c"; done
    fi
    # shellcheck source=/dev/null
    . /etc/os-release
    case "${ID:-}" in
        debian|ubuntu) ;;
        *) die "unsupported distribution: ${ID:-unknown} (Debian or Ubuntu required)" ;;
    esac
    OS_ID="$ID"
    OS_CODENAME="${VERSION_CODENAME:?missing VERSION_CODENAME}"
    git -C "$REPO_DIR" rev-parse --git-dir >/dev/null 2>&1 || die "$REPO_DIR must be a git clone of the repository"
    [ "$DRY_RUN" = 1 ] && log "dry run: nothing will be changed"
    return 0
}

step_packages() {
    log "packages"
    local pkgs=(ca-certificates curl gnupg git jq apache2-utils ufw util-linux openssh-server iptables) missing=()
    local p
    for p in "${pkgs[@]}"; do
        dpkg -s "$p" >/dev/null 2>&1 || missing+=("$p")
    done
    if [ "${#missing[@]}" -eq 0 ]; then info "already installed"; return; fi
    run apt-get update -qq
    run env DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends "${missing[@]}"
}

step_docker() {
    log "Docker Engine"
    if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
        info "already installed: $(docker version --format '{{.Server.Version}}')"
        return
    fi
    # Official repository (https://docs.docker.com/engine/install/).
    run install -d -m 0755 /etc/apt/keyrings
    run curl -fsSL "https://download.docker.com/linux/$OS_ID/gpg" -o /etc/apt/keyrings/docker.asc
    run chmod a+r /etc/apt/keyrings/docker.asc
    write_file /etc/apt/sources.list.d/docker.list 0644 <<EOF
deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/$OS_ID $OS_CODENAME stable
EOF
    run apt-get update -qq
    run env DEBIAN_FRONTEND=noninteractive apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin
    run systemctl enable --now docker
}

step_swarm() {
    log "single-node Swarm and networks"
    if [ "$DRY_RUN" = 1 ] && ! command -v docker >/dev/null 2>&1; then
        info "[dry-run] docker swarm init; docker network create edge, data, ci"
        return
    fi
    # docker_gwbridge links overlay networks to the host. Swarm creates it
    # lazily (first container on an overlay), but the deploy user's
    # authorized_keys and the CI known_hosts need its subnet and gateway now.
    # Creating it before "swarm init" is the documented way to own it.
    if ! docker network inspect docker_gwbridge >/dev/null 2>&1; then
        run docker network create --driver bridge \
            --opt com.docker.network.bridge.name=docker_gwbridge \
            --opt com.docker.network.bridge.enable_icc=false \
            --opt com.docker.network.bridge.enable_ip_masquerade=true \
            docker_gwbridge
    fi
    if [ "$(docker info --format '{{.Swarm.LocalNodeState}}')" = active ]; then
        info "swarm already active"
    else
        local addr
        addr="$(ip -4 route get 1.1.1.1 | awk '{for (i = 1; i < NF; i++) if ($i == "src") print $(i + 1)}')"
        run docker swarm init --advertise-addr "$addr"
    fi
    local net
    for net in edge data ci; do
        if docker network inspect "$net" >/dev/null 2>&1; then
            info "network $net exists"
        elif [ "$net" = ci ]; then
            # attachable: the buildkitd container (not a service) joins it.
            run docker network create --driver overlay --attachable ci
        else
            run docker network create --driver overlay "$net"
        fi
    done
}

step_directories() {
    log "directories, host configuration and deploy CLI"
    run install -d -m 0755 "$CONF_DIR"
    run install -d -m 0700 "$SECRETS_DIR"
    run install -d -m 0750 "$STATE_DIR"
    if [ -f "$CONF_DIR/config.env" ]; then
        info "$CONF_DIR/config.env exists (not touched)"
    else
        run install -m 0644 "$REPO_DIR/deploy/config/example.env" "$CONF_DIR/config.env"
        info "created $CONF_DIR/config.env from the example: EDIT IT before deploying"
    fi
    local bin
    for bin in deploy deploy-ssh; do
        if [ "$(readlink -f "/usr/local/sbin/$bin" 2>/dev/null)" != "$REPO_DIR/deploy/bin/$bin" ]; then
            run ln -sfn "$REPO_DIR/deploy/bin/$bin" "/usr/local/sbin/$bin"
        fi
    done
}

step_deploy_user() {
    log "deploy user ($DEPLOY_USER) with forced command"
    if ! id "$DEPLOY_USER" >/dev/null 2>&1; then
        run useradd --system --create-home --shell /bin/bash "$DEPLOY_USER"
        run passwd -l "$DEPLOY_USER"
    else
        info "user exists"
    fi

    # Only the read commands and "app" (validated again by deploy itself).
    local sudoers
    sudoers="$(cat <<EOF
# Managed by deploy/host/bootstrap.sh
Defaults:$DEPLOY_USER env_keep += "DEPLOY_ACTOR"
$DEPLOY_USER ALL=(root) NOPASSWD: /usr/local/sbin/deploy status, /usr/local/sbin/deploy history, /usr/local/sbin/deploy history *, /usr/local/sbin/deploy app *
EOF
)"
    if [ "$DRY_RUN" = 0 ]; then
        local tmp; tmp="$(mktemp)"
        printf '%s\n' "$sudoers" > "$tmp"
        visudo -cqf "$tmp" || { rm -f "$tmp"; die "generated sudoers file is invalid"; }
        rm -f "$tmp"
    fi
    printf '%s\n' "$sudoers" | write_file /etc/sudoers.d/starter-deploy 0440

    # CI deploy key (private half becomes a Docker Secret for Jenkins).
    if [ -s "$SECRETS_DIR/ci_deploy_ssh_key" ]; then
        info "CI deploy key exists"
    else
        run ssh-keygen -q -t ed25519 -N '' -C ci-deploy -f "$SECRETS_DIR/ci_deploy_ssh_key"
    fi
    [ "$DRY_RUN" = 1 ] && [ ! -s "$SECRETS_DIR/ci_deploy_ssh_key.pub" ] && { info "[dry-run] authorized_keys and known_hosts"; return; }

    local gw_subnet gw_ip home
    gw_subnet="$(docker network inspect docker_gwbridge --format '{{(index .IPAM.Config 0).Subnet}}')"
    gw_ip="$(docker network inspect docker_gwbridge --format '{{(index .IPAM.Config 0).Gateway}}')"
    home="$(getent passwd "$DEPLOY_USER" | cut -d: -f6)"
    # Owned by root: the deploy user cannot change its own restrictions.
    run install -d -o root -g root -m 0755 "$home/.ssh"
    write_file "$home/.ssh/authorized_keys" 0644 <<EOF
restrict,command="/usr/local/sbin/deploy-ssh",from="$gw_subnet" $(cat "$SECRETS_DIR/ci_deploy_ssh_key.pub")
EOF
    # Containers reach the host through the docker_gwbridge gateway.
    write_file "$SECRETS_DIR/ci_deploy_known_hosts" 0600 <<EOF
$gw_ip $(cut -d' ' -f1,2 /etc/ssh/ssh_host_ed25519_key.pub)
EOF
}

step_buildkit() {
    log "rootless BuildKit"
    write_file "$CONF_DIR/buildkitd.toml" 0644 <<'EOF'
# Managed by deploy/host/bootstrap.sh
[worker.oci]
  gc = true
  gckeepstorage = "10GB"

# Local registry over the internal ci network (plain HTTP, never exposed).
[registry."registry:5000"]
  http = true
EOF
    local want current
    want="$(printf '%s %s' "$BUILDKIT_IMAGE" "$(sha256sum "$CONF_DIR/buildkitd.toml" 2>/dev/null | cut -c1-12)")"
    current="$(docker inspect --format '{{index .Config.Labels "starter.spec"}}' buildkitd 2>/dev/null || true)"
    if [ "$current" = "$want" ] && [ "$(docker inspect --format '{{.State.Running}}' buildkitd 2>/dev/null)" = true ]; then
        info "buildkitd up to date"
        return
    fi
    [ -n "$current" ] && run docker rm -f buildkitd
    # Rootless: builds run as an unprivileged user inside the container. The
    # unconfined profiles are required by rootless BuildKit, not by the builds.
    run docker run -d --name buildkitd --restart unless-stopped \
        --label "starter.spec=$want" \
        --security-opt seccomp=unconfined --security-opt apparmor=unconfined \
        -v buildkit_state:/home/user/.local/share/buildkit \
        -v "$CONF_DIR/buildkitd.toml:/home/user/.config/buildkit/buildkitd.toml:ro" \
        "$BUILDKIT_IMAGE" --addr tcp://0.0.0.0:1234 --oci-worker-no-process-sandbox
    run docker network connect --alias buildkitd ci buildkitd
}

step_firewall() {
    log "firewall"
    local gw_subnet
    gw_subnet="$(docker network inspect docker_gwbridge --format '{{(index .IPAM.Config 0).Subnet}}' 2>/dev/null || echo 172.18.0.0/16)"

    # UFW filters traffic to the host itself.
    run ufw --force default deny incoming
    run ufw --force default allow outgoing
    if [ "$SSH_FROM" = any ]; then
        run ufw allow 22/tcp
    else
        local c
        IFS=',' read -r -a cidrs <<< "$SSH_FROM"
        for c in "${cidrs[@]}"; do run ufw allow from "$c" to any port 22 proto tcp; done
    fi
    # Jenkins (container) -> host SSH for the forced deploy command.
    run ufw allow from "$gw_subnet" to any port 22 proto tcp
    run ufw allow 80/tcp
    run ufw allow 443/tcp
    run ufw --force enable

    # Ports published by Docker skip UFW (they are forwarded, not input).
    # DOCKER-USER: from the internet only 80/443 reach containers; the
    # registry (5000) stays reachable from the host itself.
    write_file /usr/local/sbin/starter-docker-user-firewall 0755 <<'EOF'
#!/bin/sh
# Managed by deploy/host/bootstrap.sh: only 80/443 reach containers from outside.
set -eu
ext_if="$(ip route show default | awk '{print $5; exit}')"
iptables -N STARTER-DOCKER-USER 2>/dev/null || iptables -F STARTER-DOCKER-USER
iptables -A STARTER-DOCKER-USER -m conntrack --ctstate RELATED,ESTABLISHED -j RETURN
iptables -A STARTER-DOCKER-USER ! -i "$ext_if" -j RETURN
iptables -A STARTER-DOCKER-USER -p tcp -m conntrack --ctorigdstport 80 -j RETURN
iptables -A STARTER-DOCKER-USER -p tcp -m conntrack --ctorigdstport 443 -j RETURN
iptables -A STARTER-DOCKER-USER -j DROP
iptables -C DOCKER-USER -j STARTER-DOCKER-USER 2>/dev/null || iptables -I DOCKER-USER -j STARTER-DOCKER-USER
EOF
    write_file /etc/systemd/system/starter-docker-user-firewall.service 0644 <<'EOF'
[Unit]
Description=Restrict Docker-published ports to 80/443 from outside
After=docker.service
Requires=docker.service
PartOf=docker.service

[Service]
Type=oneshot
ExecStart=/usr/local/sbin/starter-docker-user-firewall
RemainAfterExit=yes

[Install]
WantedBy=docker.service
EOF
    run systemctl daemon-reload
    run systemctl enable starter-docker-user-firewall.service
    run systemctl restart starter-docker-user-firewall.service
}

summary() {
    log "done"
    cat <<EOF
    Next steps (docs/vps-quickstart.md):
      1. edit $CONF_DIR/config.env (domains, ACME_EMAIL, ADMIN_CIDRS, repository)
      2. deploy stack platform && deploy stack automation && deploy stack ci
      3. open https://<CI_DOMAIN> as admin (password in $SECRETS_DIR/jenkins_admin_password)
EOF
}

preflight
step_packages
step_docker
step_swarm
step_directories
step_deploy_user
step_buildkit
step_firewall
summary
