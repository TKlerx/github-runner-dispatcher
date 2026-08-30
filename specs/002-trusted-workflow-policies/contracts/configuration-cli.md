# Configuration and CLI Contract

## Repository policy YAML

```yaml
repositories:
  - owner: example
    name: agent-repository
    visibility: public
    runner_group_id: 1
    trusted_workflows:
      - workflow_id: 123456
        rules:
          - events: [pull_request]
            actors: ["*"]
            required_labels: [nas-qwen]
          - events: [repository_dispatch]
            actors: [trusted-operator]
            required_labels: [nas-qwen]
      - workflow_path: .github/workflows/issue-agent.yml
        rules:
          - events: [issues, issue_comment]
            actors: [trusted-operator]
            required_labels: [nas-qwen]
```

`visibility` defaults to `private` when omitted. Existing private configurations remain
valid. Public entries without `trusted_workflows` are invalid. Policy values are data;
the dispatcher ships no repository-specific defaults.

## Non-interactive mutation

```text
runner-participant -config <config.yml> \
  -policy-action <add|reconcile|remove> \
  -policy-file <repository-policy.yml>
```

The policy file is a strict YAML document containing one repository entry under
`repository:` using the same schema as an item in `repositories`.

- `add`: atomically adds the entry; fails if its repository already exists.
- `reconcile`: atomically inserts or exactly replaces the entry; repeating it is idempotent.
- `remove`: atomically removes the identified repository; only owner/name are used.
- Invalid input or an invalid resulting configuration leaves the config byte-for-byte unchanged.
- These flags are mutually exclusive with normal, `-check`, and interactive `-setup` modes.

Interactive `-setup` keeps its current private-repository discovery and numbered
multi-select. When replacing the allowlist, it preserves complete entries for retained
repositories rather than discarding existing policies.
