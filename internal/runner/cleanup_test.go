package runner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRemoveInstanceRejectsTargetsOutsideRunnerRoot(t *testing.T) {
	manager, _, _ := testManager(t, time.Second, 1)
	outside := filepath.Join(filepath.Dir(manager.StateDir), "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := manager.removeInstance(outside); err == nil {
		t.Fatal("removeInstance accepted an outside directory")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside directory was removed: %v", err)
	}
}

func TestReconcileCleanupDoesNotFollowSymbolicLinks(t *testing.T) {
	manager, _, root := testManager(t, time.Second, 1)
	external := filepath.Join(root, "external.txt")
	if err := os.WriteFile(external, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	directory := writeTestManifest(t, manager, "linked", Manifest{Phase: PhaseCleanupFailed})
	if err := os.Symlink(external, filepath.Join(directory, "external-link")); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	if err := manager.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(external); err != nil || string(data) != "preserve" {
		t.Fatalf("external target changed: %q, %v", data, err)
	}
	if _, err := os.Stat(directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("managed directory remains: %v", err)
	}
}
