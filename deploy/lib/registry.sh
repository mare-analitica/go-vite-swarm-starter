# shellcheck shell=bash
# Logs the host daemon in to the local registry, so "docker stack deploy
# --with-registry-auth" can hand pull credentials to the application services.

registry_login() {
    local attempt
    [ -s "$SECRETS_DIR/registry_ci_password" ] || { log "registry password missing - deploy the platform stack first"; return 1; }
    # Right after a platform deploy the registry may still be starting.
    for attempt in 1 2 3 4 5 6; do
        if docker login 127.0.0.1:5000 -u ci --password-stdin < "$SECRETS_DIR/registry_ci_password" >/dev/null 2>&1; then
            return 0
        fi
        sleep 5
    done
    log "docker login to 127.0.0.1:5000 failed after $attempt attempts (is platform_registry healthy?)"
    return 1
}
