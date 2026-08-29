# Feature Specification: On-Demand Runner Dispatch

**Feature Branch**: `001-on-demand-runner-dispatch`

**Created**: 2026-08-29

**Status**: Draft

**Input**: User description: "Share limited self-hosted capacity across private repositories owned by a personal account by starting only the runner service that currently has workflow work. Prefer one host and fall back to another without keeping every runner process resident."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Run queued work on demand (Priority: P1)

An operator configures private repositories and their registered runner services.
When workflow work is queued for one repository, the corresponding service starts,
accepts the work, and stops after the repository becomes idle.

**Why this priority**: This removes idle runner resource usage while preserving normal CI behavior.

**Independent Test**: Present queued work for one configured repository and verify that only its runner service starts, remains active through completion, and stops after the idle grace period.

**Acceptance Scenarios**:

1. **Given** all runner services are stopped, **When** work is queued for a configured repository, **Then** its runner service starts within the polling interval plus 30 seconds.
2. **Given** a runner is processing work, **When** the next observation occurs, **Then** the service remains active.
3. **Given** no work is queued or active, **When** the idle grace period expires, **Then** the runner service stops.

---

### User Story 2 - Prefer one host with safe fallback (Priority: P2)

An operator orders eligible hosts for each repository. The dispatcher tries the
preferred host first and uses the next available host only when the preferred host
cannot accept work in time.

**Why this priority**: Host preference uses the fastest machine while retaining CI when it is offline.

**Independent Test**: Queue work while the preferred host is unavailable and verify that the fallback accepts it; restore the preferred host and verify that the active job is not moved.

**Acceptance Scenarios**:

1. **Given** the preferred host is available, **When** work is queued, **Then** that host is selected.
2. **Given** the preferred host is unavailable past the configured timeout, **When** a fallback is available, **Then** the fallback service starts.
3. **Given** a fallback already accepted a job, **When** the preferred host returns, **Then** the job continues on the fallback until completion.

---

### User Story 3 - Recover and explain decisions (Priority: P3)

An operator can restart the dispatcher or recover from a network interruption without
creating competing runners or stranding work, and can understand every decision from logs.

**Why this priority**: Unattended CI must fail safely and remain diagnosable.

**Independent Test**: Restart during queued, active, and idle states and verify reconciliation, idempotent control, and secret-free decision logs.

**Acceptance Scenarios**:

1. **Given** a runner service is already active, **When** the dispatcher restarts, **Then** it adopts the observed state without starting a duplicate.
2. **Given** a control action is repeated, **When** the service already has the requested state, **Then** the outcome remains unchanged and is recorded as successful reconciliation.
3. **Given** an authentication, network, or control failure, **When** dispatch cannot proceed, **Then** the failure identifies the repository, host, and safe next action without exposing credentials.

### Edge Cases

- Multiple repositories receive queued work during the same polling interval.
- One workflow contains several sequential or parallel jobs.
- A queued run is cancelled before a runner accepts it.
- A host becomes unreachable after its service starts but before assignment.
- A runner remains active after the hosting platform reports completion.
- The platform repeats or temporarily omits a workflow observation.
- Configuration references an unknown repository, host, or service.
- Public-repository work appears while public repositories are disabled.
- Authentication expires or the platform rate limit is reached.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Operators MUST be able to allowlist repositories, eligible hosts, runner services, host priority, polling interval, fallback timeout, idle grace period, and concurrency limit.
- **FR-002**: The dispatcher MUST use queued and active workflow work as the source of truth rather than independently interpreting commits, pull requests, or issues.
- **FR-003**: The dispatcher MUST start a repository runner only when allowlisted workflow work requires it.
- **FR-004**: The dispatcher MUST keep a runner active while matching work is queued or active and stop it after the idle grace period.
- **FR-005**: The dispatcher MUST select queued work deterministically and enforce a default limit of one active repository runner per host.
- **FR-006**: The dispatcher MUST attempt hosts in configured priority order and use a fallback only after the prior host is unavailable or fails to accept work within its timeout.
- **FR-007**: The dispatcher MUST NOT preempt or migrate work already accepted by a fallback host.
- **FR-008**: Startup and periodic reconciliation MUST discover actual service state and converge safely without duplicate starts or harmful repeated stops.
- **FR-009**: Repository observation MUST use credentials limited to selected repositories and read-only workflow metadata.
- **FR-010**: Remote service control MUST operate without unrestricted root login and MUST reject services outside the configured allowlist.
- **FR-011**: Public repositories MUST be rejected by default and require explicit operator opt-in.
- **FR-012**: Logs MUST record observations, selection, start, fallback, adoption, stop, and failure decisions while redacting credentials.
- **FR-013**: A validation mode MUST report intended actions without changing service state.
- **FR-014**: Invalid or incomplete configuration MUST prevent startup and identify every offending field.
- **FR-015**: The dispatcher MUST tolerate temporary authentication, platform, host, and service-control failures without losing its ability to retry queued work.

### Key Entities

- **Repository**: An allowlisted private source repository whose workflow work may require a runner.
- **Host**: A machine capable of running one or more registered repository runner services, including its priority and availability.
- **Runner Service**: A pre-registered, normally stopped service associated with exactly one repository on one host.
- **Workflow Work**: Queued or active work reported by the hosting platform, including repository, creation time, and current state.
- **Dispatch Decision**: The selected repository, host, action, reason, timestamps, and outcome used for reconciliation and diagnosis.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: With ten configured repositories and no workflow work, no runner process remains active after the idle grace period.
- **SC-002**: New queued work begins runner activation within one polling interval plus 30 seconds in at least 99% of healthy-host observations.
- **SC-003**: At most the configured number of repository runners is active on a host during all tested concurrent-queue scenarios.
- **SC-004**: Preferred-host failure results in fallback activation within the configured timeout plus one polling interval.
- **SC-005**: Restart and duplicate-observation tests produce no duplicate runner activation and strand no queued work.
- **SC-006**: All decision and failure logs pass automated secret-redaction checks.
- **SC-007**: An operator can add a repository and two ordered hosts through configuration alone in under ten minutes, excluding runner registration.

## Assumptions

- Runner software is registered separately for each repository and host before dispatching begins.
- Target hosts provide a service manager and unprivileged remote-control mechanism.
- A small always-on controller can reach the hosting platform and configured hosts.
- The first release targets private repositories owned by individual accounts; organization-wide native runner pools remain out of scope.
- The hosting platform retains queued work long enough for periodic polling and host fallback.
- One active repository runner per host is the safe default; operators may raise the limit explicitly.
