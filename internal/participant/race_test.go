package participant

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TKlerx/github-runner-dispatcher/internal/config"
	ghapi "github.com/TKlerx/github-runner-dispatcher/internal/github"
	"github.com/TKlerx/github-runner-dispatcher/internal/runner"
)

func TestFinalRecheckSkipsCompletedJobWithoutJITPost(t *testing.T) {
	api := &raceAPI{finalStatus: "completed"}
	manager := &blockingRunner{release: make(chan struct{})}
	service, err := NewService(raceConfig(), api, manager)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.PollOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if api.jitCalls.Load() != 0 || manager.calls.Load() != 0 {
		t.Fatalf("JIT calls = %d, runner calls = %d", api.jitCalls.Load(), manager.calls.Load())
	}
}

func TestRepeatedObservationDoesNotOfferSameActiveJobTwice(t *testing.T) {
	api := &raceAPI{finalStatus: "queued"}
	manager := &blockingRunner{release: make(chan struct{})}
	service, err := NewService(raceConfig(), api, manager)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.PollOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for manager.calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if err := service.PollOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if api.jitCalls.Load() != 1 || manager.calls.Load() != 1 {
		t.Fatalf("JIT calls = %d, runner calls = %d", api.jitCalls.Load(), manager.calls.Load())
	}
	close(manager.release)
}

func raceConfig() config.Config {
	return config.Config{
		ParticipantName: "test", Repositories: []config.Repository{{Owner: "TKlerx", Name: "repo", RunnerGroupID: 1}},
		Labels: []string{"self-hosted", "Windows", "X64"}, PollInterval: 10 * time.Millisecond, Capacity: 1,
	}
}

type raceAPI struct {
	finalStatus string
	jitCalls    atomic.Int32
}

func (*raceAPI) ValidatePrivateRepository(context.Context, ghapi.Repository) error { return nil }
func (*raceAPI) ListWorkflowRuns(_ context.Context, _ ghapi.Repository, status string) ([]ghapi.WorkflowRun, error) {
	if status == "queued" {
		return []ghapi.WorkflowRun{{ID: 1, Status: "queued"}}, nil
	}
	return nil, nil
}
func (*raceAPI) ListJobs(context.Context, ghapi.Repository, int64) ([]ghapi.Job, error) {
	return []ghapi.Job{{ID: 2, RunID: 1, Status: "queued", Labels: []string{"self-hosted", "Windows", "X64"}}}, nil
}
func (api *raceAPI) GetJob(context.Context, ghapi.Repository, int64) (ghapi.Job, error) {
	return ghapi.Job{ID: 2, RunID: 1, Status: api.finalStatus, Labels: []string{"self-hosted", "Windows", "X64"}}, nil
}
func (api *raceAPI) GenerateJITConfig(context.Context, ghapi.Repository, ghapi.JITConfigRequest) (ghapi.JITConfig, error) {
	api.jitCalls.Add(1)
	var response ghapi.JITConfig
	response.Runner.ID = 3
	response.EncodedJITConfig = "secret"
	return response, nil
}

type blockingRunner struct {
	calls   atomic.Int32
	release chan struct{}
}

func (*blockingRunner) Active() int                     { return 0 }
func (*blockingRunner) Reconcile(context.Context) error { return nil }
func (manager *blockingRunner) Run(context.Context, runner.LaunchRequest, <-chan struct{}) error {
	manager.calls.Add(1)
	<-manager.release
	return nil
}
