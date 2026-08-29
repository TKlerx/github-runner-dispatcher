# Tasks: Decentralized On-Demand Runner Participation

**Input**: Design documents from `specs/001-on-demand-runner-dispatch/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/`

**Tests**: Tests are mandatory and are written before their corresponding implementation tasks.

**Organization**: Tasks are grouped by user story so each increment has an independent test.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel because it changes different files without an incomplete dependency.
- **[Story]**: Maps the task to a user story from `spec.md`.

## Phase 1: Setup

**Purpose**: Establish the smallest buildable Go project and public CI.

- [x] T001 Initialize the Go 1.27 module and `go.yaml.in/yaml/v4` dependency in `go.mod` and `go.sum`
- [x] T002 [P] Create the documented non-secret participant example in `config.example.yml`
- [x] T003 [P] Add formatting, vet, unit-test, race-test, and Windows/Linux build checks in `.github/workflows/ci.yml`

---

## Phase 2: Foundational

**Purpose**: Implement configuration, GitHub REST access, and check mode required by every story.

**Checkpoint**: Configuration and GitHub access can be validated without creating a runner or process.

- [x] T004 [P] Write strict YAML, duration, path-safety including symlink/reparse-point ancestry, duplicate-repository, common-owner, actual-label, and token-file tests in `internal/config/config_test.go`
- [x] T005 Implement typed configuration, defaults, aggregate validation, and PAT-file loading in `internal/config/config.go`
- [x] T006 [P] Write REST contract tests for private-repository validation, pagination, queued/in-progress run discovery, job labels, final job lookup, JIT creation, permission errors, and secret-free errors in `internal/github/client_test.go`
- [x] T007 Implement the bounded standard-library GitHub REST client described by `contracts/github-rest.md` in `internal/github/client.go`
- [x] T008 [P] Define observed-job, repository, runner-manifest including process identity, phase, participation-decision, and platform-neutral process-control types in `internal/participant/types.go`, `internal/runner/manifest.go`, and `internal/runner/process.go`
- [x] T009 Write CLI tests for side-effect-free `-check` plus setup pagination, archived/no-active-workflow/unknown marking, numbered multi-selection, existing-config warning, keep/cancel byte preservation, atomic allowlist-only replacement, common-owner rejection, PAT URL parameters, repository checklist, and zero token access in `cmd/runner-participant/main_test.go` and `internal/setup/setup_test.go`
- [x] T010 Implement `-config`, side-effect-free `-check`, and setup-only GitHub CLI repository/workflow discovery, existing-config protection, atomic allowlist replacement, and PAT-link behavior with documented exit codes in `cmd/runner-participant/main.go` and `internal/setup/setup.go`

---

## Phase 3: User Story 1 - Run queued work on demand (Priority: P1) MVP

**Goal**: A participant observes one matching queued job, launches one disposable JIT runner, and removes it after completion or acquisition timeout.

**Independent Test**: A fake GitHub server exposes one matching queued job; the participant creates exactly one JIT runner, executes a fake official runner once, and removes its directory.

### Tests for User Story 1

- [ ] T011 [P] [US1] Write runner-template copy, unique-directory, masked child environment, single-job exit, timeout termination, capacity, and safe-cleanup tests in `internal/runner/manager_test.go`
- [ ] T012 [P] [US1] Write queued-job matching, case-insensitive label subset, deterministic ordering, and unmatched OS rejection tests in `internal/participant/participant_test.go`

### Implementation for User Story 1

- [ ] T013 [P] [US1] Implement native runner process start, identity inspection, liveness, termination, and `run.sh` selection in `internal/runner/process_linux.go`
- [ ] T014 [P] [US1] Implement native runner process start, identity inspection, liveness, termination, and `run.cmd` selection in `internal/runner/process_windows.go`
- [ ] T015 [US1] Implement atomic manifests, disposable runner copies, JIT environment launch, acquisition timeout, capacity accounting, and bounded cleanup in `internal/runner/manager.go`
- [ ] T016 [US1] Implement polling, label matching, deterministic queued-job selection, JIT creation, and graceful shutdown in `internal/participant/participant.go` and wire normal mode in `cmd/runner-participant/main.go`
- [ ] T017 [US1] Add a fake-GitHub/fake-runner end-to-end MVP test in `test/integration/participant_test.go`

**Checkpoint**: User Story 1 runs independently with no persistent repository runner registration.

---

## Phase 4: User Story 2 - Prefer stronger participants without coordination (Priority: P2)

**Goal**: Local claim delays make stronger participants normally offer capacity first while delayed participants provide fallback.

**Independent Test**: Two participant instances share a fake GitHub queue; the short-delay instance normally receives the job, and the long-delay instance receives it when the first is absent.

### Tests for User Story 2

- [ ] T018 [P] [US2] Write fake-clock tests for local first-seen timestamps, claim eligibility, stale observation eviction, deterministic tie-breaking, and at least 100 healthy timing observations meeting SC-002 in `internal/participant/claim_test.go` and `internal/participant/timing_test.go`
- [ ] T019 [P] [US2] Write final-recheck and redundant-contender tests proving completed/assigned jobs cause no JIT POST and one GitHub job is never retriggered in `internal/participant/race_test.go`

### Implementation for User Story 2

- [ ] T020 [US2] Implement bounded first-seen tracking, claim-delay eligibility, stale eviction, and deterministic ordering in `internal/participant/claim.go`
- [ ] T021 [US2] Add immediate final job recheck and capacity recheck before JIT creation in `internal/participant/participant.go`
- [ ] T022 [US2] Add two-participant preference, fallback, and harmless redundant-listener integration scenarios in `test/integration/participant_test.go`

**Checkpoint**: User Story 2 demonstrates best-effort machine preference without coordination.

---

## Phase 5: User Story 3 - Recover and explain decisions (Priority: P3)

**Goal**: Restarts, transient API failures, stale children, and cleanup failures recover safely and remain diagnosable without leaking secrets.

**Independent Test**: Restart around waiting, assigned, dead, and cleanup-failed manifests; verify capacity, process actions, retries, and redacted decision logs.

### Tests for User Story 3

- [ ] T023 [P] [US3] Write atomic-manifest and startup-reconciliation tests for waiting, assigned, dead, overdue, malformed, unknown directories, reused PIDs, and mismatched or unverifiable process identities in `internal/runner/reconcile_test.go`
- [ ] T024 [P] [US3] Write destructive-path containment, symlink/junction no-follow, and interrupted-cleanup retry tests in `internal/runner/cleanup_test.go`
- [ ] T025 [P] [US3] Write rate-limit, `Retry-After`, transient network/5xx backoff, cancellation, and PAT/JIT redaction tests in `internal/github/retry_test.go` and `internal/participant/log_test.go`

### Implementation for User Story 3

- [ ] T026 [US3] Implement atomic manifest persistence and startup reconciliation with PID/start-marker/executable verification and no-follow cleanup confined to the configured state root in `internal/runner/manifest.go` and `internal/runner/manager.go`
- [ ] T027 [US3] Implement bounded retry scheduling for GitHub rate limits and transient failures in `internal/github/client.go`
- [ ] T028 [US3] Implement structured participation-decision logging and centralized secret redaction in `internal/participant/log.go`
- [ ] T029 [US3] Add restart-during-waiting, restart-during-assignment, and cleanup-recovery scenarios in `test/integration/participant_test.go`

**Checkpoint**: All user stories pass independently and together.

---

## Phase 6: Polish and Cross-Cutting Validation

**Purpose**: Publish usable artifacts and execute the complete quality gate.

- [ ] T030 [P] Document interactive repository selection, the manual browser repository checklist, Linux systemd and Windows startup examples, PAT setup, runner-template updates, trusted-host limitations, and the timed SC-007 onboarding check in `docs/operations.md` and `SECURITY.md`
- [ ] T031 [P] Add reproducible `linux-amd64` and `windows-amd64` release builds with checksums in `.github/workflows/release.yml`
- [ ] T032 Run `gofmt`, `go vet`, `go test -race ./...`, both target builds, secret-pattern scans, paginated setup selection, the SC-002 timing test, and the timed SC-007 scenario in `specs/001-on-demand-runner-dispatch/quickstart.md`; record any operational corrections in `README.md`

---

## Dependencies and Execution Order

### Phase dependencies

- Setup has no dependencies.
- Foundational depends on Setup and blocks all user stories.
- User Story 1 depends on Foundational and is the MVP.
- User Story 2 depends on the User Story 1 polling loop but remains independently testable through its claim/race tests.
- User Story 3 depends on the User Story 1 runner lifecycle but remains independently testable through manifest/retry tests.
- Polish depends on all selected user stories.

### Parallel opportunities

- T002 and T003 can run alongside module initialization.
- T004, T006, and T008 touch separate foundational packages.
- T011 and T012 can be written in parallel before User Story 1 implementation.
- Linux and Windows process implementations T013 and T014 are independent after T008 defines their shared contract; T015 depends on both.
- T018 and T019 can be written in parallel before claim implementation.
- T023, T024, and T025 cover independent recovery boundaries.
- Documentation and release work T030 and T031 are independent.

## Parallel Example: User Story 1

```text
T011: runner lifecycle tests in internal/runner/manager_test.go
T012: job matching tests in internal/participant/participant_test.go
T013: Linux process control in internal/runner/process_linux.go
T014: Windows process control in internal/runner/process_windows.go
```

## Implementation Strategy

### MVP first

1. Complete Setup and Foundational.
2. Complete User Story 1 through T017.
3. Validate one repository with a fake server before using a real PAT.
4. Stop here if on-demand one-job execution is sufficient.

### Incremental delivery

1. Add claim-delay preference and fallback in User Story 2.
2. Add durable reconciliation and diagnostic hardening in User Story 3.
3. Add operator/release documentation only after behavior is proven.

## Notes

- Every test task precedes the implementation it constrains.
- No task adds a database, coordinator, peer protocol, web server, or service installer.
- Real credentials are used only for manual check-mode and final controlled validation.
- Complete and validate one unchecked task at a time during implementation.
