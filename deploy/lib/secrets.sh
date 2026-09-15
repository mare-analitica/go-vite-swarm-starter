# shellcheck shell=bash
# Secrets generated ON THE HOST, never in Git or in environment variables.
#
#   ensure_secret <name> [alnum|hex]   create if missing and publish
#   require_secret <name>              human-provided (e.g. a vendor key): publish only
#   ensure_htpasswd <name> <user> <password-secret>   bcrypt htpasswd derived from a secret
#
# Each value lives in $SECRETS_DIR/<name> (root, 0600) and is published as the
# immutable Docker Secret "<name>_v<N>" (N in <name>.version, default 1). The
# variable <NAME>_SECRET=<name>_v<N> is exported for stack files:
#   secrets: { db_password: { external: true, name: ${DB_PASSWORD_SECRET} } }
#
# Rotation: write the new value, increment <name>.version, redeploy the stack.

_secret_publish() {
    local name="$1" version docker_name var vfile="$SECRETS_DIR/$1.version"
    [ -f "$vfile" ] || printf '1\n' > "$vfile"
    version="$(tr -dc '0-9' < "$vfile")"
    docker_name="${name}_v${version}"
    if ! docker secret inspect "$docker_name" >/dev/null 2>&1; then
        docker secret create "$docker_name" "$SECRETS_DIR/$name" >/dev/null
        log "secret '$docker_name' published"
    fi
    var="$(printf '%s' "$name" | tr '[:lower:]-' '[:upper:]_')_SECRET"
    export "$var=$docker_name"
}

ensure_secret() {
    local name="$1" format="${2:-alnum}" file="$SECRETS_DIR/$1"
    install -d -m 0700 "$SECRETS_DIR"
    if [ ! -s "$file" ]; then
        case "$format" in
            alnum) ( umask 077; head -c 64 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c 32 > "$file" ) ;;
            hex)   ( umask 077; head -c 32 /dev/urandom | od -An -vtx1 | tr -dc '0-9a-f' > "$file" ) ;;
            *) log "unknown secret format: $format"; return 1 ;;
        esac
        log "secret '$name' generated"
    fi
    chmod 0600 "$file"
    _secret_publish "$name"
}

require_secret() {
    local name="$1" file="$SECRETS_DIR/$1"
    if [ ! -s "$file" ]; then
        log "secret '$name' is missing: write it to $file (root, 0600) without echo, e.g."
        log "  umask 077; read -rsp 'value: ' v; printf '%s' \"\$v\" > $file; unset v"
        return 1
    fi
    chmod 0600 "$file"
    _secret_publish "$name"
}

ensure_htpasswd() {
    local name="$1" user="$2" password_secret="$3" file="$SECRETS_DIR/$1"
    command -v htpasswd >/dev/null || { log "htpasswd not found (apt install apache2-utils)"; return 1; }
    ensure_secret "$password_secret"
    if [ ! -s "$file" ] || ! htpasswd -vb "$file" "$user" "$(cat "$SECRETS_DIR/$password_secret")" >/dev/null 2>&1; then
        ( umask 077; htpasswd -nbB "$user" "$(cat "$SECRETS_DIR/$password_secret")" | sed '/^$/d' > "$file" )
        log "htpasswd '$name' generated for user '$user'"
    fi
    chmod 0600 "$file"
    _secret_publish "$name"
}
