# shellcheck shell=bash
# Hook of the "app" stack: this system's own database, bucket and credentials.
# Copy this file (and deploy/stacks/app.yml) to add another system.

ensure_pg_database app app app_db_password
ensure_bucket_user app app_s3_access_key app_s3_secret_key
registry_login
