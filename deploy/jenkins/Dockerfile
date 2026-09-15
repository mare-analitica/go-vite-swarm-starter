# syntax=docker/dockerfile:1
# Jenkins for the "ci" stack. Built on the host by deploy/hooks/ci.sh and
# pushed to the local registry, tagged with the hash of this directory.

# Static Docker CLI and buildx plugin: Jenkins is only a CLIENT of the rootless
# BuildKit container (tcp://buildkitd:1234). It has no access to the host's
# Docker API, so a compromised job cannot start containers on the host.
FROM docker:28-cli@sha256:625d9431a9f54c5a2bc90f24f0e1c3d55b1349fd857dd85035f98c2c9acbdd4d AS docker-cli

# Jenkins LTS 2.568.3, JDK 21
FROM jenkins/jenkins:lts-jdk21@sha256:c1e4c349365f6d16d88595b2c5f7e8ff39b8ae1d061f62420bac193b4b9616d0
COPY --from=docker-cli --chmod=0755 /usr/local/bin/docker /usr/local/bin/docker
COPY --from=docker-cli --chmod=0755 /usr/local/libexec/docker/cli-plugins/docker-buildx /usr/local/libexec/docker/cli-plugins/docker-buildx
COPY --chmod=0644 plugins.txt /usr/share/jenkins/ref/plugins.txt
COPY --chmod=0755 entrypoint.sh /usr/local/bin/starter-entrypoint.sh

RUN jenkins-plugin-cli --plugin-file /usr/share/jenkins/ref/plugins.txt

ENV JAVA_OPTS="-Djenkins.install.runSetupWizard=false -XX:MaxRAMPercentage=60"
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/starter-entrypoint.sh"]
