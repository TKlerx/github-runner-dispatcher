# Implementation Plan: Configurable Trusted-Workflow Policies

**Branch**: `002-trusted-workflow-policies` | **Date**: 2026-08-30 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/002-trusted-workflow-policies/spec.md`

## Summary

Extend each repository configuration with optional trusted workflows and event/actor/label
rules. Enrich GitHub workflow-run metadata, authorize policy-governed work during
observation, then re-fetch the job and run and repeat the full decision immediately
before JIT creation. Legacy private repositories remain label-gated. Public repositories
are accepted only with explicit policies. Add one atomic file-based policy CLI used by
external onboarding without embedding repository-specific behavior.

## Technical Context

**Language/Version**: Go 1.27

**Primary Dependencies**: Go standard library; existing `go.yaml.in/yaml/v4`

**Storage**: Strict YAML participant configuration and existing local runner manifests

**Testing**: `go test ./...`, `go test -race ./...`, `go vet ./...`

**Target Platform**: Linux amd64 and Windows amd64

**Project Type**: Single command-line service

**Performance Goals**: Preserve the existing polling cadence and add at most one workflow-run request per final offer; policy matching is linear in the small configured rule set.

**Constraints**: Fail closed; use only GitHub REST metadata; no Qwen constants; no new dependency; no workflow-content claims; one-job JIT lifecycle remains unchanged.

**Scale/Scope**: Tens of repositories and trusted workflows per participant, local capacity 1-4.

## Constitution Check

*GATE: Passed before research and after design.*

- [x] Runtime remains limited to observation and official JIT capacity; policy CLI mutates only the local config.
- [x] Credentials remain per participant and limited to explicitly selected repositories; public repositories require explicit policy.
- [x] Participants remain independent; final job and run rechecks precede JIT creation.
- [x] Existing one-job execution, timeout, reconciliation, registration cleanup, and local cleanup remain in place.
- [x] Every new policy boundary and requested terminal cleanup outcome has focused automated coverage.

The constitution is amended from 2.1.0 to 2.2.0 because the original least-privilege
wording assumed every selected repository was private. The amendment permits explicitly
selected public repositories only behind mandatory trusted-workflow policy.

## Project Structure

### Documentation (this feature)

```text
specs/002-trusted-workflow-policies/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── configuration-cli.md
│   └── github-rest.md
└── tasks.md
```

### Source Code (repository root)

```text
cmd/runner-participant/       # CLI flags, check mode, runtime wiring
internal/config/              # strict policy schema and validation
internal/github/              # server-observed repository/job/run metadata
internal/participant/         # policy evaluation and final authorization
internal/runner/              # unchanged JIT lifecycle and cleanup
internal/setup/               # existing selector plus atomic policy mutation
test/integration/             # lifecycle and end-to-end participant behavior
```

**Structure Decision**: Extend the existing packages at their current trust boundaries.
Policy matching belongs in `participant`; YAML mutation reuses `setup`'s atomic writer.

## Complexity Tracking

No constitution exception or new abstraction is required.
