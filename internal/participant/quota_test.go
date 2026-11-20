package participant

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	ghapi "github.com/TKlerx/github-runner-dispatcher/internal/github"
)

type quotaAPI struct {
	raceAPI
	calls atomic.Int32
}

func (api *quotaAPI) ListWorkflowRuns(context.Context, ghapi.Repository, string) ([]ghapi.WorkflowRun, error) {
	api.calls.Add(1)
	return nil, &ghapi.APIError{StatusCode: 403, RateLimitReset: time.Now().Add(time.Hour)}
}

func TestQuotaCooldownHonorsResetAndCancellation(t *testing.T) {
	cfg := raceConfig()
	cfg.PollInterval = time.Millisecond
	api := &quotaAPI{}
	service, err := NewService(cfg, api, &blockingRunner{release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := service.Run(ctx); err != nil || api.calls.Load() != 1 {
		t.Fatalf("Run=%v, requests while quota exhausted=%d", err, api.calls.Load())
	}
}
