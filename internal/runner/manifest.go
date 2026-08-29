package runner

import "time"

const ManifestSchemaVersion = 1

type RepositoryIdentity struct {
	Owner string `json:"owner"`
	Name  string `json:"name"`
}

type Phase string

const (
	PhasePreparing     Phase = "preparing"
	PhaseWaiting       Phase = "waiting"
	PhaseAssigned      Phase = "assigned"
	PhaseTimedOut      Phase = "timed_out"
	PhaseFailed        Phase = "failed"
	PhaseExited        Phase = "exited"
	PhaseCleaning      Phase = "cleaning"
	PhaseCleanupFailed Phase = "cleanup_failed"
)

type Manifest struct {
	SchemaVersion       int                `json:"schema_version"`
	InstanceID          string             `json:"instance_id"`
	Repository          RepositoryIdentity `json:"repository"`
	ObservedJobID       int64              `json:"observed_job_id"`
	RunnerID            int64              `json:"runner_id"`
	RunnerName          string             `json:"runner_name"`
	ProcessID           int                `json:"process_id"`
	ProcessStartMarker  string             `json:"process_start_marker"`
	ProcessExecutable   string             `json:"process_executable"`
	Phase               Phase              `json:"phase"`
	CreatedAt           time.Time          `json:"created_at"`
	AcquisitionDeadline time.Time          `json:"acquisition_deadline"`
	CleanupAttempts     int                `json:"cleanup_attempts"`
	LastError           string             `json:"last_error,omitempty"`
}
