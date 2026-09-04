# Plan

Reuse GitHub workflow-run/job listing to discover this runner's actual assignment.
Keep authorization checks before runner creation unchanged. Use Linux process groups
and verified process identity for termination, including context cancellation.
Remove workspace contents before the manifest, and reuse cleanup during reconciliation
so a failed attempt restores recovery metadata. No dependencies or policy exceptions.

Validate focused regressions, then Go formatting, vet, unit/race tests and builds on
Windows and Linux. Document recovery for already damaged state; do not automatically
delete unknown state or weaken capacity/identity checks.
