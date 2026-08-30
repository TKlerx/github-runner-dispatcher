# Feature Specification: Configurable Trusted-Workflow Policies

**Feature Branch**: `002-trusted-workflow-policies`

**Created**: 2026-08-30

**Status**: Draft

**Input**: User description: "Add generic per-repository trusted-workflow authorization policies for secure Qwen runner dispatch while preserving existing private-repository behavior."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Authorize trusted workflow jobs (Priority: P1)

An operator can attach an explicit authorization policy to a configured repository.
The participant offers a one-job runner only when GitHub's own job and workflow-run
metadata match an allowed workflow identity, event, actor, and required label set.

**Why this priority**: A dedicated capability label is not a security boundary; the
dispatcher must independently authorize the workflow invocation before exposing a
trusted machine to it.

**Independent Test**: Configure policies for coding, fix-review, pull-request review,
and repository-dispatch review workflows, then present matching and mismatching
GitHub metadata and verify that only exact matches can receive a JIT runner.

**Acceptance Scenarios**:

1. **Given** a queued job bearing a dedicated label but naming an unknown workflow, **When** the participant evaluates it, **Then** the job is denied.
2. **Given** a coding or fix-review job triggered by an actor other than its configured operator, **When** the participant evaluates it, **Then** the job is denied.
3. **Given** an authorized pull-request review workflow whose policy explicitly allows any actor, **When** any actor opens or updates a pull request, **Then** the matching job is eligible.
4. **Given** an authorized review workflow invoked through repository dispatch, **When** the actor is not the configured operator, **Then** the job is denied; the configured operator is eligible.
5. **Given** metadata in issue text, workflow inputs, or another client-controlled payload claims a trusted actor, event, workflow, repository, or label, **When** GitHub's server-observed metadata does not match the policy, **Then** the claim has no effect and the job is denied.

---

### User Story 2 - Revalidate immediately before offering capacity (Priority: P1)

Immediately before generating JIT configuration, the participant re-fetches both the
workflow job and its workflow run and repeats the full authorization decision. Missing,
changed, contradictory, or otherwise inconsistent metadata prevents dispatch.

**Why this priority**: Authorization based on a stale observation leaves a race between
policy evaluation and runner creation.

**Independent Test**: Supply an initially authorized observation followed by changed,
missing, or inconsistent final job/run metadata and verify that no JIT configuration is
generated.

**Acceptance Scenarios**:

1. **Given** an initially authorized queued job, **When** its workflow, event, actor, repository, labels, run identity, or state changes before the final check, **Then** the participant denies it without generating JIT configuration.
2. **Given** an initially authorized queued job, **When** either final metadata request fails or omits a policy-relevant field, **Then** the participant denies it without generating JIT configuration.
3. **Given** final job and workflow-run metadata that identify different runs or repositories, **When** the participant compares them, **Then** it denies the job.

---

### User Story 3 - Admit explicitly governed public repositories (Priority: P2)

An operator may configure a public repository only when it has explicit trusted-workflow
policies and every job dispatched for that repository must match one of those policies.
Private repositories without policies retain the existing label-based behavior.

**Why this priority**: This adds the Qwen use case without silently broadening the trust
model for existing private repositories.

**Independent Test**: Validate private repositories with and without policies and public
repositories with and without policies, then verify that only the explicit public-policy
case can dispatch and existing private configuration behaves unchanged.

**Acceptance Scenarios**:

1. **Given** an existing private repository configuration without policies, **When** a compatible queued job is observed, **Then** its behavior is unchanged.
2. **Given** a public repository without an explicit trusted-workflow policy, **When** configuration is checked or work is observed, **Then** the repository is rejected and no runner is offered.
3. **Given** a public repository with explicit policies, **When** a job does not match any policy, **Then** it is denied even if its runner labels match.

---

### User Story 4 - Manage repository policies non-interactively (Priority: P3)

Repository onboarding can add, reconcile, and remove a repository policy through a
small non-interactive interface. Interactive setup continues to use the existing
repository discovery and numbered multi-select rather than a second selector.

**Why this priority**: The owning repository must remain responsible for its workflows
and onboarding while the dispatcher supplies a stable, generic configuration boundary.

**Independent Test**: Invoke add/reconcile/remove operations against temporary
configuration files and verify idempotent results, validation failures without partial
writes, and preservation of unrelated configuration.

**Acceptance Scenarios**:

1. **Given** a valid policy document and repository identity, **When** onboarding adds or reconciles it, **Then** the repository entry exactly reflects the requested policy and unrelated settings remain unchanged.
2. **Given** an existing repository policy, **When** onboarding removes it, **Then** only that policy is removed; a now-unprotected public repository is rejected rather than retained insecurely.
3. **Given** invalid or incomplete policy input, **When** onboarding attempts a mutation, **Then** configuration remains unchanged and the command fails with actionable validation errors.

---

### User Story 5 - Preserve one-job lifecycle cleanup (Priority: P1)

Authorized work continues to run on one-job JIT runners whose temporary runner and work
directories are removed after every terminal outcome.

**Why this priority**: Policy authorization must not regress the existing ephemeral
lifecycle or leave credentials and workspace data behind.

**Independent Test**: Exercise completed, failed, cancelled, and timed-out lifecycles
and verify that each removes its temporary local directories and GitHub runner registration.

**Acceptance Scenarios**:

1. **Given** an authorized JIT runner, **When** its job completes, fails, or is cancelled, **Then** its temporary runner/work directories and registration are cleaned up.
2. **Given** an authorized JIT runner that never acquires work before timeout, **When** the timeout elapses, **Then** it is terminated and cleaned up.
3. **Given** successful cleanup, **When** the participant reports the outcome, **Then** it does not claim that cleanup sandboxed the workflow or proved it read-only.

### Edge Cases

- A workflow is renamed while retaining its workflow ID, or replaced at the same path with a new workflow ID.
- Workflow-run metadata reports a reusable workflow path or attempt suffix in addition to the caller workflow.
- An actor is renamed, deleted, or represented with different letter casing.
- A rerun has a different triggering actor from the actor that initiated the original run.
- A job's labels change between observation and final recheck.
- GitHub returns a job that references a run not returned for the same repository.
- A configured policy contains an empty actor list, empty event list, missing workflow identity, wildcard mixed with named actors, or duplicate values.
- Reconciliation receives the same desired policy repeatedly.
- Removing the last policy from a public repository would leave it configured but unauthorized.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Existing private repositories without a trusted-workflow policy MUST retain their current queued-job and label-matching behavior.
- **FR-002**: Operators MUST be able to configure optional per-repository trusted workflows using an exact workflow path or immutable workflow identity, with one or more authorization rules containing allowed events, allowed actors, and required runner labels.
- **FR-003**: Each trusted workflow MUST identify exactly one workflow by path or immutable identity and MUST contain at least one rule; every rule MUST contain at least one allowed event, at least one allowed actor, and at least one required runner label.
- **FR-004**: Actor policy MUST support explicitly named actors and an explicit wildcard meaning any GitHub-reported actor; an omitted or empty actor list MUST deny all jobs.
- **FR-005**: Policy authorization MUST use only metadata returned by GitHub for the workflow job and workflow run being evaluated.
- **FR-006**: Actor, event, workflow, repository, or label values obtained from issue text, comments, workflow inputs, dispatch payload fields, pull-request content, or other user-controlled text MUST NOT participate in authorization.
- **FR-007**: Matching runner labels without a matching trusted-workflow policy MUST NOT authorize a policy-governed job.
- **FR-008**: Immediately before generating JIT configuration, the participant MUST re-fetch both the workflow job and workflow run and repeat repository, run identity, queued state, workflow, event, actor, and label authorization.
- **FR-009**: Missing policy-relevant metadata, retrieval failure, state changes, or inconsistency between the final job and run observations MUST deny dispatch without generating JIT configuration.
- **FR-010**: Public repositories MUST be rejected unless they have at least one explicit trusted-workflow policy, and every dispatched job for a configured public repository MUST match a policy.
- **FR-011**: Private repositories with policies MUST require every dispatched job to match a policy; private repositories without policies MUST remain backward compatible.
- **FR-012**: Each authorized runner MUST remain a one-job JIT runner and MUST clean its temporary runner/work directories and registration after completion, failure, cancellation, and timeout, including retry through existing reconciliation when immediate local cleanup fails.
- **FR-013**: Documentation and logs MUST describe JIT cleanup as lifecycle hygiene only and MUST NOT claim it sandboxes workflows or proves that workflow behavior is read-only.
- **FR-014**: The dispatcher MUST remain repository-agnostic and MUST NOT embed Qwen workflow names, prompts, runner labels, actor names, or onboarding decisions.
- **FR-015**: A non-interactive interface MUST add, reconcile, and remove one repository policy atomically while preserving unrelated configuration and returning a nonzero result for invalid policy input.
- **FR-016**: Interactive repository selection MUST reuse the existing repository discovery and numbered multi-select behavior rather than introduce a separate selector.
- **FR-017**: Configuration validation MUST reject ambiguous or ineffective policies, including empty match fields, wildcard actors combined with named actors in one rule, malformed repository identities, duplicate trusted workflow identities within one repository, and duplicate authorization rules within one trusted workflow.
- **FR-018**: Authorization decisions MUST be logged without secrets and with enough server-observed identity information to explain why a job was allowed or denied.

### Key Entities

- **Repository Configuration**: A selected GitHub repository, its visibility, runner group, and optional trusted-workflow policies.
- **Trusted Workflow**: One exact workflow identity plus one or more authorization rules, allowing the same workflow to impose different actor restrictions for different events.
- **Authorization Rule**: One combination of allowed events, allowed actors, and required runner labels for a trusted workflow.
- **Server-Observed Job Metadata**: Repository, run and job identities, status, labels, and runner assignment returned by GitHub.
- **Server-Observed Workflow Run Metadata**: Repository, workflow identity/path, event, actor, status, and run identity returned by GitHub.
- **Policy Mutation**: An add/reconcile/remove request for one repository policy applied atomically to configuration.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: All specified unauthorized workflow, actor, event, label, public-repository, changed-metadata, missing-metadata, and inconsistent-metadata test cases result in zero JIT configurations generated.
- **SC-002**: All specified authorized review cases generate capacity only after two successful policy evaluations, including the immediate final recheck.
- **SC-003**: Existing private-repository configurations without policies pass their prior automated behavior suite unchanged.
- **SC-004**: Completed, failed, cancelled, and timed-out runner lifecycle tests leave zero temporary runner/work directories and zero temporary runner registrations.
- **SC-005**: Repeating the same reconciliation request produces byte-equivalent effective policy configuration and preserves every unrelated setting.
- **SC-006**: No dispatcher source or default configuration contains Qwen-specific workflow names, prompts, labels, or actor identities.

## Assumptions

- GitHub's workflow job and workflow run APIs expose stable run linkage plus server-observed workflow, event, actor, repository, and label metadata needed for authorization.
- Actor matching is case-insensitive because GitHub login casing is not a security distinction; wildcard is represented explicitly and exclusively.
- The workflow-run actor is the authorization actor; triggering-actor differences on reruns are treated as inconsistent or denied unless the implementation can establish an unambiguous server-defined rule without weakening policy.
- Workflow paths are repository-relative exact paths; immutable workflow IDs are preferred when onboarding can obtain them reliably.
- The initial Qwen policy values are supplied by `nas-coding-agent`; examples and product defaults in this repository remain generic.
- Workflow content safety and read-only guarantees are outside dispatcher scope.
