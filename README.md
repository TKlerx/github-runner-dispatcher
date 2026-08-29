# GitHub Runner Dispatcher

A small decentralized service for sharing trusted self-hosted capacity across
private GitHub repositories owned by a personal account.

Each participant observes matching queued GitHub Actions jobs, applies its local
claim delay, and launches a one-job JIT runner. Participants do not coordinate, and
repositories need no persistent runner registration or dedicated service.

## Project status

- [Feature specification](specs/001-on-demand-runner-dispatch/spec.md)
- [Project constitution](.specify/memory/constitution.md)
- Implementation has not started.

## Scope

The project launches the official GitHub Actions runner with JIT configuration. It
does not trigger workflows, coordinate participants, support public repositories,
or provide a general-purpose job scheduler.

## Development

This repository uses [GitHub Spec Kit](https://github.com/github/spec-kit):

1. Specify requirements.
2. Create the implementation plan.
3. Generate ordered tasks.
4. Implement one validated task at a time.

## Security

Only private repositories are supported. See [SECURITY.md](SECURITY.md) before
testing with real credentials or runner hosts.

## License

[MIT](LICENSE)
