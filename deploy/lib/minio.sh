# shellcheck shell=bash
# One bucket and one set of credentials per system on the shared MinIO.
#
#   ensure_bucket_user <bucket> <access-key-secret> <secret-key-secret>
#
# The credentials can read, write and delete objects in that bucket only.
# The root password and the app secret key go through the exec's stdin, so
# they never appear in a process argument on the host.

_minio_container() {
    docker ps -q --filter "name=platform_minio" --filter "health=healthy" | head -1
}

ensure_bucket_user() {
    local bucket="$1" access_secret="$2" key_secret="$3" container access
    [[ "$bucket" =~ ^[a-z0-9][a-z0-9-]{2,62}$ ]] || { log "invalid bucket name: $bucket"; return 1; }
    container="$(_minio_container)"
    [ -n "$container" ] || { log "platform_minio is not healthy - deploy the platform stack first"; return 1; }
    ensure_secret "$access_secret" access
    ensure_secret "$key_secret"
    access="$(cat "$SECRETS_DIR/$access_secret")"

    # Line 1: root password. Line 2: app secret key. The script is an argument
    # (no secrets in it); values are read from stdin inside the container.
    printf '%s\n%s\n' "$(cat "$SECRETS_DIR/minio_root_password")" "$(cat "$SECRETS_DIR/$key_secret")" \
        | docker exec -i -e BUCKET="$bucket" -e ACCESS="$access" "$container" sh -c '
set -eu
IFS= read -r root_pw
IFS= read -r app_key
export MC_HOST_local="http://minio-root:${root_pw}@127.0.0.1:9000"
mc mb --ignore-existing "local/${BUCKET}" >/dev/null
cat > /tmp/policy.json <<EOF
{"Version":"2012-10-17","Statement":[
 {"Effect":"Allow","Action":["s3:GetObject","s3:PutObject","s3:DeleteObject"],"Resource":["arn:aws:s3:::${BUCKET}/*"]},
 {"Effect":"Allow","Action":["s3:ListBucket","s3:GetBucketLocation"],"Resource":["arn:aws:s3:::${BUCKET}"]}]}
EOF
mc admin policy create local "${BUCKET}-rw" /tmp/policy.json >/dev/null
rm -f /tmp/policy.json
printf "%s" "$app_key" | mc admin user add local "${ACCESS}" >/dev/null 2>&1 \
  || mc admin user add local "${ACCESS}" "$app_key" >/dev/null
mc admin policy attach local "${BUCKET}-rw" --user "${ACCESS}" >/dev/null 2>&1 || true
'
    log "bucket '$bucket' with scoped credentials ready"
}
