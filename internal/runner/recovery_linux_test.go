//go:build linux

package runner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCleanupFailurePreservesManifestForRetry(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires unprivileged filesystem permissions")
	}
	manager, _, _ := testManager(t, time.Second, 1)
	directory := writeTestManifest(t, manager, "retry", Manifest{Phase: PhaseCleanupFailed})
	locked := filepath.Join(directory, "workspace")
	if err := os.Mkdir(locked, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locked, "file"), []byte("busy"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0700) })
	if err := manager.Reconcile(context.Background()); err == nil {
		t.Fatal("expected deletion failure")
	}
	if _, err := readManifest(directory); err != nil {
		t.Fatalf("cleanup destroyed recovery metadata: %v", err)
	}
	if manager.Active() != 1 {
		t.Fatal("failed cleanup must retain its capacity reservation")
	}
	if err := os.Chmod(locked, 0700); err != nil {
		t.Fatal(err)
	}
	if err := manager.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cleanup was not retried: %v", err)
	}
	if manager.Active() != 0 {
		t.Fatal("recovered cleanup did not release capacity")
	}
}

func TestLinuxManagerCancellationKillsRunnerChildren(t *testing.T) {
	root := t.TempDir()
	template := filepath.Join(root, "template")
	if err := os.Mkdir(template, 0700); err != nil {
		t.Fatal(err)
	}
	childFile := filepath.Join(root, "child.pid")
	// A child that ignores SIGTERM reproduces an installer outliving run.sh.
	script := "#!/bin/sh\nsh -c 'trap \"\" TERM; while :; do sleep 1; done' &\necho $! > '" + childFile + "'\nwait\n"
	if err := os.WriteFile(filepath.Join(template, "run.sh"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(filepath.Join(root, "state"), template, 1, time.Minute, NativeProcessController{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- manager.Run(ctx, launchRequest("test"), make(chan struct{})) }()
	var childPID int
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(childFile); err == nil {
			childPID, _ = strconv.Atoi(strings.TrimSpace(string(data)))
			if childPID > 0 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if childPID == 0 {
		t.Fatal("runner did not start its child")
	}
	t.Cleanup(func() { _ = syscall.Kill(childPID, syscall.SIGKILL) })
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runner cancellation hung")
	}
	// Zombies are terminated, but may await reaping by the host's init process.
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile("/proc/" + strconv.Itoa(childPID) + "/stat")
		if errors.Is(err, os.ErrNotExist) {
			return
		}
		if err == nil && strings.HasPrefix(string(data)[strings.LastIndexByte(string(data), ')')+1:], " Z ") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("runner child survived cancellation")
}
