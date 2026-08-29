# Quickstart: Decentralized On-Demand Runner Participation

This quickstart describes the intended v1 workflow after implementation.

## 1. Prepare GitHub access

Create a separate fine-grained PAT for this machine. Select only the private
repositories it may serve and grant Metadata read, Actions read, and Administration
write. Save only the token in a machine-local file.

## 2. Prepare the official runner template

Download the official GitHub Actions runner matching this machine, extract it once,
and do not run `config.sh` or `config.cmd`. Keep this directory clean and update it
when GitHub releases required runner updates.

## 3. Configure the participant

Copy `config.example.yml`, list the private repositories, set labels to the machine's
actual OS and architecture, and point `token_file`, `runner_template_dir`, and
`state_dir` at machine-local paths.

Use shorter claim delays for preferred machines:

```text
strong workstation: 0s
jan-cachy:          15s
NAS:                60s
```

## 4. Validate without mutation

```text
runner-participant -config config.yml -check
```

The command must reject public or inaccessible repositories, mismatched OS/CPU
labels, unsafe state paths, missing runner files, and insufficient token permissions.
It must not create a runner in GitHub or start a process.

For the SC-007 usability check, start timing with the binary, clean runner template,
and PAT already present. Add one repository to the YAML, run `-check`, and record
whether validation succeeds within ten minutes.

## 5. Run in the foreground

```text
runner-participant -config config.yml
```

Queue a label-compatible job in one allowlisted private repository. Verify that the
participant waits for its claim delay, rechecks the job, creates one temporary runner,
and removes the runner directory after completion.

## 6. Run at startup

Configure the same foreground command with systemd on Linux or trusted Windows
service/task tooling. The project does not install privileged services itself.

## Verification scenarios

- With no queued jobs, no official runner process is present.
- With two participants, the shorter-delay machine normally accepts the job.
- With the preferred participant offline, the delayed participant accepts it.
- A Windows participant ignores jobs requiring `Linux`; Linux ignores `Windows`.
- Restarting the participant during a job does not exceed local capacity.
- Interrupting cleanup causes the next startup to retry cleanup before offering work.
- A reused PID or mismatched process identity is reported and never signaled.
- Cleanup unlinks a workflow-created symlink or Windows junction without traversing it.
- Logs never contain the PAT or encoded JIT configuration.
