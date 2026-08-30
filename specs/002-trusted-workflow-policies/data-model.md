# Data Model: Configurable Trusted-Workflow Policies

## Repository Configuration

- `owner`: non-empty GitHub owner; all configured repositories retain the existing common-owner rule.
- `name`: non-empty repository name.
- `visibility`: `private` or `public`; omitted means `private` for legacy files.
- `runner_group_id`: positive integer, default 1.
- `trusted_workflows`: optional unique list of Trusted Workflow values.

Validation: `public` requires at least one trusted workflow. Repository identities are
case-insensitively unique.

## Trusted Workflow

- Exactly one of `workflow_id` (positive immutable GitHub workflow ID) or
  `workflow_path` (exact `.github/workflows/*.yml|yaml` path).
- `rules`: one or more Authorization Rule values.

Validation: identities are unique per repository. Paths are repository-relative,
slash-normalized, and cannot contain query/fragment/ref suffixes.

## Authorization Rule

- `events`: non-empty, case-sensitive GitHub event names.
- `actors`: non-empty GitHub logins or the sole value `*`.
- `required_labels`: non-empty, case-insensitive runner labels.

Validation: values are trimmed and unique; wildcard cannot be mixed with named actors;
required labels must be advertised by the participant; duplicate normalized rules are
rejected.

## Server-Observed Metadata

### Job

- Job ID, run ID, status, labels, and assigned runner identity.

### Workflow Run

- Run ID, status, workflow ID/path, event, actor login, triggering actor login, and
  repository full name/visibility.

## Authorization state transition

```text
observed -> label-compatible -> initial policy authorized -> claim eligible
         -> final job+run re-fetch -> consistent and policy authorized
         -> JIT generated -> one job -> cleanup

any missing/mismatch/error -------------------------------> denied, no JIT
```
