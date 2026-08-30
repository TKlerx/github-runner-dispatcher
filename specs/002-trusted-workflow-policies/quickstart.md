# Quickstart: Trusted-Workflow Policies

1. Keep an existing private repository unchanged to verify backward compatibility.
2. Create a generic policy file using `contracts/configuration-cli.md`; prefer workflow IDs.
3. Reconcile it non-interactively:

   ```text
   runner-participant -config /path/config.yml -policy-action reconcile -policy-file /path/repository-policy.yml
   ```

4. Run `runner-participant -config /path/config.yml -check`.
5. Queue one authorized and one unauthorized job. Confirm only the authorized job reaches
   JIT creation and that its local runner directory and GitHub registration disappear.

Do not treat successful cleanup as sandboxing or as proof that the workflow is read-only.
Use a dedicated unprivileged host/account for untrusted workflow execution.
