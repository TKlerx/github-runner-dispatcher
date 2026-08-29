package participant

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/TKlerx/github-runner-dispatcher/internal/config"
	ghapi "github.com/TKlerx/github-runner-dispatcher/internal/github"
	"github.com/TKlerx/github-runner-dispatcher/internal/runner"
)

type GitHubAPI interface {
	ValidatePrivateRepository(context.Context, ghapi.Repository) error
	ListWorkflowRuns(context.Context, ghapi.Repository, string) ([]ghapi.WorkflowRun, error)
	ListJobs(context.Context, ghapi.Repository, int64) ([]ghapi.Job, error)
	GetJob(context.Context, ghapi.Repository, int64) (ghapi.Job, error)
	GenerateJITConfig(context.Context, ghapi.Repository, ghapi.JITConfigRequest) (ghapi.JITConfig, error)
}

type RunnerManager interface {
	Active() int
	Run(context.Context, runner.LaunchRequest, <-chan struct{}) error
}

type Service struct {
	config  config.Config
	github  GitHubAPI
	runners RunnerManager
	mu      sync.Mutex
	active  map[string]struct{}
}

func NewService(cfg config.Config, github GitHubAPI, runners RunnerManager) (*Service, error) {
	if github == nil || runners == nil {
		return nil, errors.New("GitHub client and runner manager are required")
	}
	return &Service{config: cfg, github: github, runners: runners, active: map[string]struct{}{}}, nil
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
	jobs, err := service.observe(ctx)
	if err != nil {
		return err
	}
	for _, job := range matchingQueuedJobs(jobs, service.config.Labels) {
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
					observed = append(observed, ObservedJob{
						Repository: Repository{Owner: configured.Owner, Name: configured.Name}, RunID: run.ID,
						JobID: job.ID, Name: job.Name, Status: job.Status, Labels: append([]string(nil), job.Labels...), RunnerName: job.RunnerName,
					})
				}
			}
		}
	}
	return observed, nil
}

func (service *Service) offer(ctx context.Context, observed ObservedJob) error {
	repository := ghapi.Repository{Owner: observed.Repository.Owner, Name: observed.Repository.Name}
	job, err := service.github.GetJob(ctx, repository, observed.JobID)
	if err != nil {
		return err
	}
	if job.Status != "queued" || !labelsMatch(job.Labels, service.config.Labels) {
		return nil
	}
	if err := service.github.ValidatePrivateRepository(ctx, repository); err != nil {
		return err
	}
	configured, ok := service.configuredRepository(observed.Repository)
	if !ok || service.localCapacity() < 1 {
		return nil
	}
	runnerName, err := uniqueRunnerName(service.config.ParticipantName)
	if err != nil {
		return err
	}
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
		_ = service.runners.Run(ctx, runner.LaunchRequest{
			Repository:    runner.RepositoryIdentity{Owner: observed.Repository.Owner, Name: observed.Repository.Name},
			ObservedJobID: observed.JobID, RunnerID: jit.Runner.ID, RunnerName: runnerName, JITConfig: jit.EncodedJITConfig,
		}, assigned)
		service.mu.Lock()
		delete(service.active, key)
		service.mu.Unlock()
	}()
	go service.watchAssignment(ctx, repository, observed.JobID, runnerName, assigned, done)
	return nil
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
	return service.config.Capacity - len(service.active)
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
