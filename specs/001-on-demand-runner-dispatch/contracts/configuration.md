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

`runner_template_dir` and `state_dir` must have link-free ancestry: symbolic links
and Windows reparse points are rejected. Workflow-created links inside a disposable
work directory are unlinked during cleanup without traversing their targets.

## Secret contract

- `token_file` points to a file containing only the fine-grained PAT plus optional
  trailing newline.
- Literal token values in YAML are rejected as unknown fields.
- The token and encoded JIT configuration are never logged or persisted in manifests.
- Every participant uses a different PAT restricted to the configured repositories.
- Required permissions are Metadata read, Actions read, and Administration write.

## CLI contract

```text
runner-participant -config <path> [-check | -setup]
```

- `-config` is required.
- `-setup` uses the current `gh` authentication to enumerate owned private
  repositories, marks archived entries and entries with no active Actions workflow,
  and accepts a numbered multi-selection. Workflow status lookup failure is shown as
  unknown rather than silently treated as no CI.
- Because `-config` names an existing copied example or machine configuration,
  `-setup` warns immediately and defaults to cancellation. The operator can keep the
  existing allowlist without writing, or replace only `repositories` after a final
  confirmation. Cancellation leaves the file byte-for-byte unchanged.
- `-setup` prints a URL based on
  `https://github.com/settings/personal-access-tokens/new` with `target_name`,
  `actions=read`, `administration=write`, and `metadata=read`, followed by the exact
  repository names the operator must select manually in GitHub. The URL cannot
  preselect repositories.
- `-setup` does not require an existing PAT or token file and never reads or prints a
  token. All selected repositories must have the same owner.
- `-check` validates local files and path ancestry, labels, token access, repository
  visibility, and GitHub permissions without creating JIT runners or child processes.
- Normal mode runs in the foreground until interrupted.
- Exit `0`: clean shutdown or successful check.
- Exit `2`: invalid configuration or local prerequisite.
- Exit `3`: GitHub CLI authentication or API failure, or runtime PAT
  authentication/permission/repository validation failure.
- Exit `4`: unrecoverable runtime failure.
