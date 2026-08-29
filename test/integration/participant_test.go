package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/TKlerx/github-runner-dispatcher/internal/config"
	ghapi "github.com/TKlerx/github-runner-dispatcher/internal/github"
	"github.com/TKlerx/github-runner-dispatcher/internal/participant"
	"github.com/TKlerx/github-runner-dispatcher/internal/runner"
)

func TestParticipantOffersOneJITRunnerForMatchingQueuedJob(t *testing.T) {
	var mu sync.Mutex
	posts := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/TKlerx/repo/actions/runs" && r.URL.Query().Get("status") == "queued":
			_, _ = w.Write([]byte(`{"workflow_runs":[{"id":5,"status":"queued"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/TKlerx/repo/actions/runs" && r.URL.Query().Get("status") == "in_progress":
			_, _ = w.Write([]byte(`{"workflow_runs":[]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/TKlerx/repo/actions/runs/5/jobs":
			_, _ = w.Write([]byte(`{"jobs":[{"id":11,"run_id":5,"name":"test","status":"queued","labels":["self-hosted","Windows","X64"]}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/TKlerx/repo/actions/jobs/11":
			_, _ = w.Write([]byte(`{"id":11,"run_id":5,"status":"queued","labels":["self-hosted","Windows","X64"]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/TKlerx/repo":
			_, _ = w.Write([]byte(`{"full_name":"TKlerx/repo","private":true}`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/TKlerx/repo/actions/runners/generate-jitconfig":
			var request ghapi.JITConfigRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			mu.Lock()
			posts = append(posts, r.URL.Path)
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"runner": map[string]any{"id": 42, "name": request.Name}, "encoded_jit_config": "secret-jit"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := ghapi.NewClient(server.URL, "2026-03-10", "token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	manager := &fakeRunnerManager{requests: make(chan runner.LaunchRequest, 1)}
	service, err := participant.NewService(config.Config{
		ParticipantName: "test", Repositories: []config.Repository{{Owner: "TKlerx", Name: "repo", RunnerGroupID: 1}},
		Labels: []string{"self-hosted", "Windows", "X64"}, PollInterval: time.Second, Capacity: 1,
	}, client, manager)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.PollOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case request := <-manager.requests:
		if request.ObservedJobID != 11 || request.RunnerID != 42 || request.JITConfig != "secret-jit" {
			t.Fatalf("launch request = %#v", request)
		}
	case <-time.After(time.Second):
		t.Fatal("runner was not launched")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(posts) != 1 {
		t.Fatalf("JIT POSTs = %v", posts)
	}
}

type fakeRunnerManager struct {
	requests chan runner.LaunchRequest
}

func (*fakeRunnerManager) Active() int { return 0 }
func (manager *fakeRunnerManager) Run(_ context.Context, request runner.LaunchRequest, _ <-chan struct{}) error {
	manager.requests <- request
	return nil
}
