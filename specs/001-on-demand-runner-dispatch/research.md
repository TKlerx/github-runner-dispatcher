# Research: Decentralized On-Demand Runner Participation

## Go executable

**Decision**: Use Go 1.27 and produce native Windows and Linux executables.

**Rationale**: Go 1.27 is the current stable release, provides cross-compilation,
HTTP, JSON, structured logging, process control, and testing in the standard library,
and needs no runtime installation on participant machines.

**Alternatives considered**: Node.js and Python require a runtime on every machine;
.NET is cross-platform but adds a larger runtime; Rust adds build complexity without
a demonstrated benefit for this small service.

## Configuration format

**Decision**: Use strict YAML parsed by `go.yaml.in/yaml/v4`; load the PAT only from
a separate file.

**Rationale**: YAML keeps the multi-repository configuration readable. The maintained
v4 parser supports strict typed decoding, while all other runtime behavior remains in
the standard library. A token file avoids secrets in shareable configuration.

**Alternatives considered**: JSON would remove the only dependency but is less
pleasant for operator-edited configuration. TOML needs a dependency without a clear
benefit. Environment-only configuration becomes awkward for repository lists.

## GitHub job observation

**Decision**: For each repository, list queued and in-progress workflow runs and then
list their latest jobs. Cache each queued job's first local observation time and use
that as the claim-delay origin.

**Rationale**: Workflow-run endpoints support status filtering and require Actions
read. Job responses contain status, labels, runner identity, run ID, and job ID, but
do not expose a queued timestamp. Local first-seen time preserves independent
participants and deterministic local behavior.

**Alternatives considered**: Webhooks need a reachable server and central routing.
Interpreting commits or pull requests duplicates GitHub workflow logic. Using workflow
run creation time is wrong for dependent jobs that become queued much later.

## JIT provisioning

**Decision**: Call
`POST /repos/{owner}/{repo}/actions/runners/generate-jitconfig` with a configurable
runner group ID (default `1`), unique runner name, participant labels, and `_work`.

**Rationale**: The repository JIT endpoint creates an ephemeral runner that handles
at most one job and requires Administration write. The official API example uses
runner group ID `1`; keeping it configurable avoids hard-coding an account-specific
assumption.

**Alternatives considered**: Registration tokens require persistent configuration
and cleanup. Organization runners do not cover personal-account repositories.

## Secret-safe runner startup

**Decision**: Set `ACTIONS_RUNNER_INPUT_JITCONFIG` only in the child environment and
launch the official runner without a JIT command-line argument.

**Rationale**: The official runner recognizes `jitconfig` as a secret input and reads
`ACTIONS_RUNNER_INPUT_JITCONFIG`. This avoids exposing the encoded configuration in
our logs or command arguments. The environment exists only for the child process and
is discarded after launch.

**Alternatives considered**: GitHub documents `--jitconfig`, but command arguments
are more visible to local process inspection. Writing JIT configuration to disk would
create another secret-cleanup obligation.

## Disposable runner directory

**Decision**: Copy one operator-maintained, unconfigured runner template directory
into a unique managed directory for each JIT runner.

**Rationale**: JIT startup writes runner credentials and diagnostics beside the
runner binary. A per-job copy gives Windows and Linux the same cleanup boundary and
supports configured capacity greater than one without shared mutable runner files.

**Alternatives considered**: Reusing one directory leaks state and prevents safe
concurrency. Downloading and extracting the runner for every job increases latency
and requires update/download logic. Containers exclude native Windows participation.

## Decentralized preference

**Decision**: Each participant applies its own claim delay, performs a final job
recheck, and accepts occasional redundant unassigned JIT runners.

**Rationale**: Shorter-delay machines normally connect first. GitHub assigns a job to
only one runner, so no distributed lock is required. The acquisition timeout bounds
the cost of a race.

**Alternatives considered**: A coordinator creates a single point of failure. A
distributed lock adds infrastructure and credentials while still not reserving a
specific GitHub job for a JIT runner.

## Local recovery state

**Decision**: Persist one non-secret JSON manifest inside each managed runner
directory and update it atomically.

**Rationale**: Manifests let a restarted participant count live children, correlate
runner names with GitHub jobs, terminate overdue unassigned runners, and retry
cleanup. No database is needed at the intended scale.

**Alternatives considered**: Memory-only state cannot reconcile after restart. A
database or local daemon protocol is unnecessary for a handful of child processes.

## Service installation

**Decision**: Ship a foreground process; document systemd and Windows service/task
wrappers instead of implementing an installer.

**Rationale**: Native operating-system tooling already handles startup and restart.
Keeping installation external reduces privilege requirements and platform code.

**Alternatives considered**: An embedded service installer is convenient but needs
administrator-specific logic and expands the v1 security surface.
