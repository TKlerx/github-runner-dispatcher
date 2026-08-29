package runner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestManagerCopiesTemplateMasksEnvironmentAndCleansAfterOneRun(t *testing.T) {
	manager, controller, state := testManager(t, time.Second, 1)
	process := controller.nextProcess()
	assigned := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- manager.Run(context.Background(), launchRequest("secret-jit"), assigned) }()
	spec := controller.waitForStart(t)
	if spec.WorkingDir == manager.TemplateDir || !strings.HasPrefix(spec.WorkingDir, filepath.Join(state, "runners")) {
		t.Fatalf("working directory = %s", spec.WorkingDir)
	}
	if _, err := os.Stat(filepath.Join(spec.WorkingDir, "template-marker")); err != nil {
		t.Fatalf("template was not copied: %v", err)
	}
	environment := strings.Join(spec.Environment, "\n")
	if !strings.Contains(environment, "ACTIONS_RUNNER_INPUT_JITCONFIG=secret-jit") || strings.Contains(environment, "GH_TOKEN=") || strings.Contains(environment, "GITHUB_TOKEN=") {
		t.Fatalf("unsafe child environment: %s", environment)
	}
	close(assigned)
	process.exit(nil)
	if err := <-done; err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if _, err := os.Stat(spec.WorkingDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("runner directory remains: %v", err)
	}
	if controller.starts() != 1 || manager.Active() != 0 {
		t.Fatalf("starts = %d, active = %d", controller.starts(), manager.Active())
	}
}

func TestManagerTerminatesUnassignedRunnerAfterTimeout(t *testing.T) {
	manager, controller, _ := testManager(t, 20*time.Millisecond, 1)
	process := controller.nextProcess()
	done := make(chan error, 1)
	go func() { done <- manager.Run(context.Background(), launchRequest("secret-jit"), make(chan struct{})) }()
	controller.waitForStart(t)
	controller.waitForTermination(t)
	process.exit(nil)
	if err := <-done; !errors.Is(err, ErrAcquisitionTimeout) {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestManagerEnforcesCapacity(t *testing.T) {
	manager, controller, _ := testManager(t, time.Second, 1)
	process := controller.nextProcess()
	done := make(chan error, 1)
	go func() { done <- manager.Run(context.Background(), launchRequest("one"), make(chan struct{})) }()
	controller.waitForStart(t)
	if err := manager.Run(context.Background(), launchRequest("two"), make(chan struct{})); !errors.Is(err, ErrCapacity) {
		t.Fatalf("second Run() error = %v", err)
	}
	process.exit(nil)
	<-done
}

func TestManagerUsesUniqueRunnerDirectories(t *testing.T) {
	manager, controller, _ := testManager(t, time.Second, 1)
	var directories []string
	for i := 0; i < 2; i++ {
		process := controller.nextProcess()
		done := make(chan error, 1)
		go func() { done <- manager.Run(context.Background(), launchRequest("secret"), make(chan struct{})) }()
		directories = append(directories, controller.waitForStart(t).WorkingDir)
		process.exit(nil)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if directories[0] == directories[1] {
		t.Fatalf("runner directory was reused: %s", directories[0])
	}
}

func testManager(t *testing.T, timeout time.Duration, capacity int) (*Manager, *fakeController, string) {
	t.Helper()
	root := t.TempDir()
	template := filepath.Join(root, "template")
	state := filepath.Join(root, "state")
	if err := os.MkdirAll(template, 0o700); err != nil {
		t.Fatal(err)
	}
	runnerScript := "run.sh"
	if runtime.GOOS == "windows" {
		runnerScript = "run.cmd"
	}
	for name, content := range map[string]string{runnerScript: "runner", "template-marker": "copied"} {
		if err := os.WriteFile(filepath.Join(template, name), []byte(content), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("GH_TOKEN", "must-not-leak")
	t.Setenv("GITHUB_TOKEN", "must-not-leak")
	controller := &fakeController{started: make(chan StartSpec, 4), terminated: make(chan ProcessIdentity, 4)}
	manager, err := NewManager(state, template, capacity, timeout, controller)
	if err != nil {
		t.Fatal(err)
	}
	return manager, controller, state
}

func launchRequest(secret string) LaunchRequest {
	return LaunchRequest{
		Repository:    RepositoryIdentity{Owner: "TKlerx", Name: "repo"},
		ObservedJobID: 7,
		RunnerID:      9,
		RunnerName:    "participant-abcd",
		JITConfig:     secret,
	}
}

type fakeController struct {
	mu         sync.Mutex
	processes  []*fakeProcess
	startCount int
	started    chan StartSpec
	terminated chan ProcessIdentity
	statuses   map[int]ProcessStatus
	inspectErr map[int]error
}

func (controller *fakeController) nextProcess() *fakeProcess {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	process := &fakeProcess{identity: ProcessIdentity{PID: len(controller.processes) + 100, StartMarker: "start", Executable: "runner"}, done: make(chan error, 1)}
	controller.processes = append(controller.processes, process)
	return process
}

func (controller *fakeController) Start(_ context.Context, spec StartSpec) (Process, error) {
	controller.mu.Lock()
	process := controller.processes[controller.startCount]
	controller.startCount++
	controller.mu.Unlock()
	controller.started <- spec
	return process, nil
}

func (controller *fakeController) Inspect(identity ProcessIdentity) (ProcessStatus, error) {
	if err := controller.inspectErr[identity.PID]; err != nil {
		return ProcessMismatched, err
	}
	if status, exists := controller.statuses[identity.PID]; exists {
		return status, nil
	}
	return ProcessMatches, nil
}

func (controller *fakeController) Terminate(identity ProcessIdentity) error {
	controller.terminated <- identity
	return nil
}

func (controller *fakeController) waitForStart(t *testing.T) StartSpec {
	t.Helper()
	select {
	case spec := <-controller.started:
		return spec
	case <-time.After(time.Second):
		t.Fatal("process did not start")
		return StartSpec{}
	}
}

func (controller *fakeController) waitForTermination(t *testing.T) {
	t.Helper()
	select {
	case <-controller.terminated:
	case <-time.After(time.Second):
		t.Fatal("process was not terminated")
	}
}

func (controller *fakeController) starts() int {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	return controller.startCount
}

type fakeProcess struct {
	identity ProcessIdentity
	done     chan error
}

func (process *fakeProcess) Identity() ProcessIdentity { return process.identity }
func (process *fakeProcess) Wait() error               { return <-process.done }
func (process *fakeProcess) exit(err error)            { process.done <- err }
