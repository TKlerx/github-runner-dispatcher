# GitHub REST Authorization Contract

## Initial observation

The existing queued/in-progress workflow-run listing and run-job listing must retain:

- run: `id`, `status`, `workflow_id`, `path`, `event`, `actor.login`,
  `triggering_actor.login`, `repository.full_name`, `repository.private`;
- job: `id`, `run_id`, `status`, `labels`, `runner_id`, `runner_name`.

Policy-governed work is claim-eligible only after these server fields form a complete,
consistent match.

## Immediate final authorization

Before `POST .../generate-jitconfig`, call in order:

1. `GET /repos/{owner}/{repo}/actions/jobs/{job_id}`
2. `GET /repos/{owner}/{repo}/actions/runs/{run_id}`

Require job ID, run ID, configured repository, queued job state, workflow identity,
event, both actor fields, visibility, and labels to be present and consistent with the
original observation and current policy. Re-run policy evaluation on these fresh values.
Any request error or mismatch returns a deny decision and does not call JIT generation.

## Repository validation

`GET /repos/{owner}/{repo}` returns canonical `full_name` and `private`. Check mode
requires it to equal configured identity/visibility and rejects public repositories
without policy before any mutating endpoint is considered.

## Trust boundary

Authorization never consumes event payload bodies, workflow inputs, issue/comment text,
pull-request content, client request fields, workflow names, or job names. Labels are a
required match but never sufficient authorization for a policy-governed repository.
