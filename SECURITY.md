# Security Policy

## Reporting a vulnerability

Please report vulnerabilities through GitHub private vulnerability reporting when
available. Do not open a public issue containing credentials, host details, or an
unpatched exploit.

## Operational warning

Self-hosted runners execute repository workflow code on operator-controlled machines.
Use label-only dispatch only with private repositories and contributors you trust.
Public repositories require explicit trusted-workflow policies for every dispatched
job. Give each participant a separate fine-grained PAT limited to selected repositories and
the Metadata read, Actions read, and Administration write permissions required for
JIT runners. Treat JIT configuration as a secret and assume workflow jobs can modify
their host outside the temporary work directory.

Repository discovery may use GitHub CLI during setup, but the service must never
reuse, print, or persist GitHub CLI's authentication token. The participant PAT is
created manually from the generated GitHub form and stored only in `token_file`.

## Trusted-host boundary

An allowlisted workflow is arbitrary code running with the participant account's OS
permissions. Disposable runner directories prevent state reuse between jobs; they
are not a sandbox. Do not run participants on machines containing unrelated secrets,
personal data, privileged Docker sockets, reusable SSH credentials, or network access
that the repository must not receive. Repository collaborators and every dependency
that can alter a workflow are inside the trust boundary.

A trusted-workflow policy checks GitHub-reported workflow identity, event, actors, and
runner labels. It does not inspect or prove workflow content, branch protection, checkout
behavior, payload safety, or read-only execution. Prefer immutable workflow IDs over
paths where practical. Named-actor rules require both GitHub's original `actor` and
current `triggering_actor` to match; wildcard actor rules must be explicit.

Use a dedicated unprivileged OS account, one PAT per participant, restrictive file
permissions, and an explicit repository allowlist. Keep `token_file` outside the
managed state directory. Revoke only that machine's PAT when it is retired or
suspected compromised.

The participant never triggers, reruns, approves, or cancels workflows. Setup uses
GitHub CLI only to list owned private repositories and workflow status. Normal and
check modes do not execute `gh`; check mode uses read-only GitHub API requests and
does not generate JIT configuration.
