# GitHub Runner Dispatcher

A small decentralized service for sharing trusted self-hosted capacity across
explicitly selected GitHub repositories.

Each participant observes matching queued GitHub Actions jobs, applies its local
claim delay, and launches a one-job JIT runner. Participants do not coordinate, and
repositories need no persistent runner registration or dedicated service.

The setup command uses an existing GitHub CLI login to list owned private
repositories, writes the operator's selection to YAML, and prints a prefilled PAT
form plus the repositories that must still be selected manually in GitHub.
Existing configuration is never silently overwritten, and repositories without an
active Actions workflow are marked during selection but remain optional.

## Project status

- [Feature specification](specs/001-on-demand-runner-dispatch/spec.md)
- [Project constitution](.specify/memory/constitution.md)
- The participant, interactive setup, check mode, recovery, and Windows/Linux builds are implemented.
- See [operations](docs/operations.md) for installation and startup examples.

## Scope

The project launches the official GitHub Actions runner with JIT configuration. It
does not trigger workflows, coordinate participants, interpret event payload text,
or provide a general-purpose job scheduler. Existing private repositories may use
label-only dispatch. Public repositories are accepted only when every dispatched job
matches an explicit trusted workflow, event, actor, and required-label rule.

## Trusted-workflow policies

Policies identify a workflow by exact path or immutable GitHub workflow ID and contain
one or more event/actor/label rules. The participant evaluates only metadata returned
by GitHub, then re-fetches the job and workflow run and repeats the decision immediately
before creating JIT configuration. Missing, changed, or inconsistent metadata denies
dispatch. Runner labels alone never authorize a policy-governed job.

External onboarding can atomically manage one complete repository entry:

```text
runner-participant -config config.yml -policy-action reconcile -policy-file repository-policy.yml
```

Actions are `add`, `reconcile`, and `remove`. See the
[configuration contract](specs/002-trusted-workflow-policies/contracts/configuration-cli.md).

## Development

This repository uses [GitHub Spec Kit](https://github.com/github/spec-kit):

1. Specify requirements.
2. Create the implementation plan.
3. Generate ordered tasks.
4. Implement one validated task at a time.

```text
go test ./...
go vet ./...
go build ./cmd/runner-participant
```

The race detector requires CGO. The public CI matrix runs it on both Windows and
Linux; a local Go installation with `CGO_ENABLED=0` should use the CI result rather
than silently treating a skipped local race command as validation.

## Security

See [SECURITY.md](SECURITY.md) before testing with real credentials or runner hosts.

## License

[MIT](LICENSE)
