package runner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReconcileCountsLiveWaitingAndAssignedRunners(t *testing.T) {
	manager, controller, _ := testManager(t, time.Second, 4)
	controller.statuses = map[int]ProcessStatus{101: ProcessMatches, 102: ProcessMatches}
	writeTestManifest(t, manager, "waiting", Manifest{ProcessID: 101, ProcessStartMarker: "a", ProcessExecutable: "runner-a", Phase: PhaseWaiting, AcquisitionDeadline: time.Now().Add(time.Minute)})
	writeTestManifest(t, manager, "assigned", Manifest{ProcessID: 102, ProcessStartMarker: "b", ProcessExecutable: "runner-b", Phase: PhaseAssigned})

	if err := manager.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if manager.Active() != 2 {
		t.Fatalf("active = %d", manager.Active())
	}
}

func TestReconcileCleansDeadAndTerminatesOnlyVerifiedOverdueRunner(t *testing.T) {
	manager, controller, _ := testManager(t, time.Second, 4)
	controller.statuses = map[int]ProcessStatus{101: ProcessMissing, 102: ProcessMatches, 103: ProcessMismatched}
	dead := writeTestManifest(t, manager, "dead", Manifest{ProcessID: 101, Phase: PhaseWaiting})
	overdue := writeTestManifest(t, manager, "overdue", Manifest{ProcessID: 102, ProcessStartMarker: "b", ProcessExecutable: "runner-b", Phase: PhaseWaiting, AcquisitionDeadline: time.Now().Add(-time.Minute)})
	mismatched := writeTestManifest(t, manager, "reused", Manifest{ProcessID: 103, ProcessStartMarker: "c", ProcessExecutable: "runner-c", Phase: PhaseWaiting, AcquisitionDeadline: time.Now().Add(-time.Minute)})

	if err := manager.Reconcile(context.Background()); err == nil {
		t.Fatal("Reconcile() accepted a mismatched process identity")
	}
	for _, removed := range []string{dead, overdue} {
		if _, err := os.Stat(removed); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("directory remains: %s", removed)
		}
	}
	if _, err := os.Stat(mismatched); err != nil {
		t.Fatalf("mismatched process directory was removed: %v", err)
	}
	select {
	case identity := <-controller.terminated:
		if identity.PID != 102 {
			t.Fatalf("terminated PID = %d", identity.PID)
		}
	default:
		t.Fatal("verified overdue runner was not terminated")
	}
	if manager.Active() != 1 {
		t.Fatalf("unverifiable runner must reserve capacity, active = %d", manager.Active())
	}
}

func TestReconcileReportsMalformedUnknownAndInspectionFailureWithoutDeleting(t *testing.T) {
	manager, controller, _ := testManager(t, time.Second, 4)
	controller.inspectErr = map[int]error{104: errors.New("access denied")}
	unknown := filepath.Join(manager.StateDir, "runners", "unknown")
	malformed := filepath.Join(manager.StateDir, "runners", "malformed")
	if err := os.MkdirAll(unknown, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(malformed, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(malformed, "manifest.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	unverifiable := writeTestManifest(t, manager, "unverifiable", Manifest{ProcessID: 104, Phase: PhaseWaiting})

	if err := manager.Reconcile(context.Background()); err == nil {
		t.Fatal("Reconcile() did not report unsafe directories")
	}
	for _, path := range []string{unknown, malformed, unverifiable} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("unsafe directory was removed: %s: %v", path, err)
		}
	}
	if manager.Active() != 3 {
		t.Fatalf("unsafe directories must reserve capacity, active = %d", manager.Active())
	}
}

func TestReconcileRetriesCleanupFailedManifest(t *testing.T) {
	manager, _, _ := testManager(t, time.Second, 1)
	directory := writeTestManifest(t, manager, "cleanup", Manifest{Phase: PhaseCleanupFailed})
	if err := manager.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cleanup-failed directory remains: %v", err)
	}
}

func writeTestManifest(t *testing.T, manager *Manager, name string, manifest Manifest) string {
	t.Helper()
	directory := filepath.Join(manager.StateDir, "runners", name)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest.SchemaVersion = ManifestSchemaVersion
	manifest.InstanceID = name
	if err := writeManifest(directory, &manifest); err != nil {
		t.Fatal(err)
	}
	return directory
}
