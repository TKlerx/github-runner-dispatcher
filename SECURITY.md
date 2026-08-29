# Security Policy

## Reporting a vulnerability

Please report vulnerabilities through GitHub private vulnerability reporting when
available. Do not open a public issue containing credentials, host details, or an
unpatched exploit.

## Operational warning

Self-hosted runners execute repository workflow code on operator-controlled machines.
Use this project only with private repositories and contributors you trust. Give
each participant a separate fine-grained PAT limited to selected repositories and
the Metadata read, Actions read, and Administration write permissions required for
JIT runners. Treat JIT configuration as a secret and assume workflow jobs can modify
their host outside the temporary work directory.

Repository discovery may use GitHub CLI during setup, but the service must never
reuse, print, or persist GitHub CLI's authentication token. The participant PAT is
created manually from the generated GitHub form and stored only in `token_file`.
