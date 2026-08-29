package integration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TKlerx/github-runner-dispatcher/internal/runner"
)

func TestRestartReconcilesWaitingAssignedAndCleanupRecovery(t *testing.T) {
	root := t.TempDir()
	state, template := filepath.Join(root, "state"), filepath.Join(root, "template")
	if err := os.MkdirAll(template, 0o700); err != nil {
		t.Fatal(err)
	}
	controller := &restartController{}
	manager, err := runner.NewManager(state, template, 4, time.Minute, controller)
	if err != nil {
		t.Fatal(err)
	}
	writeRestartManifest(t, state, "waiting", runner.Manifest{ProcessID: 1, ProcessStartMarker: "one", ProcessExecutable: "runner", Phase: runner.PhaseWaiting, AcquisitionDeadline: time.Now().Add(time.Minute)})
	writeRestartManifest(t, state, "assigned", runner.Manifest{ProcessID: 2, ProcessStartMarker: "two", ProcessExecutable: "runner", Phase: runner.PhaseAssigned})
	cleanup := writeRestartManifest(t, state, "cleanup", runner.Manifest{Phase: runner.PhaseCleanupFailed})

	if err := manager.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if manager.Active() != 2 {
		t.Fatalf("recovered active runners = %d", manager.Active())
	}
	if _, err := os.Stat(cleanup); !os.IsNotExist(err) {
		t.Fatalf("cleanup recovery directory remains: %v", err)
	}
}

func TestRestartTerminatesVerifiedOverdueWaitingRunner(t *testing.T) {
	root := t.TempDir()
	state, template := filepath.Join(root, "state"), filepath.Join(root, "template")
	if err := os.MkdirAll(template, 0o700); err != nil {
		t.Fatal(err)
	}
	controller := &restartController{}
	manager, err := runner.NewManager(state, template, 1, time.Minute, controller)
	if err != nil {
		t.Fatal(err)
	}
	directory := writeRestartManifest(t, state, "overdue", runner.Manifest{ProcessID: 3, ProcessStartMarker: "three", ProcessExecutable: "runner", Phase: runner.PhaseWaiting, AcquisitionDeadline: time.Now().Add(-time.Minute)})
	if err := manager.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if controller.terminated.Load() != 1 {
		t.Fatalf("terminations = %d", controller.terminated.Load())
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatalf("overdue directory remains: %v", err)
	}
}

func writeRestartManifest(t *testing.T, state, name string, manifest runner.Manifest) string {
	t.Helper()
	directory := filepath.Join(state, "runners", name)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest.SchemaVersion, manifest.InstanceID = runner.ManifestSchemaVersion, name
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "manifest.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return directory
}

type restartController struct{ terminated atomic.Int32 }

func (*restartController) Start(context.Context, runner.StartSpec) (runner.Process, error) {
	panic("not used")
}
func (*restartController) Inspect(runner.ProcessIdentity) (runner.ProcessStatus, error) {
	return runner.ProcessMatches, nil
}
func (controller *restartController) Terminate(runner.ProcessIdentity) error {
	controller.terminated.Add(1)
	return nil
}
