# shellcheck shell=bash
# Hook of the "ci" stack (Jenkins).

ensure_secret jenkins_admin_password
ensure_secret registry_ci_password   # created with the registry (platform hook)

# Optional read token for a private repository. Docker refuses empty secrets,
# so a public repository gets a placeholder and no credential in the job.
if [ ! -s "$SECRETS_DIR/jenkins_git_token" ]; then
    ( umask 077; printf 'none' > "$SECRETS_DIR/jenkins_git_token" )
fi
require_secret jenkins_git_token
if [ "$(cat "$SECRETS_DIR/jenkins_git_token")" = none ]; then
    GIT_CREDENTIALS_ID=""
else
    GIT_CREDENTIALS_ID="git-read"
fi

# Deploy key and known_hosts come from deploy/host/bootstrap.sh, together with
# the matching authorized_keys entry; generating them here would not work.
for f in ci_deploy_ssh_key ci_deploy_known_hosts; do
    [ -s "$SECRETS_DIR/$f" ] || { log "$SECRETS_DIR/$f is missing - run deploy/host/bootstrap.sh"; return 1; }
    require_secret "$f"
done
DEPLOY_SSH_HOST="$(docker network inspect docker_gwbridge --format '{{(index .IPAM.Config 0).Gateway}}')"

# Jenkins image: tag = hash of its build context, rebuilt only when it changes.
JENKINS_IMAGE_TAG="$(cat "$DEPLOY_DIR/jenkins/Dockerfile" "$DEPLOY_DIR/jenkins/plugins.txt" "$DEPLOY_DIR/jenkins/entrypoint.sh" | sha256sum | cut -c1-12)"
registry_login
if ! docker manifest inspect --insecure "127.0.0.1:5000/platform/jenkins:$JENKINS_IMAGE_TAG" >/dev/null 2>&1; then
    log "building the Jenkins image ($JENKINS_IMAGE_TAG), this takes a few minutes"
    docker build -q -t "127.0.0.1:5000/platform/jenkins:$JENKINS_IMAGE_TAG" "$DEPLOY_DIR/jenkins" >/dev/null \
        || { log "Jenkins image build failed"; return 1; }
    docker push -q "127.0.0.1:5000/platform/jenkins:$JENKINS_IMAGE_TAG" >/dev/null \
        || { log "Jenkins image push failed"; return 1; }
fi

JENKINS_CASC_HASH="$(sha256sum "$DEPLOY_DIR/jenkins/casc/jenkins.yaml" | cut -c1-12)"
JENKINS_JOBS_HASH="$(sha256sum "$DEPLOY_DIR/jenkins/casc/jobs.yaml" | cut -c1-12)"
export GIT_CREDENTIALS_ID DEPLOY_SSH_HOST JENKINS_IMAGE_TAG JENKINS_CASC_HASH JENKINS_JOBS_HASH
