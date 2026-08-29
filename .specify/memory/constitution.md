<!--
Sync Impact Report
- Version change: 2.0.0 -> 2.1.0
- Modified principles: Minimal Participant, Least Privilege, Tests Define Done
- Added sections: none
- Removed sections: none
- Templates updated: .specify/templates/plan-template.md,
  .specify/templates/tasks-template.md
- Runtime guidance updated: README.md, SECURITY.md
- Follow-up TODOs: none
-->
# GitHub Runner Dispatcher Constitution

## Core Principles

### I. Minimal Participant
The runtime participant MUST only observe queued GitHub Actions jobs and offer
temporary JIT runner capacity for matching jobs. It MUST reuse GitHub's job
assignment, official runner software, and native process management. It MUST NOT
trigger, retry, cancel, or execute workflow logic itself. Setup tooling MAY read
repository and workflow metadata and generate local configuration or a PAT form URL,
but MUST NOT create tokens or mutate GitHub state.

### II. Least Privilege
Each participant MUST use its own fine-grained PAT restricted to explicitly selected
private repositories. Permissions MUST be limited to Metadata read, Actions read,
and Administration write as required by GitHub's JIT runner API. Secrets and encoded
JIT configuration MUST never appear in committed configuration, logs, process
arguments under project control, or error messages. Setup MUST NOT reuse, print, or
persist GitHub CLI authentication as the participant credential.

### III. Independent Participation
Participants MUST operate without a central coordinator, distributed lock, peer
discovery, or peer communication. Local claim delay and capacity MUST be explicit.
Job state MUST be rechecked immediately before offering capacity. Redundant JIT
runners are acceptable, but the service MUST never duplicate a workflow job.

### IV. Recoverable Ephemeral Execution
Every JIT runner MUST process at most one job in a unique temporary directory.
Participants MUST reconcile locally launched processes after startup or interruption,
enforce local capacity, terminate unassigned runners after timeout, and retry failed
cleanup before offering new capacity.

### V. Tests Define Done
Every state transition, security boundary, and failure path MUST have an automated
test. Tests MUST cover matching queued work, label rejection, claim delay, final
recheck, JIT creation, capacity, redundant contenders, restart reconciliation,
timeouts, cleanup, and secret redaction on Windows and Linux where behavior differs.
Setup tests MUST cover selection pagination, existing-config protection, workflow
status warnings, and secret-free PAT URL generation. A change is not complete while
its tests or static checks fail.

## Security and Runtime Constraints

- The first release MUST support private repositories only.
- Repository names, participant identity, labels, timing values, and capacity MUST be
  configuration rather than project-specific constants.
- Participants MUST advertise only their actual operating system and architecture.
- Runner software MUST be installed once per participant; repositories MUST NOT need
  persistent runner registration or repository-specific services.
- Runtime dependencies MUST remain minimal and justified. Native platform features
  take precedence over new infrastructure. GitHub CLI MAY be required for setup but
  MUST NOT be required during normal participant execution.
- Logs MUST reconstruct local participation decisions without exposing secrets.

## Development Workflow

Development follows Spec Kit phases: specify, plan, tasks, implement, and validate.
Each implementation task includes its smallest relevant automated test. Pull
requests MUST document constitution compliance, pass all checks, and update public
documentation when behavior or configuration changes.

## Governance

This constitution supersedes conflicting project guidance. Amendments require a
documented rationale, an appropriate semantic version change, and propagation to
affected templates and specifications. Every plan and review MUST verify the five
core principles. Complexity exceptions MUST name the rejected simpler alternative
and the evidence that requires the exception.

**Version**: 2.1.0 | **Ratified**: 2026-08-29 | **Last Amended**: 2026-08-29
