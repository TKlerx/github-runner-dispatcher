# Configuration Contract

The participant reads one strict YAML document. Unknown keys, duplicate repository
entries, invalid durations, and unsupported labels are startup errors.

```yaml
participant_name: jan-cachy
repositories:
  - owner: TKlerx
    name: travel-agent
    runner_group_id: 1
  - owner: TKlerx
    name: calendar-sync
    runner_group_id: 1
labels:
  - self-hosted
  - Linux
  - X64
poll_interval: 10s
claim_delay: 0s
acquisition_timeout: 90s
capacity: 1
token_file: /etc/github-runner-dispatcher/token
runner_template_dir: /opt/actions-runner-template
state_dir: /var/lib/github-runner-dispatcher
github_api_url: https://api.github.com
github_api_version: 2026-03-10
```

A slower fallback participant uses the same repository list with a larger local
`claim_delay`, for example `60s`. Windows paths are normal YAML strings; forward
slashes are recommended where accepted by Windows.

## Secret contract

- `token_file` points to a file containing only the fine-grained PAT plus optional
  trailing newline.
- Literal token values in YAML are rejected as unknown fields.
- The token and encoded JIT configuration are never logged or persisted in manifests.
- Every participant uses a different PAT restricted to the configured repositories.
- Required permissions are Metadata read, Actions read, and Administration write.

## CLI contract

```text
runner-participant -config <path> [-check]
```

- `-config` is required.
- `-check` validates local files, labels, token access, repository visibility, and
  GitHub permissions without creating JIT runners or child processes.
- Normal mode runs in the foreground until interrupted.
- Exit `0`: clean shutdown or successful check.
- Exit `2`: invalid configuration or local prerequisite.
- Exit `3`: authentication/permission/repository validation failure.
- Exit `4`: unrecoverable runtime failure.
