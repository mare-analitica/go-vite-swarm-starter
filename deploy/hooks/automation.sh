# shellcheck shell=bash
# Hook of the "automation" stack (n8n).

ensure_pg_database n8n n8n n8n_db_password
# Encrypts the credentials stored in n8n: losing it makes them unreadable.
ensure_secret n8n_encryption_key hex

N8N_START_HASH="$(sha256sum "$DEPLOY_DIR/n8n/n8n-start.sh" | cut -c1-12)"
N8N_WORKFLOW_HASH="$(sha256sum "$REPO_DIR/dev/n8n/note-created.json" | cut -c1-12)"
export N8N_START_HASH N8N_WORKFLOW_HASH
