package participant

import (
	"context"
	"errors"
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

func TestFinalPolicyRecheckFailsClosedBeforeJIT(t *testing.T) {
	t.Parallel()

	baseRun := ghapi.WorkflowRun{
		ID: 1, Status: "queued", WorkflowID: 77, Path: ".github/workflows/review.yml", Event: "pull_request",
		Actor: ghapi.Actor{Login: "contributor"}, TriggeringActor: ghapi.Actor{Login: "contributor"},
		Repository: ghapi.RepositoryInfo{FullName: "TKlerx/agent", Private: false},
	}
	baseJob := ghapi.Job{ID: 2, RunID: 1, Status: "queued", Labels: []string{"self-hosted", "Linux", "X64", "dedicated"}}
	tests := []struct {
		name   string
		mutate func(*ghapi.WorkflowRun, *ghapi.Job)
		err    error
	}{
		{"missing actor", func(run *ghapi.WorkflowRun, _ *ghapi.Job) { run.Actor.Login = "" }, nil},
		{"changed workflow", func(run *ghapi.WorkflowRun, _ *ghapi.Job) { run.WorkflowID = 88 }, nil},
		{"changed labels", func(_ *ghapi.WorkflowRun, job *ghapi.Job) { job.Labels = []string{"self-hosted", "Linux", "X64"} }, nil},
		{"inconsistent run identity", func(run *ghapi.WorkflowRun, _ *ghapi.Job) { run.ID = 9 }, nil},
		{"missing run response", func(*ghapi.WorkflowRun, *ghapi.Job) {}, errors.New("missing run")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			finalRun, finalJob := baseRun, baseJob
			finalJob.Labels = append([]string(nil), baseJob.Labels...)
			test.mutate(&finalRun, &finalJob)
			api := &policyRaceAPI{observedRun: baseRun, finalRun: finalRun, job: finalJob, runErr: test.err}
			manager := &blockingRunner{release: make(chan struct{})}
			service, err := NewService(policyRaceConfig(), api, manager)
			if err != nil {
				t.Fatal(err)
			}
			_ = service.PollOnce(context.Background())
			if api.jitCalls.Load() != 0 || manager.calls.Load() != 0 {
				t.Fatalf("JIT calls = %d, runner calls = %d", api.jitCalls.Load(), manager.calls.Load())
			}
		})
	}
}

func TestFinalPolicyRecheckAllowsConsistentMetadata(t *testing.T) {
	run := ghapi.WorkflowRun{
		ID: 1, Status: "queued", WorkflowID: 77, Path: ".github/workflows/review.yml", Event: "pull_request",
		Actor: ghapi.Actor{Login: "contributor"}, TriggeringActor: ghapi.Actor{Login: "contributor"},
		Repository: ghapi.RepositoryInfo{FullName: "TKlerx/agent", Private: false},
	}
	job := ghapi.Job{ID: 2, RunID: 1, Status: "queued", Labels: []string{"self-hosted", "Linux", "X64", "dedicated"}}
	api := &policyRaceAPI{observedRun: run, finalRun: run, job: job}
	manager := &blockingRunner{release: make(chan struct{})}
	service, err := NewService(policyRaceConfig(), api, manager)
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

func policyRaceConfig() config.Config {
	repository := policyRepository()
	repository.RunnerGroupID = 1
	return config.Config{
		ParticipantName: "test", Repositories: []config.Repository{repository},
		Labels: []string{"self-hosted", "Linux", "X64", "dedicated"}, PollInterval: 10 * time.Millisecond, Capacity: 1,
	}
}

type raceAPI struct {
	finalStatus string
	jitCalls    atomic.Int32
	deleteCalls atomic.Int32
}

func (*raceAPI) ValidatePrivateRepository(context.Context, ghapi.Repository) error { return nil }
func (*raceAPI) ListWorkflowRuns(_ context.Context, _ ghapi.Repository, status string) ([]ghapi.WorkflowRun, error) {
	if status == "queued" {
		return []ghapi.WorkflowRun{{ID: 1, Status: "queued", Repository: ghapi.RepositoryInfo{FullName: "TKlerx/repo", Private: true}}}, nil
	}
	return nil, nil
}
func (*raceAPI) ListJobs(context.Context, ghapi.Repository, int64) ([]ghapi.Job, error) {
	return []ghapi.Job{{ID: 2, RunID: 1, Status: "queued", Labels: []string{"self-hosted", "Windows", "X64"}}}, nil
}
func (api *raceAPI) GetJob(context.Context, ghapi.Repository, int64) (ghapi.Job, error) {
	return ghapi.Job{ID: 2, RunID: 1, Status: api.finalStatus, Labels: []string{"self-hosted", "Windows", "X64"}}, nil
}
func (*raceAPI) GetWorkflowRun(context.Context, ghapi.Repository, int64) (ghapi.WorkflowRun, error) {
	return ghapi.WorkflowRun{ID: 1, Status: "queued", Repository: ghapi.RepositoryInfo{FullName: "TKlerx/repo", Private: true}}, nil
}
func (api *raceAPI) GenerateJITConfig(context.Context, ghapi.Repository, ghapi.JITConfigRequest) (ghapi.JITConfig, error) {
	api.jitCalls.Add(1)
	var response ghapi.JITConfig
	response.Runner.ID = 3
	response.EncodedJITConfig = "secret"
	return response, nil
}
func (api *raceAPI) DeleteRunner(context.Context, ghapi.Repository, int64) error {
	api.deleteCalls.Add(1)
	return nil
}

type policyRaceAPI struct {
	raceAPI
	observedRun ghapi.WorkflowRun
	finalRun    ghapi.WorkflowRun
	job         ghapi.Job
	runErr      error
}

func (api *policyRaceAPI) ListWorkflowRuns(_ context.Context, _ ghapi.Repository, status string) ([]ghapi.WorkflowRun, error) {
	if status == "queued" {
		return []ghapi.WorkflowRun{api.observedRun}, nil
	}
	return nil, nil
}

func (api *policyRaceAPI) ListJobs(context.Context, ghapi.Repository, int64) ([]ghapi.Job, error) {
	return []ghapi.Job{api.job}, nil
}

func (api *policyRaceAPI) GetJob(context.Context, ghapi.Repository, int64) (ghapi.Job, error) {
	return api.job, nil
}

func (api *policyRaceAPI) GetWorkflowRun(context.Context, ghapi.Repository, int64) (ghapi.WorkflowRun, error) {
	return api.finalRun, api.runErr
}

type blockingRunner struct {
	calls   atomic.Int32
	release chan struct{}
}

func TestRegistrationCleanupRunsForEveryTerminalOutcome(t *testing.T) {
	for name, outcome := range map[string]error{
		"completed": nil,
		"failed":    errors.New("job failed"),
		"cancelled": context.Canceled,
		"timed out": runner.ErrAcquisitionTimeout,
	} {
		t.Run(name, func(t *testing.T) {
			api := &raceAPI{finalStatus: "queued"}
			service, err := NewService(raceConfig(), api, terminalRunner{outcome: outcome})
			if err != nil {
				t.Fatal(err)
			}
			if err := service.PollOnce(context.Background()); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(time.Second)
			for api.deleteCalls.Load() == 0 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if api.deleteCalls.Load() != 1 {
				t.Fatalf("registration cleanup calls = %d", api.deleteCalls.Load())
			}
		})
	}
}

type terminalRunner struct{ outcome error }

func (terminalRunner) Active() int                     { return 0 }
func (terminalRunner) Reconcile(context.Context) error { return nil }
func (manager terminalRunner) Run(context.Context, runner.LaunchRequest, <-chan struct{}) error {
	return manager.outcome
}

func (*blockingRunner) Active() int                     { return 0 }
func (*blockingRunner) Reconcile(context.Context) error { return nil }
func (manager *blockingRunner) Run(context.Context, runner.LaunchRequest, <-chan struct{}) error {
	manager.calls.Add(1)
	<-manager.release
	return nil
}
