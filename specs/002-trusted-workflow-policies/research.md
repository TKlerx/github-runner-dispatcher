# Research: Configurable Trusted-Workflow Policies

## GitHub metadata source

**Decision**: Link a job to its run using the job's server-returned `run_id`. Authorize
against the run's `workflow_id`, `path`, `event`, `actor.login`,
`triggering_actor.login`, and `repository` identity/visibility, plus the job's status
and labels.

**Rationale**: GitHub's job response exposes run linkage and runner labels, while the
workflow-run response exposes the workflow, event, actors, and repository. No issue,
comment, dispatch payload, workflow input, or PR body is needed.

**Alternatives considered**: Workflow/job names are mutable display text and are not
authorization identities. Reading event payloads would cross the stated trust boundary.

Sources:

- https://docs.github.com/en/rest/actions/workflow-jobs?apiVersion=2026-03-10
- https://docs.github.com/en/rest/actions/workflow-runs?apiVersion=2026-03-10
- https://docs.github.com/en/rest/actions/workflows?apiVersion=2026-03-10

## Actor semantics

**Decision**: Named-actor rules require both non-empty `actor.login` and
`triggering_actor.login` to be allowed. Wildcard rules accept any non-empty values for
both fields. A mismatch on a rerun therefore denies named-actor work.

**Rationale**: `actor` represents the original run initiator while `triggering_actor`
can identify who initiated a rerun. Requiring both fails closed without selecting an
attacker-favorable interpretation.

**Alternatives considered**: Trusting only `actor` permits an unauthorized rerun;
trusting only `triggering_actor` can let an authorized user rerun work originally
created by a disallowed actor.

## Policy shape

**Decision**: A repository contains unique trusted workflow identities. Each workflow
contains multiple rules, and each rule combines events, actors, and required labels.

**Rationale**: One workflow may allow any actor for `pull_request` but only a named actor
for `repository_dispatch`. Flattened union lists cannot express that safely.

**Alternatives considered**: Duplicate workflow entries are ambiguous; separate
event-to-actor maps add a second policy language without reducing validation.

## Public repository gating

**Decision**: Repository visibility is server-validated. Configuration defaults omitted
visibility to `private` for backward compatibility. A configured `public` repository
must contain trusted workflows, and runtime also requires GitHub to report it as public.

**Rationale**: Explicit visibility makes offline config validation possible while the
check/runtime API verification detects stale or false declarations.

## Policy mutation interface

**Decision**: Add `-policy-action add|reconcile|remove -policy-file <path>`. The strict
policy document describes one complete repository entry. `add` refuses an existing
repository, `reconcile` atomically replaces or inserts that repository entry, and
`remove` atomically removes it. Other configuration stays untouched.

**Rationale**: Whole-entry reconciliation is idempotent and gives onboarding one small,
non-interactive contract. It avoids field-by-field patch semantics.

**Alternatives considered**: JSON/YAML fragments on command arguments risk shell quoting
and secret-adjacent logs. A second interactive selector duplicates existing setup.

## Security and API limitations

- GitHub metadata authenticates which workflow/run GitHub reports; it does not prove the
  workflow content is safe, read-only, or based on a protected branch.
- A workflow path is mutable. Workflow ID is the stronger identity when available, but
  the dispatcher supports exact path because the requirement explicitly permits it.
- JIT and temporary-directory cleanup reduce state reuse; they are not process, network,
  filesystem, container, or secret isolation.
- GitHub remains the final job allocator. Multiple independent participants may create
  redundant JIT registrations, but cannot duplicate one workflow job.
