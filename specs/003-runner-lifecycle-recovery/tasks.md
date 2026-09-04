# Tasks

- [x] T001 Fix actual assignment detection, Linux process-group termination and
  retryable cleanup with focused regression tests; validate and commit.

Validation: Windows `go test ./...` and `go vet ./...` passed; Linux unprivileged
`go test -race -count=1 ./...` passed, including real child-process cancellation and
permission-denied cleanup recovery. Linux amd64 binary built. Windows race execution
requires the CI C compiler (not installed on this workstation).
