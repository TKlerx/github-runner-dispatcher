# GitHub REST Contract

All calls send:

```text
Accept: application/vnd.github+json
Authorization: Bearer <participant PAT>
X-GitHub-Api-Version: 2026-03-10
User-Agent: github-runner-dispatcher/<version>
```

## Read operations

1. `GET /repos/{owner}/{repo}` validates existence and `private: true`.
2. `GET /repos/{owner}/{repo}/actions/runs?status=queued&per_page=100` discovers queued runs.
3. `GET /repos/{owner}/{repo}/actions/runs?status=in_progress&per_page=100` discovers active runs.
4. `GET /repos/{owner}/{repo}/actions/runs/{run_id}/jobs?filter=latest&per_page=100` returns job status, labels, IDs, and runner identity.
5. `GET /repos/{owner}/{repo}/actions/jobs/{job_id}` performs the final state recheck.
6. `GET /repos/{owner}/{repo}/actions/runners?per_page=1` verifies repository runner-administration access in side-effect-free check mode.

All paginated responses follow `Link` headers. Observation handles `403` rate limits
using `Retry-After` or `X-RateLimit-Reset`; transient `5xx` and network failures use
bounded exponential backoff without discarding locally tracked runners.

## JIT creation

`POST /repos/{owner}/{repo}/actions/runners/generate-jitconfig`

```json
{
  "name": "jan-cachy-a1b2c3d4",
  "runner_group_id": 1,
  "labels": ["self-hosted", "Linux", "X64"],
  "work_folder": "_work"
}
```

The response supplies `runner.id`, `runner.name`, and `encoded_jit_config`. Only the
non-secret IDs and name enter the manifest. The encoded configuration goes directly
to the child environment as `ACTIONS_RUNNER_INPUT_JITCONFIG`.

## Permissions

- Repository metadata: Metadata read.
- Workflow runs and jobs: Actions read.
- Repository JIT configuration: Administration write.

No endpoint that triggers, reruns, cancels, approves, or modifies workflow execution
is called.
