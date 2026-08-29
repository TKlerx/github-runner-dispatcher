# GitHub Runner Dispatcher

A small controller for sharing limited self-hosted capacity across private GitHub
repositories owned by a personal account.

The dispatcher observes queued GitHub Actions work, starts only the matching
pre-registered repository runner service, prefers configured hosts, falls back when
needed, and stops idle services. The project is currently in specification phase.

## Project status

- [Feature specification](specs/001-on-demand-runner-dispatch/spec.md)
- [Project constitution](.specify/memory/constitution.md)
- Implementation has not started.

## Scope

The project controls existing runner services. It does not replace the official
GitHub Actions runner, interpret repository events independently, or provide a
general-purpose job scheduler.

## Development

This repository uses [GitHub Spec Kit](https://github.com/github/spec-kit):

1. Specify requirements.
2. Create the implementation plan.
3. Generate ordered tasks.
4. Implement one validated task at a time.

## Security

Private repositories are the default. See [SECURITY.md](SECURITY.md) before testing
with real credentials or runner hosts.

## License

[MIT](LICENSE)
