package participant

import (
	"context"
	"testing"
	"time"

	ghapi "github.com/TKlerx/github-runner-dispatcher/internal/github"
)

func TestWatchAssignmentFindsDifferentJobOnThisRunner(t *testing.T) {
	for _, status := range []string{"in_progress", "completed"} {
		t.Run(status, func(t *testing.T) {
			api := &assignmentAPI{raceAPI: raceAPI{finalStatus: "queued"}, assignedJob: ghapi.Job{ID: 99, RunID: 3, Status: status, RunnerName: "test-runner"}}
			service, err := NewService(raceConfig(), api, terminalRunner{})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			assigned := make(chan struct{})
			go service.watchAssignment(ctx, ghapi.Repository{Owner: "TKlerx", Name: "repo"}, 2, "test-runner", assigned, make(chan struct{}))
			select {
			case <-assigned:
			case <-ctx.Done():
				t.Fatal("different job assigned to our runner was treated as idle")
			}
		})
	}
}

func TestRunnerAssignedRejectsOtherRunnerAndQueuedJob(t *testing.T) {
	for _, job := range []ghapi.Job{
		{Status: "in_progress", RunnerName: "another-runner"},
		{Status: "queued", RunnerName: "test-runner"},
	} {
		api := &assignmentAPI{assignedJob: job}
		service, err := NewService(raceConfig(), api, terminalRunner{})
		if err != nil {
			t.Fatal(err)
		}
		if service.runnerAssigned(context.Background(), ghapi.Repository{}, "test-runner") {
			t.Fatal("unassigned runner treated as busy")
		}
	}
}

type assignmentAPI struct {
	raceAPI
	assignedJob ghapi.Job
}

func (api *assignmentAPI) ListWorkflowRuns(context.Context, ghapi.Repository, string) ([]ghapi.WorkflowRun, error) {
	return []ghapi.WorkflowRun{{ID: 3, Status: "in_progress"}}, nil
}

func (api *assignmentAPI) ListJobs(context.Context, ghapi.Repository, int64) ([]ghapi.Job, error) {
	return []ghapi.Job{api.assignedJob}, nil
}
