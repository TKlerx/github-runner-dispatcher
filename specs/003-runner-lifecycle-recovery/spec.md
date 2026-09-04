# Runner lifecycle recovery

GitHub may assign a JIT runner a different job from the one that prompted its
creation. Observe the actual runner assignment before applying acquisition timeout.
On Linux, terminate the verified runner process group so child installers cannot
continue writing during cleanup. Failed cleanup must retain its manifest for retry.
Unknown or unverifiable state must continue to reserve capacity and fail closed.

Acceptance: another job assigned to this runner cancels acquisition timeout; another
runner does not; Linux cancellation terminates child processes; failed deletion
preserves a readable manifest and reconciliation succeeds once deletion is possible.
