# Feature Specification: Decentralized On-Demand Runner Participation

**Feature Branch**: `001-on-demand-runner-dispatch`

**Created**: 2026-08-29

**Status**: Draft

**Input**: User description: "Let trusted Windows and Linux machines independently contribute temporary self-hosted capacity to allowlisted private repositories, with stronger machines usually taking work first and no persistent per-repository runner setup."

## Clarifications

### Session 2026-08-29

- Q: Which queued work should activate a repository runner? → A: Only queued jobs whose required labels match the configured runner.
- Q: How should machines coordinate and provision repository runners? → A: Each trusted machine independently polls GitHub, applies a local claim delay, and launches a one-job JIT runner without peer coordination or persistent per-repository registration.
- Q: How should each participant authenticate to GitHub? → A: Each participant uses its own fine-grained PAT restricted to the selected repositories.
- Q: How should consecutive jobs be isolated on a participant? → A: Each JIT runner uses a unique temporary runner and work directory that is removed after exit.
- Q: Which repository visibility should the first release support? → A: Private repositories only.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Run queued work on demand (Priority: P1)

An operator installs one participant service on a trusted machine and configures its
private-repository allowlist, actual runner labels, capacity, and claim delay. When a
matching job remains queued, the participant creates a one-job JIT runner, which exits
and deregisters after completing the job.

**Why this priority**: This removes idle runner resource usage while preserving normal CI behavior.

**Independent Test**: Present matching queued work for one allowlisted repository and verify that the participant creates a JIT runner without prior repository setup, processes at most one job, and leaves no runner process afterward.

**Acceptance Scenarios**:

1. **Given** no runner is active, **When** a label-compatible job is queued for an allowlisted repository and the local claim delay elapses, **Then** a JIT runner starts within one polling interval plus 30 seconds.
2. **Given** a JIT runner accepts a job, **When** the job completes, **Then** that runner exits, cannot accept a second job, and its temporary directory is removed.
3. **Given** a JIT runner receives no job before the acquisition timeout, **When** no matching work remains queued, **Then** the participant terminates it.

---

### User Story 2 - Prefer stronger participants without coordination (Priority: P2)

Each participant has a local claim delay. Stronger machines use shorter delays and
therefore normally offer capacity first; slower fallback machines offer capacity only
when the job remains queued. Participants do not discover or communicate with peers.

**Why this priority**: Host preference uses the fastest machine while retaining CI when it is offline.

**Independent Test**: Run two participants with different claim delays, then verify that the shorter-delay participant normally accepts work while the longer-delay participant takes work when the first is unavailable.

**Acceptance Scenarios**:

1. **Given** two healthy compatible participants with different claim delays, **When** work is queued, **Then** the shorter-delay participant becomes eligible first.
2. **Given** the shorter-delay participant is unavailable, **When** the longer delay elapses and the job is still queued, **Then** the fallback participant offers a JIT runner.
3. **Given** multiple participants race after their final recheck, **When** GitHub assigns the job to one runner, **Then** the others do not execute that job and exit after their acquisition timeout if no other matching job exists.

---

### User Story 3 - Recover and explain decisions (Priority: P3)

An operator can restart a participant or recover from a network interruption without
stranding work, and can understand its local decisions from logs.

**Why this priority**: Unattended CI must fail safely and remain diagnosable.

**Independent Test**: Restart during queued and active states and verify safe recovery, bounded redundant runner startup, and secret-free decision logs.

**Acceptance Scenarios**:

1. **Given** a locally launched runner is still active, **When** the participant restarts, **Then** it reconciles that process before offering more local capacity.
2. **Given** another participant accepts a previously observed job, **When** the local claim delay expires, **Then** the final recheck prevents unnecessary runner startup whenever GitHub already reports the assignment.
3. **Given** an authentication, network, or runner-launch failure, **When** participation cannot proceed, **Then** the failure identifies the repository, participant, and safe next action without exposing credentials.

### Edge Cases

- Multiple allowlisted repositories receive queued work during the same polling interval.
- One workflow contains several sequential or parallel jobs.
- A queued run is cancelled before a runner accepts it.
- Multiple participants create compatible JIT runners during the same assignment race.
- A participant shuts down after its JIT runner starts but before assignment.
- A JIT runner remains idle after the hosting platform reports no matching queued work.
- The platform repeats or temporarily omits a workflow observation.
- Configuration references an unknown repository or invalid runner label.
- Authentication expires or the platform rate limit is reached.
- A Windows participant observes a Linux-only job, or vice versa.
- Temporary-directory cleanup is interrupted or initially fails.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Operators MUST be able to configure an explicit private-repository allowlist, participant name, actual operating-system and architecture labels, custom labels, polling interval, claim delay, acquisition timeout, and local capacity limit.
- **FR-002**: Each participant MUST use queued and active workflow jobs as the source of truth rather than independently interpreting commits, pull requests, or issues.
- **FR-003**: A participant MUST create a repository-scoped JIT runner only for an allowlisted queued job whose required labels match that participant.
- **FR-004**: Each JIT runner MUST process at most one job in a unique temporary runner and work directory; it MUST exit and remove that directory after completion, while an unassigned runner MUST be terminated and cleaned after the acquisition timeout when no matching work remains queued.
- **FR-005**: A participant MUST select matching queued work deterministically and enforce a default local capacity of one active JIT runner.
- **FR-006**: Each participant MUST operate independently without a central coordinator, peer discovery, or peer communication.
- **FR-007**: A participant MUST wait until the job age reaches its local claim delay and MUST re-fetch job state immediately before creating a JIT runner.
- **FR-008**: Concurrent participants MAY create redundant JIT runners, but the service MUST NOT trigger, rerun, or otherwise duplicate workflow jobs.
- **FR-009**: Startup and periodic reconciliation MUST discover locally launched runner processes and converge safely without exceeding local capacity.
- **FR-010**: Each participant MUST use its own fine-grained PAT limited to selected repositories, with Metadata read, Actions read, and Administration write permissions required to observe jobs and create JIT runners.
- **FR-011**: Participants MUST reject public repositories; the first release MUST support private repositories only.
- **FR-012**: Logs MUST record observations, eligibility timing, final rechecks, JIT creation, assignment, completion, timeout, and failures while redacting credentials and JIT configuration.
- **FR-013**: A validation mode MUST report intended actions without creating JIT runners or changing process state.
- **FR-014**: Invalid or incomplete configuration MUST prevent startup and identify every offending field.
- **FR-015**: Participants MUST tolerate temporary authentication, platform, and runner-launch failures without losing the ability to retry queued work.
- **FR-016**: The participant service MUST run on supported Windows and Linux hosts and MUST advertise only the host's actual operating-system and architecture labels.
- **FR-017**: A participant MUST ignore jobs whose required labels do not match the participant, including Linux-only jobs observed by Windows participants.
- **FR-018**: Failed temporary-directory cleanup MUST be logged and retried during startup reconciliation before new local capacity is offered.

### Key Entities

- **Repository**: An allowlisted private source repository whose workflow work may require a runner.
- **Participant**: One trusted Windows or Linux machine running the service, including its verified labels, claim delay, and local capacity.
- **JIT Runner**: A temporary repository-scoped runner created from just-in-time configuration, capable of processing at most one job in its own disposable directory.
- **Workflow Work**: A queued or active job reported by the hosting platform, including repository, required labels, creation time, and current state.
- **Participation Decision**: The observed job, eligibility time, final state check, local action, reason, timestamps, and outcome used for reconciliation and diagnosis.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: With ten configured repositories and no matching queued work, no JIT runner process remains active after its acquisition timeout.
- **SC-002**: Matching queued work begins runner activation within the local claim delay plus one polling interval and 30 seconds in at least 99% of healthy observations.
- **SC-003**: At most the configured number of JIT runners is active on a participant during all tested concurrent-queue scenarios.
- **SC-004**: When a shorter-delay participant is unavailable, a healthy fallback begins runner activation within its claim delay plus one polling interval and 30 seconds.
- **SC-005**: Concurrent-participant and restart tests execute each GitHub job at most once and leave no unassigned local runner beyond its acquisition timeout.
- **SC-006**: All decision and failure logs pass automated secret-redaction checks.
- **SC-007**: An operator can add an allowlisted repository or a Windows/Linux participant through configuration alone in under ten minutes, excluding the one-time runner software installation.

## Out of Scope

- Triggering, retrying, or cancelling workflow runs.
- Guaranteeing an exact machine choice when compatible participants race.
- A central coordinator, distributed lock, or participant-to-participant protocol.
- Making a Linux-only workflow executable on Windows or vice versa.
- Automatically enrolling every private repository accessible to an account.
- Public repositories, including any opt-in or trusted-event mode.

## Assumptions

- Runner software is installed once on each participant; repository registration is created on demand through GitHub's JIT runner API.
- Participants are trusted because private-repository jobs may expose source code and workflow secrets.
- Operators create a separate fine-grained PAT for each participant so one machine can be revoked without disrupting the others.
- GitHub assigns each queued job to at most one connected runner even when compatible runners race.
- The first release targets private repositories owned by individual accounts; organization-wide native runner pools remain out of scope.
- The hosting platform retains queued work long enough for polling and delayed fallback participation.
- One active JIT runner per participant is the safe default; operators may raise the limit explicitly.
- Existing Linux-only workflows remain Linux-only; Windows support applies to the participant service and label-compatible jobs.
