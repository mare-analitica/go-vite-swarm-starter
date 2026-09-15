#!/bin/sh
# Prepares Jenkins to build on the rootless BuildKit container, then starts it.
set -eu

# Registry credentials for pushes, written at start from the Docker Secret
# (never in the image, an environment variable or a process argument).
# BuildKit reaches the registry as registry:5000 over the ci network.
install -d -m 0700 "$JENKINS_HOME/.docker"
auth="$(printf 'ci:%s' "$(cat /run/secrets/registry_ci_password)" | base64 | tr -d '\n')"
# umask only for this file (subshell): leaking it into Jenkins would make every
# checkout 0600 and images built from it unreadable for non-root users.
( umask 077
  printf '{"auths":{"registry:5000":{"auth":"%s"}}}\n' "$auth" > "$JENKINS_HOME/.docker/config.json" )
unset auth

if ! docker buildx inspect buildkit >/dev/null 2>&1; then
    docker buildx create --name buildkit --driver remote tcp://buildkitd:1234 >/dev/null
fi
docker buildx use --default buildkit

exec /usr/local/bin/jenkins.sh "$@"
