package participant

import (
	"strings"
	"testing"

	"github.com/TKlerx/github-runner-dispatcher/internal/config"
)

func TestTrustedWorkflowAuthorization(t *testing.T) {
	t.Parallel()

	repository := policyRepository()
	base := policyJob()
	tests := []struct {
		name       string
		mutate     func(*ObservedJob)
		authorized bool
	}{
		{"unknown workflow with dedicated label", func(job *ObservedJob) { job.WorkflowPath = ".github/workflows/unknown.yml" }, false},
		{"coding job by another actor", func(job *ObservedJob) { job.Actor, job.TriggeringActor = "other", "other" }, false},
		{"fix job by another actor", func(job *ObservedJob) {
			job.WorkflowPath, job.Event, job.Actor, job.TriggeringActor = ".github/workflows/fix.yml", "issue_comment", "other", "other"
		}, false},
		{"pull request review by any actor", func(job *ObservedJob) {
			job.WorkflowID, job.WorkflowPath, job.Event, job.Actor, job.TriggeringActor = 77, ".github/workflows/review.yml", "pull_request", "contributor", "contributor"
		}, true},
		{"repository dispatch review by trusted actor", func(job *ObservedJob) {
			job.WorkflowID, job.WorkflowPath, job.Event = 77, ".github/workflows/review.yml", "repository_dispatch"
		}, true},
		{"repository dispatch review by another actor", func(job *ObservedJob) {
			job.WorkflowID, job.WorkflowPath, job.Event, job.Actor, job.TriggeringActor = 77, ".github/workflows/review.yml", "repository_dispatch", "other", "other"
		}, false},
		{"unauthorized rerun actor", func(job *ObservedJob) { job.TriggeringActor = "other" }, false},
		{"missing actor metadata", func(job *ObservedJob) { job.Actor = "" }, false},
		{"missing required label", func(job *ObservedJob) { job.Labels = []string{"self-hosted", "Linux", "X64"} }, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			job := base
			job.Labels = append([]string(nil), base.Labels...)
			test.mutate(&job)
			authorized, reason := authorizeJob(repository, job)
			if authorized != test.authorized {
				t.Fatalf("authorizeJob() = %v, %q", authorized, reason)
			}
		})
	}
}

func TestPolicyFreePrivateRepositoryRemainsAuthorized(t *testing.T) {
	t.Parallel()

	authorized, reason := authorizeJob(config.Repository{Owner: "TKlerx", Name: "private", Visibility: "private"}, ObservedJob{})
	if !authorized || !strings.Contains(reason, "no workflow policy") {
		t.Fatalf("authorizeJob() = %v, %q", authorized, reason)
	}
	if authorized, _ := authorizeJob(config.Repository{Owner: "TKlerx", Name: "public", Visibility: "public"}, ObservedJob{}); authorized {
		t.Fatal("policy-free public repository was authorized")
	}
}

func policyRepository() config.Repository {
	return config.Repository{
		Owner: "TKlerx", Name: "agent", Visibility: "public",
		TrustedWorkflows: []config.TrustedWorkflow{
			{WorkflowPath: ".github/workflows/issues.yml", Rules: []config.AuthorizationRule{{Events: []string{"issues", "issue_comment"}, Actors: []string{"TKlerx"}, RequiredLabels: []string{"dedicated"}}}},
			{WorkflowPath: ".github/workflows/fix.yml", Rules: []config.AuthorizationRule{{Events: []string{"issue_comment"}, Actors: []string{"TKlerx"}, RequiredLabels: []string{"dedicated"}}}},
			{WorkflowID: 77, Rules: []config.AuthorizationRule{
				{Events: []string{"pull_request"}, Actors: []string{"*"}, RequiredLabels: []string{"dedicated"}},
				{Events: []string{"repository_dispatch"}, Actors: []string{"TKlerx"}, RequiredLabels: []string{"dedicated"}},
			}},
		},
	}
}

func policyJob() ObservedJob {
	return ObservedJob{
		Repository: Repository{Owner: "TKlerx", Name: "agent"}, ServerRepository: "TKlerx/agent",
		RunID: 1, JobID: 2, Status: "queued", Labels: []string{"self-hosted", "Linux", "X64", "dedicated"},
		WorkflowID: 10, WorkflowPath: ".github/workflows/issues.yml", Event: "issue_comment",
		Actor: "TKlerx", TriggeringActor: "TKlerx",
	}
}
