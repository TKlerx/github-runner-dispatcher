package participant

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/TKlerx/github-runner-dispatcher/internal/config"
	ghapi "github.com/TKlerx/github-runner-dispatcher/internal/github"
	"github.com/TKlerx/github-runner-dispatcher/internal/runner"
)

type GitHubAPI interface {
	ListWorkflowRuns(context.Context, ghapi.Repository, string) ([]ghapi.WorkflowRun, error)
	ListJobs(context.Context, ghapi.Repository, int64) ([]ghapi.Job, error)
	GetJob(context.Context, ghapi.Repository, int64) (ghapi.Job, error)
	GetWorkflowRun(context.Context, ghapi.Repository, int64) (ghapi.WorkflowRun, error)
	GenerateJITConfig(context.Context, ghapi.Repository, ghapi.JITConfigRequest) (ghapi.JITConfig, error)
	DeleteRunner(context.Context, ghapi.Repository, int64) error
}

type RunnerManager interface {
	Active() int
	Reconcile(context.Context) error
	Run(context.Context, runner.LaunchRequest, <-chan struct{}) error
}

type Service struct {
	config  config.Config
	github  GitHubAPI
	runners RunnerManager
	mu      sync.Mutex
	active  map[string]struct{}
	claims  *claimTracker
	now     func() time.Time
	logger  *DecisionLogger
}

func NewService(cfg config.Config, github GitHubAPI, runners RunnerManager, loggers ...*DecisionLogger) (*Service, error) {
	if github == nil || runners == nil {
		return nil, errors.New("GitHub client and runner manager are required")
	}
	staleAfter := 10 * cfg.PollInterval
	if staleAfter < time.Minute {
		staleAfter = time.Minute
	}
	logger := NewDecisionLogger(io.Discard)
	if len(loggers) > 0 && loggers[0] != nil {
		logger = loggers[0]
	}
	return &Service{config: cfg, github: github, runners: runners, active: map[string]struct{}{}, claims: newClaimTracker(10_000, staleAfter), now: time.Now, logger: logger}, nil
}

func (service *Service) Run(ctx context.Context) error {
	ticker := time.NewTicker(service.config.PollInterval)
	defer ticker.Stop()
	for {
		_ = service.PollOnce(ctx)
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (service *Service) PollOnce(ctx context.Context) error {
	if err := service.runners.Reconcile(ctx); err != nil && service.runners.Active() >= service.config.Capacity {
		service.log(ObservedJob{}, DecisionError, "runner reconciliation failed", err.Error())
		return err
	}
	jobs, err := service.observe(ctx)
	if err != nil {
		service.log(ObservedJob{}, DecisionError, "GitHub observation failed", err.Error())
		return err
	}
	matching := matchingQueuedJobs(jobs, service.config.Labels)
	now := service.now()
	tracked := service.claims.observe(now, matching)
	for _, job := range tracked {
		service.log(job, DecisionWait, "matching queued job observed", "eligible at "+job.FirstSeenAt.Add(service.config.ClaimDelay).UTC().Format(time.RFC3339Nano))
	}
	for _, job := range eligibleClaims(now, service.config.ClaimDelay, tracked) {
		if service.localCapacity() < 1 || service.isActive(job) {
			continue
		}
		if err := service.offer(ctx, job); err != nil {
			return err
		}
	}
	return nil
}

func (service *Service) observe(ctx context.Context) ([]ObservedJob, error) {
	var observed []ObservedJob
	for _, configured := range service.config.Repositories {
		repository := ghapi.Repository{Owner: configured.Owner, Name: configured.Name}
		for _, status := range []string{"queued", "in_progress"} {
			runs, err := service.github.ListWorkflowRuns(ctx, repository, status)
			if err != nil {
				return nil, err
			}
			for _, run := range runs {
				jobs, err := service.github.ListJobs(ctx, repository, run.ID)
				if err != nil {
					return nil, err
				}
				for _, job := range jobs {
					candidate := observedJob(configured, run, job)
					authorized, reason := authorizeJob(configured, candidate)
					if !authorized {
						service.log(candidate, DecisionIgnore, "workflow authorization denied", reason+"; "+policyIdentity(candidate))
						continue
					}
					observed = append(observed, candidate)
				}
			}
		}
	}
	return observed, nil
}

func observedJob(repository config.Repository, run ghapi.WorkflowRun, job ghapi.Job) ObservedJob {
	return ObservedJob{
		Repository:       Repository{Owner: repository.Owner, Name: repository.Name},
		ServerRepository: run.Repository.FullName, RepositoryPrivate: run.Repository.Private,
		RunID: run.ID, JobRunID: job.RunID, JobID: job.ID, Name: job.Name, Status: job.Status,
		Labels: append([]string(nil), job.Labels...), RunnerName: job.RunnerName,
		WorkflowID: run.WorkflowID, WorkflowPath: run.Path, Event: run.Event,
		Actor: run.Actor.Login, TriggeringActor: run.TriggeringActor.Login,
	}
}

func (service *Service) offer(ctx context.Context, observed ObservedJob) error {
	repository := ghapi.Repository{Owner: observed.Repository.Owner, Name: observed.Repository.Name}
	job, err := service.github.GetJob(ctx, repository, observed.JobID)
	if err != nil {
		return err
	}
	run, err := service.github.GetWorkflowRun(ctx, repository, observed.RunID)
	if err != nil {
		return err
	}
	configured, ok := service.configuredRepository(observed.Repository)
	if !ok || service.localCapacity() < 1 {
		return nil
	}
	final := observedJob(configured, run, job)
	if !finalMetadataConsistent(configured, observed, final, run.Status) || !labelsMatch(job.Labels, service.config.Labels) {
		service.log(observed, DecisionIgnore, "final job recheck no longer matches", job.Status)
		return nil
	}
	authorized, reason := authorizeJob(configured, final)
	if !authorized {
		service.log(observed, DecisionIgnore, "final workflow authorization denied", reason+"; "+policyIdentity(final))
		return nil
	}
	runnerName, err := uniqueRunnerName(service.config.ParticipantName)
	if err != nil {
		return err
	}
	service.log(observed, DecisionOffer, "final recheck passed", "JIT runner created")
	jit, err := service.github.GenerateJITConfig(ctx, repository, ghapi.JITConfigRequest{
		Name: runnerName, RunnerGroupID: configured.RunnerGroupID, Labels: append([]string(nil), service.config.Labels...), WorkFolder: "_work",
	})
	if err != nil {
		return err
	}
	key := jobKey(observed)
	service.mu.Lock()
	if _, exists := service.active[key]; exists || len(service.active) >= service.config.Capacity {
		service.mu.Unlock()
		return nil
	}
	service.active[key] = struct{}{}
	service.mu.Unlock()
	assigned := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		runErr := service.runners.Run(ctx, runner.LaunchRequest{
			Repository:    runner.RepositoryIdentity{Owner: observed.Repository.Owner, Name: observed.Repository.Name},
			ObservedJobID: observed.JobID, RunnerID: jit.Runner.ID, RunnerName: runnerName, JITConfig: jit.EncodedJITConfig,
		}, assigned)
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		deleteErr := service.github.DeleteRunner(cleanupCtx, repository, jit.Runner.ID)
		cancel()
		var apiError *ghapi.APIError
		if deleteErr != nil && (!errors.As(deleteErr, &apiError) || apiError.StatusCode != http.StatusNotFound) {
			runErr = errors.Join(runErr, fmt.Errorf("remove GitHub runner registration: %w", deleteErr))
		}
		outcome := "runner completed"
		if runErr != nil {
			outcome = runErr.Error()
		}
		service.log(observed, DecisionCleanup, "runner lifecycle ended", outcome)
		service.mu.Lock()
		delete(service.active, key)
		service.mu.Unlock()
	}()
	go service.watchAssignment(ctx, repository, observed.JobID, runnerName, assigned, done)
	return nil
}

func finalMetadataConsistent(repository config.Repository, observed, final ObservedJob, runStatus string) bool {
	if final.JobID != observed.JobID || final.RunID != observed.RunID || final.JobRunID != observed.RunID ||
		final.Status != "queued" || (runStatus != "queued" && runStatus != "in_progress") ||
		!strings.EqualFold(final.ServerRepository, repository.Owner+"/"+repository.Name) ||
		final.RepositoryPrivate != (repository.Visibility == "" || repository.Visibility == "private") ||
		!sameLabels(final.Labels, observed.Labels) {
		return false
	}
	if len(repository.TrustedWorkflows) == 0 {
		return true
	}
	return final.WorkflowID == observed.WorkflowID && final.WorkflowPath == observed.WorkflowPath &&
		final.Event == observed.Event && strings.EqualFold(final.Actor, observed.Actor) &&
		strings.EqualFold(final.TriggeringActor, observed.TriggeringActor)
}

func sameLabels(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	counts := make(map[string]int, len(left))
	for _, label := range left {
		counts[strings.ToLower(label)]++
	}
	for _, label := range right {
		key := strings.ToLower(label)
		counts[key]--
		if counts[key] < 0 {
			return false
		}
	}
	return true
}

func (service *Service) log(job ObservedJob, decision Decision, reason, outcome string) {
	service.logger.Log(ParticipationDecision{
		Repository: job.Repository, JobID: job.JobID, Participant: service.config.ParticipantName,
		Decision: decision, Reason: reason, Outcome: outcome, Timestamp: service.now().UTC(),
	})
}

func (service *Service) watchAssignment(ctx context.Context, repository ghapi.Repository, jobID int64, runnerName string, assigned chan struct{}, done <-chan struct{}) {
	ticker := time.NewTicker(service.config.PollInterval)
	defer ticker.Stop()
	for {
		job, err := service.github.GetJob(ctx, repository, jobID)
		if err == nil && job.Status == "in_progress" && job.RunnerName == runnerName {
			close(assigned)
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-ticker.C:
		}
	}
}

func (service *Service) localCapacity() int {
	service.mu.Lock()
	defer service.mu.Unlock()
	used := len(service.active)
	if recoveredOrRunning := service.runners.Active(); recoveredOrRunning > used {
		used = recoveredOrRunning
	}
	return service.config.Capacity - used
}

func (service *Service) isActive(job ObservedJob) bool {
	service.mu.Lock()
	defer service.mu.Unlock()
	_, exists := service.active[jobKey(job)]
	return exists
}

func (service *Service) configuredRepository(repository Repository) (config.Repository, bool) {
	for _, configured := range service.config.Repositories {
		if strings.EqualFold(configured.Owner, repository.Owner) && strings.EqualFold(configured.Name, repository.Name) {
			return configured, true
		}
	}
	return config.Repository{}, false
}

func matchingQueuedJobs(jobs []ObservedJob, participantLabels []string) []ObservedJob {
	matched := make([]ObservedJob, 0, len(jobs))
	for _, job := range jobs {
		if job.Status == "queued" && labelsMatch(job.Labels, participantLabels) {
			matched = append(matched, job)
		}
	}
	sort.Slice(matched, func(i, j int) bool {
		left, right := matched[i], matched[j]
		if repository := strings.Compare(strings.ToLower(left.Repository.Owner+"/"+left.Repository.Name), strings.ToLower(right.Repository.Owner+"/"+right.Repository.Name)); repository != 0 {
			return repository < 0
		}
		if left.RunID != right.RunID {
			return left.RunID < right.RunID
		}
		return left.JobID < right.JobID
	})
	return matched
}

func labelsMatch(required, available []string) bool {
	labels := make(map[string]bool, len(available))
	for _, label := range available {
		labels[strings.ToLower(label)] = true
	}
	for _, label := range required {
		if !labels[strings.ToLower(label)] {
			return false
		}
	}
	return true
}

func uniqueRunnerName(participant string) (string, error) {
	random := make([]byte, 4)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("create runner name: %w", err)
	}
	return participant + "-" + hex.EncodeToString(random), nil
}

func jobKey(job ObservedJob) string {
	return strings.ToLower(job.Repository.Owner+"/"+job.Repository.Name) + ":" + fmt.Sprint(job.JobID)
}
