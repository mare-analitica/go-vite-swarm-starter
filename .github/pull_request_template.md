## Summary

<!-- What changes and why. Keep the PR title in Conventional Commits format: it becomes the squash commit subject. -->

Closes #

## Verification

<!-- How you verified it: tests, commands on a disposable Swarm, screenshots. -->

## Checklist

- [ ] Linked to an issue
- [ ] No real hostnames, domains, IPs, customer names or credentials
- [ ] Local checks pass (`make lint`, `make test`, shellcheck, hadolint — see CONTRIBUTING.md)
- [ ] Images pinned by digest; secrets only through Docker Secrets and `*_FILE`
- [ ] Platform changes tested on a disposable single-node Swarm
- [ ] Docs and `CHANGELOG.md` (`[Unreleased]`) updated for user-visible changes
