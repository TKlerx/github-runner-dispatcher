# Operations

## Install a participant

1. Download the `linux-amd64` or `windows-amd64` release binary.
2. Download and extract the matching official GitHub Actions runner into a clean
   template directory. Do not run `config.sh` or `config.cmd`.
3. Copy `config.example.yml` to a machine-local `config.yml` and update the
   participant name, actual OS labels, paths, capacity, and claim delay.
4. Install GitHub CLI only on a machine used for interactive setup, authenticate it,
   and run `runner-participant -config config.yml -setup`.

Setup warns whenever the configuration already exists and defaults to cancellation.
Choose `keep` to inspect the existing allowlist without changing a byte, or
explicitly choose `replace`, select repository numbers such as `1,3-5`, and confirm.
Archived repositories, repositories without an active workflow, and workflow-status
lookup failures are marked before selection. A repository without active CI is safe
to select; it remains idle until an active workflow queues a matching job.

Setup prints a fine-grained-PAT form and an exact repository checklist. In the
browser, choose "Only select repositories," select every listed repository, and
grant Metadata read, Actions read, and Administration write. Store the resulting PAT
as the only line in `token_file`. Setup never obtains the PAT from GitHub CLI.

Run the non-mutating validation before starting the service:

```text
runner-participant -config config.yml -check
```

## Linux systemd example

```ini
[Unit]
Description=On-demand GitHub Actions runner participant
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=github-runner
ExecStart=/usr/local/bin/runner-participant -config /etc/github-runner-dispatcher/config.yml
Restart=on-failure
RestartSec=10
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
```

Create the configured state and template directories with ownership restricted to
that service user, then use `systemctl enable --now runner-participant`.

## Windows startup example

Run this once from an elevated PowerShell terminal to create a startup task under a
dedicated local account. Grant that account access only to the configured binary,
token, template, and state paths.

```powershell
$action = New-ScheduledTaskAction -Execute 'C:\Program Files\github-runner-dispatcher\runner-participant.exe' -Argument '-config C:\ProgramData\github-runner-dispatcher\config.yml'
$trigger = New-ScheduledTaskTrigger -AtStartup
Register-ScheduledTask -TaskName 'GitHub Runner Participant' -Action $action -Trigger $trigger -User '.\github-runner' -RunLevel Limited
```

## Preference and updates

Participants do not communicate. A stronger machine normally wins by using a shorter
`claim_delay`; a slower fallback uses a longer delay. GitHub may still receive
redundant compatible JIT runners during a race, but it assigns a job only once and
idle contenders time out.

To update the official runner, stop the participant, replace the clean template,
run `-check`, and restart. Existing disposable copies finish or reconcile from their
own directories.

## Ten-minute onboarding check (SC-007)

Start with the binary, clean official runner template, and PAT installed. Start a
timer, add or select one private repository, run `-check`, and stop the timer when it
passes. The target is under ten minutes. `-check` performs GET requests only and
starts no child process.
