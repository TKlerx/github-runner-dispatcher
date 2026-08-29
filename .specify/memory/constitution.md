<!--
Sync Impact Report
- Version change: template -> 1.0.0
- Added principles: Minimal Controller, Least Privilege, Deterministic Dispatch,
  Recoverable Operation, Tests Define Done
- Added sections: Security and Runtime Constraints, Development Workflow
- Templates updated: plan-template.md, tasks-template.md
- Deferred items: none
-->
# GitHub Runner Dispatcher Constitution

## Core Principles

### I. Minimal Controller
The project MUST solve runner activation and deactivation only. It MUST reuse the
hosting platform's workflow filters, registered runner software, operating-system
service manager, and standard authentication mechanisms. Features unrelated to
dispatching MUST be rejected until a measured need exists.

### II. Least Privilege
Credentials MUST be restricted to explicitly configured repositories and the
minimum read permissions needed to observe workflow work. Remote control MUST run
as an unprivileged account and MUST only permit control of allowlisted runner
services. Secrets MUST never appear in configuration committed to source control,
logs, process arguments, or error messages.

### III. Deterministic Dispatch
For a given host, at most one repository runner MAY be active by default. Work MUST
be selected deterministically from the configured queue. Host priority and fallback
timeouts MUST be explicit. A job already accepted by a fallback host MUST never be
preempted when a preferred host returns.

### IV. Recoverable Operation
The controller MUST reconcile actual runner-service state after startup, restart,
network interruption, and partial failure. Repeated polling and control actions MUST
be idempotent. Failure to reach a preferred host MUST not strand work when an
allowlisted fallback is available.

### V. Tests Define Done
Every state transition, security boundary, and failure path MUST have an automated
test. Tests MUST cover queued work, active work, idle shutdown, host fallback,
restart reconciliation, duplicate observations, and command allowlisting. A change
is not complete while its tests or static checks fail.

## Security and Runtime Constraints

- Private repositories are the default and public repositories MUST require an
  explicit opt-in with a visible warning.
- Repository names, host names, service names, timing values, and concurrency limits
  MUST be configuration rather than project-specific constants.
- The controller MUST not execute workflow code itself; it only controls registered
  runner services.
- Runtime dependencies MUST remain minimal and justified. Native platform features
  take precedence over new infrastructure.
- Logs MUST be sufficient to reconstruct dispatch decisions without exposing secrets.

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

**Version**: 1.0.0 | **Ratified**: 2026-08-29 | **Last Amended**: 2026-08-29
