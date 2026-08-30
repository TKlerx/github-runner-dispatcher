package participant

import (
	"reflect"
	"testing"

	"github.com/TKlerx/github-runner-dispatcher/internal/config"
	ghapi "github.com/TKlerx/github-runner-dispatcher/internal/github"
)

func TestMatchingQueuedJobsUsesCaseInsensitiveLabelSubset(t *testing.T) {
	jobs := []ObservedJob{
		{Repository: Repository{Owner: "TKlerx", Name: "repo"}, RunID: 2, JobID: 3, Status: "queued", Labels: []string{"SELF-HOSTED", "windows", "x64"}},
		{Repository: Repository{Owner: "TKlerx", Name: "repo"}, RunID: 2, JobID: 4, Status: "queued", Labels: []string{"self-hosted", "Linux", "X64"}},
		{Repository: Repository{Owner: "TKlerx", Name: "repo"}, RunID: 2, JobID: 5, Status: "in_progress", Labels: []string{"self-hosted", "Windows", "X64"}},
	}

	matched := matchingQueuedJobs(jobs, []string{"self-hosted", "Windows", "X64", "strong"})
	if len(matched) != 1 || matched[0].JobID != 3 {
		t.Fatalf("matched = %#v", matched)
	}
}

func TestMatchingQueuedJobsOrdersDeterministically(t *testing.T) {
	jobs := []ObservedJob{
		{Repository: Repository{Owner: "TKlerx", Name: "zeta"}, RunID: 1, JobID: 1, Status: "queued", Labels: []string{"self-hosted"}},
		{Repository: Repository{Owner: "TKlerx", Name: "alpha"}, RunID: 2, JobID: 3, Status: "queued", Labels: []string{"self-hosted"}},
		{Repository: Repository{Owner: "TKlerx", Name: "alpha"}, RunID: 1, JobID: 2, Status: "queued", Labels: []string{"self-hosted"}},
	}

	matched := matchingQueuedJobs(jobs, []string{"self-hosted"})
	ids := []int64{matched[0].JobID, matched[1].JobID, matched[2].JobID}
	if !reflect.DeepEqual(ids, []int64{2, 3, 1}) {
		t.Fatalf("job order = %v", ids)
	}
}

func TestLabelsMatchRejectsUnmatchedOperatingSystem(t *testing.T) {
	if labelsMatch([]string{"self-hosted", "Linux", "X64"}, []string{"self-hosted", "Windows", "X64"}) {
		t.Fatal("Windows participant matched a Linux job")
	}
}

func TestObservedJobUsesOnlyGitHubRunAndJobMetadata(t *testing.T) {
	repository := policyRepository()
	run := ghapi.WorkflowRun{
		ID: 1, WorkflowID: 77, Path: ".github/workflows/review.yml", Event: "pull_request",
		Actor: ghapi.Actor{Login: "contributor"}, TriggeringActor: ghapi.Actor{Login: "contributor"},
		Repository: ghapi.RepositoryInfo{FullName: "TKlerx/agent", Private: false},
	}
	job := ghapi.Job{ID: 2, RunID: 1, Status: "queued", Labels: []string{"self-hosted", "Linux", "X64", "dedicated"}}

	observed := observedJob(repository, run, job)
	authorized, reason := authorizeJob(repository, observed)
	if !authorized {
		t.Fatalf("authorizeJob() = false, %q; observed = %#v", reason, observed)
	}
	job.RunID = 9
	if authorized, _ := authorizeJob(repository, observedJob(repository, run, job)); authorized {
		t.Fatal("job linked to a different run was authorized")
	}
}

func TestObservedJobPreservesPolicyFreePrivateBehavior(t *testing.T) {
	repository := config.Repository{Owner: "TKlerx", Name: "private", Visibility: "private"}
	job := observedJob(repository, ghapi.WorkflowRun{ID: 1}, ghapi.Job{ID: 2, RunID: 1, Status: "queued", Labels: []string{"self-hosted"}})
	if authorized, reason := authorizeJob(repository, job); !authorized {
		t.Fatalf("legacy private job denied: %s", reason)
	}
}
