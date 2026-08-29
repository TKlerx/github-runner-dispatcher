# Data Model: Decentralized On-Demand Runner Participation

No database is used. Configuration is loaded from YAML; only runner manifests are
persisted as local JSON.

## ParticipantConfig

| Field | Type | Rules |
|---|---|---|
| participant_name | string | Required; 1-64 safe filename characters |
| repositories | list of RepositoryConfig | Required; unique; at least one |
| labels | list of string | Required; includes verified OS and architecture |
| poll_interval | duration | Required; 5s-5m |
| claim_delay | duration | Required; 0s-30m |
| acquisition_timeout | duration | Required; 30s-30m |
| capacity | integer | Required; 1-16; default 1 |
| token_file | path | Required; readable regular file |
| runner_template_dir | path | Required; clean official runner files |
| state_dir | path | Required; must not be filesystem root or template directory |
| github_api_url | URL | Optional; defaults to `https://api.github.com` |
| github_api_version | string | Optional; defaults to `2026-03-10` |

## RepositoryConfig

| Field | Type | Rules |
|---|---|---|
| owner | string | Required GitHub login |
| name | string | Required repository name |
| runner_group_id | integer | Positive; default 1 |

Identity is the case-insensitive `owner/name` pair. Startup validation confirms that
the repository exists, is private, and is accessible to the PAT.

## ObservedJob

| Field | Type | Rules |
|---|---|---|
| repository | repository identity | Must be allowlisted |
| run_id | integer | From GitHub |
| job_id | integer | Globally stable GitHub job identity |
| name | string | Diagnostic only |
| status | queued/in_progress/completed | From GitHub |
| labels | set of string | Compared case-insensitively |
| runner_name | optional string | Used to confirm assignment |
| first_seen_at | timestamp | Local monotonic/wall-clock observation |

Identity is `repository + job_id`. A queued job is eligible when its labels are a
subset of participant labels and `now >= first_seen_at + claim_delay`.

## RunnerManifest

Stored as `state_dir/runners/<instance_id>/manifest.json` with no credentials or JIT
configuration.

| Field | Type | Rules |
|---|---|---|
| schema_version | integer | Starts at 1 |
| instance_id | random string | Unique directory identity |
| repository | repository identity | Allowlisted at creation time |
| observed_job_id | integer | Job that caused capacity offering |
| runner_id | integer | Returned by JIT API |
| runner_name | string | Participant name plus random suffix |
| process_id | integer | Set after process start |
| phase | RunnerPhase | See lifecycle below |
| created_at | timestamp | UTC |
| acquisition_deadline | timestamp | UTC |
| cleanup_attempts | integer | Monotonic count |
| last_error | optional redacted string | Never contains secrets |

### RunnerPhase transitions

```text
preparing -> waiting -> assigned -> exited -> cleaning -> removed
                 |          |          |
                 +-> timed_out          +-> cleanup_failed -> cleaning
                 +-> failed -------------------------------^
```

- `preparing`: temporary copy and JIT request in progress.
- `waiting`: runner process is live but assignment is not confirmed.
- `assigned`: GitHub reports this runner name on an in-progress job.
- `timed_out`: acquisition deadline passed without assignment.
- `failed`: launch or process failure.
- `exited`: runner ended after assignment or failure.
- `cleaning`: process is stopped and recursive deletion is safe.
- `cleanup_failed`: deletion failed and blocks new capacity until retried.
- `removed`: terminal conceptual state; manifest directory no longer exists.

## ParticipationDecision

An in-memory structured-log event containing repository, job ID, participant,
decision (`ignore`, `wait`, `offer`, `adopt`, `terminate`, `cleanup`, `error`), reason,
timestamp, and redacted outcome. It is not stored separately from logs.
