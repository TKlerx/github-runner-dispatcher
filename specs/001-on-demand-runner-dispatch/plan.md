# Implementation Plan: Decentralized On-Demand Runner Participation

**Branch**: `001-on-demand-runner-dispatch` | **Date**: 2026-08-29 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/001-on-demand-runner-dispatch/spec.md`

## Summary

Build one small Go executable that runs independently on trusted Windows and Linux
machines. It polls allowlisted private repositories for queued jobs, filters by
actual participant labels, applies a local first-seen claim delay, rechecks the job,
and starts an official one-job GitHub Actions runner from repository-scoped JIT
configuration. Each runner uses a copied temporary runner directory, while a small
local JSON manifest supports restart reconciliation and cleanup.

## Technical Context

**Language/Version**: Go 1.27

**Primary Dependencies**: Go standard library; `go.yaml.in/yaml/v4` for strict YAML configuration

**Setup Dependency**: Authenticated GitHub CLI, required only for interactive repository discovery

**Storage**: Local filesystem only: YAML configuration, PAT file, transient runner directories, and atomic JSON runner manifests

**Testing**: Go `testing`, `httptest`, fake clock/process interfaces, race detector, and Windows/Linux CI builds

**Target Platform**: Windows 10/11 x64 and supported x64 Linux distributions running the official GitHub Actions runner

**Project Type**: Single cross-platform background service with foreground CLI entry point

**Performance Goals**: Under 50 MiB idle RSS; zero runner child processes while idle; complete one observation cycle for 10 repositories inside a 10-second polling interval under healthy API conditions

**Constraints**: Private repositories only; one fine-grained PAT per participant; no database, coordinator, peer protocol, embedded web server, or repository-specific service; default local capacity one

**Scale/Scope**: Personal-account installations with 1-50 allowlisted repositories and 1-4 concurrent local JIT runners

## Constitution Check

*GATE: Passed before research and re-checked after design.*

- [x] Runtime scope is limited to observing jobs and offering official JIT runner capacity; setup is read-only against GitHub.
- [x] Per-participant credentials follow least privilege and explicit private-repository selection; setup does not reuse GitHub CLI authentication as the participant token.
- [x] Participants remain independent; claim delay, final recheck, labels, and local capacity are explicit.
- [x] One-job temporary execution, restart reconciliation, timeouts, and cleanup are recoverable.
- [x] State transitions, setup/configuration mutation, security boundaries, and Windows/Linux differences have automated test coverage.

The sole non-standard runtime dependency is the maintained YAML parser required for
a human-editable strict configuration contract. GitHub access, logging, HTTP,
process execution, manifests, scheduling, and testing use the standard library.

## Design

### Repository and PAT setup

Setup mode is a one-time convenience and the only feature that depends on GitHub CLI.
It executes `gh api --paginate --slurp` against
`/user/repos?visibility=private&affiliation=owner&per_page=100`, sorts results by full
name, and queries each repository's Actions workflows to mark entries with no active
workflow. Archived and no-active-workflow repositories remain selectable because CI
may be added or enabled later.

Because setup starts from a copied example or machine config, an existing file always
causes a warning and defaults to cancellation. The operator may keep its current
allowlist without a write—useful after copying NAS configuration to another host—or
explicitly replace only the repository list. Replacement is shown for confirmation
and written atomically while preserving every other typed setting.

The standard library builds a fine-grained-PAT form URL with the common repository
owner as `target_name` and `actions=read`, `administration=write`, and `metadata=read`.
GitHub does not support preselecting individual repositories through URL parameters,
so setup prints the selected names as a checklist for the browser form. Setup never
retrieves or accepts the resulting token. Repositories with different owners are
rejected because one fine-grained PAT has one resource owner.

### Observation and selection

For every allowlisted repository, each cycle lists queued and in-progress workflow
runs, then lists their latest jobs. The participant retains a bounded in-memory
`first_seen_at` value for each queued job because GitHub does not expose the job's
queue timestamp. Matching jobs become eligible at `first_seen_at + claim_delay` and
are ordered by eligibility time, repository name, run ID, then job ID.

Immediately before JIT creation, the participant fetches the job again. It proceeds
only if the job is still queued, all required labels are supported locally, the
repository is still private and allowlisted, and local capacity remains available.

### JIT runner lifecycle

The operator provides one clean, unconfigured official runner template directory.
For each eligible job the participant copies it into a unique directory below the
managed state root, requests repository JIT configuration, stores a non-secret JSON
manifest atomically, and launches `run.sh` or `run.cmd`. JIT configuration is passed
through `ACTIONS_RUNNER_INPUT_JITCONFIG`; it is never written to the manifest or log.

The runner name contains the participant name and a random suffix. Assignment is
confirmed when GitHub reports a matching in-progress job with that runner name. An
unassigned runner is terminated after the acquisition timeout. Completed or dead
runners have their directory removed. Cleanup failures remain in the manifest and
are retried before new capacity is offered. Recursive cleanup never follows symbolic
links or Windows reparse points; it removes the link object itself and rejects any
target whose directory ancestry escapes the configured state root.

### Recovery

At startup the participant scans only its configured state root. Each manifest is
reconciled with process liveness and GitHub job state. A live runner counts against
capacity; a completed/dead runner is cleaned; an overdue unassigned runner is
terminated only after its PID, operating-system-reported process start marker, and
executable path inside its instance directory all match the manifest. A mismatched or
unverifiable PID is never signaled and is reported for operator recovery. Invalid or
unexpected directories are reported but never recursively removed. All destructive
cleanup targets must resolve below the configured state root without traversing
symbolic links or Windows reparse points.

### Configuration and secrets

Configuration is strict YAML: unknown fields and duplicate repository names fail
validation. The PAT is loaded from a separate file whose path is configured; literal
tokens in YAML are unsupported. Check mode validates configuration, token access,
private repository visibility, runner template contents, state-root safety, and
labels without creating JIT runners or processes. The template and state-root
ancestry must not contain symbolic links or Windows reparse points.

### Packaging and service operation

Release artifacts are single `linux-amd64` and `windows-amd64` executables. The
program runs in the foreground and handles termination signals. Operators use
systemd or Windows service/task tooling externally; v1 does not implement a service
installer.

## Project Structure

### Documentation (this feature)

```text
specs/001-on-demand-runner-dispatch/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── configuration.md
│   └── github-rest.md
└── tasks.md
```

### Source Code (repository root)

```text
cmd/runner-participant/
└── main.go

internal/
├── config/
│   ├── config.go
│   └── config_test.go
├── github/
│   ├── client.go
│   └── client_test.go
├── participant/
│   ├── participant.go
│   └── participant_test.go
├── runner/
│   ├── manager.go
│   ├── process_linux.go
│   ├── process_windows.go
│   └── manager_test.go
└── setup/
    ├── setup.go
    └── setup_test.go

test/
└── integration/
    └── participant_test.go

config.example.yml
go.mod
go.sum
```

**Structure Decision**: Use one Go module and five internal packages. Tests remain
next to their packages except for the single end-to-end fake-GitHub integration
test. No interface layer is introduced unless required to replace time, HTTP, or
process execution in tests.

## Complexity Tracking

No constitution violations or complexity exceptions.
