# Quickstart: Decentralized On-Demand Runner Participation

This quickstart describes the intended v1 workflow after implementation.

## 1. Select repositories and prepare GitHub access

Copy `config.example.yml` to `config.yml` and set the participant name, labels, local
paths, claim delay, and capacity. The token file does not need to exist yet.

Authenticate GitHub CLI on this machine or another trusted setup machine, then run:

```text
runner-participant -config config.yml -setup
```

Select the private repositories this participant may serve. Setup writes the YAML
allowlist and prints a prefilled fine-grained-PAT form URL. Open it, choose "Only
select repositories," manually select every repository in the printed checklist,
and generate the token. GitHub's URL format cannot preselect repositories. Save only
the resulting token in the configured machine-local token file.

If `config.yml` already exists, setup warns and defaults to cancellation. Choose to
keep its allowlist unchanged—useful after copying NAS configuration to `jan-cachy`—or
explicitly replace only the repository list. Before using a copied file, update its
participant name, OS/architecture labels, paths, and claim delay for the new machine.
Repositories with no active workflow are marked but remain selectable; selecting one
is harmless and only adds idle polling until CI is added or enabled.

## 2. Prepare the official runner template

Download the official GitHub Actions runner matching this machine, extract it once,
and do not run `config.sh` or `config.cmd`. Keep this directory clean and update it
when GitHub releases required runner updates.

## 3. Finish participant configuration

Confirm that `token_file`, `runner_template_dir`, and `state_dir` point at the prepared
machine-local paths.

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

- Setup discovers private repositories across multiple pages, marks archived entries,
  marks repositories with no active workflows, and writes only the selected repositories.
- Cancelling setup or keeping a copied allowlist leaves the config byte-for-byte unchanged.
- The generated PAT link contains the common repository owner and only Metadata read,
  Actions read, and Administration write permissions.
- With no queued jobs, no official runner process is present.
- With two participants, the shorter-delay machine normally accepts the job.
- With the preferred participant offline, the delayed participant accepts it.
- A Windows participant ignores jobs requiring `Linux`; Linux ignores `Windows`.
- Restarting the participant during a job does not exceed local capacity.
- Interrupting cleanup causes the next startup to retry cleanup before offering work.
- A reused PID or mismatched process identity is reported and never signaled.
- Cleanup unlinks a workflow-created symlink or Windows junction without traversing it.
- Logs never contain the PAT or encoded JIT configuration.
