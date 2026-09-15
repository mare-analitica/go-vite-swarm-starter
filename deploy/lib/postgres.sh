# shellcheck shell=bash
# One PostgreSQL role and database per system on the shared "platform" database.
#
#   ensure_pg_database <database> <role> <password-secret>
#
# Idempotent: creates role and database when missing and always reapplies the
# password from the secret (so a rotation takes effect on the next deploy).
# SQL, including the password, goes through psql's stdin — never a process
# argument. Applications never use the superuser.

_pg_container() {
    docker ps -q --filter "name=platform_postgres" --filter "health=healthy" | head -1
}

ensure_pg_database() {
    local db="$1" role="$2" secret_name="$3" container pw
    [[ "$db" =~ ^[a-z_][a-z0-9_]*$ && "$role" =~ ^[a-z_][a-z0-9_]*$ ]] \
        || { log "invalid database/role name: $db/$role"; return 1; }
    container="$(_pg_container)"
    [ -n "$container" ] || { log "platform_postgres is not healthy - deploy the platform stack first"; return 1; }
    ensure_secret "$secret_name"
    pw="$(cat "$SECRETS_DIR/$secret_name")"
    [[ "$pw" =~ ^[A-Za-z0-9]+$ ]] || { log "unexpected characters in $secret_name"; return 1; }

    docker exec -i "$container" psql -U postgres -v ON_ERROR_STOP=1 -q >/dev/null <<SQL
DO \$\$ BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '${role}') THEN
    CREATE ROLE ${role} LOGIN;
  END IF;
END \$\$;
ALTER ROLE ${role} WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE PASSWORD '${pw}';
SELECT 'CREATE DATABASE ${db} OWNER ${role}'
  WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '${db}')\gexec
REVOKE ALL ON DATABASE ${db} FROM PUBLIC;
GRANT ALL ON DATABASE ${db} TO ${role};
SQL
    log "database '$db' / role '$role' ready"
}
