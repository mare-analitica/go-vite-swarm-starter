# shellcheck shell=bash
# Hook of the "platform" stack, sourced by deploy before the stack is deployed.

ensure_secret postgres_password
ensure_secret minio_root_password
# Registry user "ci": BuildKit pushes with it and the host daemon pulls with it.
ensure_htpasswd registry_htpasswd ci registry_ci_password
