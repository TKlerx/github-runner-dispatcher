package participant

import (
	"fmt"
	"strings"

	"github.com/TKlerx/github-runner-dispatcher/internal/config"
)

func authorizeJob(repository config.Repository, job ObservedJob) (bool, string) {
	if len(repository.TrustedWorkflows) == 0 {
		if repository.Visibility == "" || repository.Visibility == "private" {
			return true, "private repository has no workflow policy"
		}
		return false, "public repository has no workflow policy"
	}
	if job.RunID < 1 || job.JobRunID != job.RunID || job.JobID < 1 || job.ServerRepository == "" ||
		job.WorkflowID < 1 || job.WorkflowPath == "" || job.Event == "" ||
		job.Actor == "" || job.TriggeringActor == "" {
		return false, "policy metadata is incomplete"
	}
	configuredName := repository.Owner + "/" + repository.Name
	if !strings.EqualFold(job.ServerRepository, configuredName) || job.RepositoryPrivate != (repository.Visibility == "private") {
		return false, "repository identity or visibility does not match policy"
	}
	for _, workflow := range repository.TrustedWorkflows {
		if !workflowMatches(workflow, job) {
			continue
		}
		for _, rule := range workflow.Rules {
			if ruleMatches(rule, job) {
				return true, "trusted workflow rule matched"
			}
		}
		return false, "workflow matched but event, actor, or labels did not"
	}
	return false, "workflow is not trusted"
}

func workflowMatches(workflow config.TrustedWorkflow, job ObservedJob) bool {
	if workflow.WorkflowID > 0 {
		return workflow.WorkflowID == job.WorkflowID
	}
	return workflow.WorkflowPath == job.WorkflowPath
}

func ruleMatches(rule config.AuthorizationRule, job ObservedJob) bool {
	if !contains(rule.Events, job.Event) || !labelsMatch(rule.RequiredLabels, job.Labels) {
		return false
	}
	if len(rule.Actors) == 1 && rule.Actors[0] == "*" {
		return job.Actor != "" && job.TriggeringActor != ""
	}
	return containsFold(rule.Actors, job.Actor) && containsFold(rule.Actors, job.TriggeringActor)
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func containsFold(values []string, wanted string) bool {
	for _, value := range values {
		if strings.EqualFold(value, wanted) {
			return true
		}
	}
	return false
}

func policyIdentity(job ObservedJob) string {
	return fmt.Sprintf("workflow_id=%d path=%s event=%s actor=%s triggering_actor=%s", job.WorkflowID, job.WorkflowPath, job.Event, job.Actor, job.TriggeringActor)
}
