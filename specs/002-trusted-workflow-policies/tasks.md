# Tasks: Configurable Trusted-Workflow Policies

**Input**: Design documents from `specs/002-trusted-workflow-policies/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: Security boundaries, configuration mutation, final rechecks, and every requested cleanup outcome require focused automated tests.

## Phase 1: Specification and Governance

- [x] T001 Create and validate feature specification, plan, research, data model, contracts, quickstart, and constitution amendment in `specs/002-trusted-workflow-policies/`, `.specify/memory/constitution.md`, `.specify/templates/plan-template.md`, and `AGENTS.md`

---

## Phase 2: Foundational Policy Data

- [x] T002 Add strict visibility, trusted-workflow, and authorization-rule configuration types and validation with backward-compatibility tests in `internal/config/config.go` and `internal/config/config_test.go`
- [x] T003 Add complete server-observed repository and workflow-run metadata retrieval with REST contract tests in `internal/github/client.go` and `internal/github/client_test.go`

---

## Phase 3: User Story 1 - Authorize Trusted Workflow Jobs (Priority: P1) 🎯 MVP

**Goal**: Match workflow identity, event, actors, and required labels using only GitHub metadata.

**Independent Test**: Unknown workflow, unauthorized coding/fix actor, wildcard PR actor, and restricted repository-dispatch actor cases produce the expected allow/deny decisions.

- [x] T004 [US1] Add fail-closed policy evaluation and focused Qwen-shaped-but-generic authorization tests in `internal/participant/policy.go`, `internal/participant/policy_test.go`, and `internal/participant/types.go`
- [x] T005 [US1] Apply initial policy authorization during observation without changing policy-free private behavior in `internal/participant/participant.go` and `internal/participant/participant_test.go`

---

## Phase 4: User Story 2 - Revalidate Before JIT (Priority: P1)

**Goal**: Fetch fresh job and workflow-run metadata and deny any missing, changed, or inconsistent state before JIT creation.

**Independent Test**: Final metadata errors or differences cause zero JIT calls; a fully consistent authorized job creates one JIT configuration.

- [ ] T006 [US2] Add immediate job/run re-fetch, consistency checks, repeated policy evaluation, and integration tests in `internal/participant/participant.go`, `internal/participant/race_test.go`, and `test/integration/participant_test.go`

---

## Phase 5: User Story 3 - Govern Public Repositories (Priority: P2)

**Goal**: Permit explicit-policy public repositories while rejecting ungoverned or visibility-inconsistent public repositories.

**Independent Test**: Public without policy is invalid; configured and GitHub visibility mismatches fail; private legacy configuration passes.

- [ ] T007 [US3] Replace private-only repository validation with policy-aware visibility checks in `internal/github/client.go`, `cmd/runner-participant/main.go`, and their tests

---

## Phase 6: User Story 4 - Manage Policies Non-interactively (Priority: P3)

**Goal**: Atomically add, reconcile, and remove one complete repository policy entry while preserving unrelated configuration.

**Independent Test**: All three operations, idempotent reconciliation, invalid-input rollback, and retained interactive policy entries pass against temporary files.

- [ ] T008 [US4] Add strict repository-policy file parsing and atomic add/reconcile/remove operations while preserving retained setup entries in `internal/setup/policy.go`, `internal/setup/setup.go`, and tests
- [ ] T009 [US4] Expose mutually exclusive policy CLI flags and exit behavior in `cmd/runner-participant/main.go` and `cmd/runner-participant/main_test.go`

---

## Phase 7: User Story 5 - Preserve Lifecycle Cleanup (Priority: P1)

**Goal**: Prove local and GitHub cleanup for completed, failed, cancelled, and timed-out runners.

**Independent Test**: Every terminal outcome leaves no runner directory and invokes registration cleanup once.

- [ ] T010 [US5] Add focused terminal-outcome cleanup coverage and fix only discovered lifecycle gaps in `internal/runner/manager_test.go` and `test/integration/participant_test.go`

---

## Phase 8: Documentation and Validation

- [ ] T011 Update generic examples and security language without Qwen-specific constants in `config.example.yml`, `README.md`, `SECURITY.md`, and `docs/operations.md`
- [ ] T012 Run formatting, vet, unit, race, and build validation; verify no Qwen constants and complete all task checkboxes in `specs/002-trusted-workflow-policies/tasks.md`

## Dependencies & Execution Order

- T002 and T003 establish the data boundary and must precede policy integration.
- T004 depends on T002 and T003; T005 depends on T004.
- T006 depends on T005; T007 depends on T002 and T003.
- T008 depends on T002; T009 depends on T008.
- T010 may proceed after T006; documentation and final validation follow all code tasks.

## Implementation Strategy

Implement in task order. Each task includes its focused tests, is validated before its
checkbox is marked, and is committed independently. No Qwen-specific value enters source,
defaults, or public examples.
