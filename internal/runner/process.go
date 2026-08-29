package runner

import "context"

type ProcessIdentity struct {
	PID         int
	StartMarker string
	Executable  string
}

type ProcessStatus uint8

const (
	ProcessMissing ProcessStatus = iota
	ProcessMatches
	ProcessMismatched
)

type StartSpec struct {
	Executable  string
	WorkingDir  string
	Environment []string
}

type Process interface {
	Identity() ProcessIdentity
	Wait() error
}

type ProcessController interface {
	Start(context.Context, StartSpec) (Process, error)
	Inspect(ProcessIdentity) (ProcessStatus, error)
	Terminate(ProcessIdentity) error
}
