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

func TestParticipantPreferenceFallbackAndHarmlessRedundancy(t *testing.T) {
	queue := &sharedQueue{queued: true, assignOnPost: true}
	server := httptest.NewServer(http.HandlerFunc(queue.serveHTTP))
	defer server.Close()
	client, err := ghapi.NewClient(server.URL, "2026-03-10", "token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	newService := func(delay time.Duration) *participant.Service {
		service, serviceErr := participant.NewService(config.Config{
			ParticipantName: "test", Repositories: []config.Repository{{Owner: "TKlerx", Name: "repo", RunnerGroupID: 1}},
			Labels: []string{"self-hosted", "Windows", "X64"}, PollInterval: time.Millisecond, ClaimDelay: delay, Capacity: 1,
		}, client, &fakeRunnerManager{requests: make(chan runner.LaunchRequest, 4)})
		if serviceErr != nil {
			t.Fatal(serviceErr)
		}
		return service
	}

	strong, fallback := newService(0), newService(5*time.Millisecond)
	if err := strong.PollOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := fallback.PollOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if queue.postCount() != 1 {
		t.Fatalf("strong-machine scenario posts = %d", queue.postCount())
	}

	queue.reset(true, true)
	fallback = newService(5 * time.Millisecond)
	if err := fallback.PollOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := fallback.PollOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if queue.postCount() != 1 {
		t.Fatalf("fallback scenario posts = %d", queue.postCount())
	}

	queue.reset(true, false)
	if err := newService(0).PollOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := newService(0).PollOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if queue.postCount() != 2 || queue.unexpectedMutations != 0 {
		t.Fatalf("redundant scenario posts = %d, unexpected mutations = %d", queue.postCount(), queue.unexpectedMutations)
	}
}

type sharedQueue struct {
	mu                  sync.Mutex
	queued              bool
	assignOnPost        bool
	posts               int
	unexpectedMutations int
}

func (queue *sharedQueue) serveHTTP(w http.ResponseWriter, r *http.Request) {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	queued := queue.queued
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/TKlerx/repo/actions/runs" && r.URL.Query().Get("status") == "queued":
		if queued {
			_, _ = w.Write([]byte(`{"workflow_runs":[{"id":5,"status":"queued"}]}`))
		} else {
			_, _ = w.Write([]byte(`{"workflow_runs":[]}`))
		}
	case r.Method == http.MethodGet && r.URL.Path == "/repos/TKlerx/repo/actions/runs":
		_, _ = w.Write([]byte(`{"workflow_runs":[]}`))
	case r.Method == http.MethodGet && r.URL.Path == "/repos/TKlerx/repo/actions/runs/5/jobs":
		_, _ = w.Write([]byte(`{"jobs":[{"id":11,"run_id":5,"status":"queued","labels":["self-hosted","Windows","X64"]}]}`))
	case r.Method == http.MethodGet && r.URL.Path == "/repos/TKlerx/repo/actions/jobs/11":
		status := "completed"
		if queued {
			status = "queued"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 11, "run_id": 5, "status": status, "labels": []string{"self-hosted", "Windows", "X64"}})
	case r.Method == http.MethodGet && r.URL.Path == "/repos/TKlerx/repo":
		_, _ = w.Write([]byte(`{"private":true}`))
	case r.Method == http.MethodPost && r.URL.Path == "/repos/TKlerx/repo/actions/runners/generate-jitconfig":
		queue.posts++
		if queue.assignOnPost {
			queue.queued = false
		}
		_, _ = w.Write([]byte(`{"runner":{"id":42,"name":"test"},"encoded_jit_config":"secret"}`))
	case r.Method != http.MethodGet:
		queue.unexpectedMutations++
		http.NotFound(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (queue *sharedQueue) reset(queued, assignOnPost bool) {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	queue.queued, queue.assignOnPost, queue.posts, queue.unexpectedMutations = queued, assignOnPost, 0, 0
}

func (queue *sharedQueue) postCount() int {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	return queue.posts
}

type fakeRunnerManager struct {
	requests chan runner.LaunchRequest
}

func (*fakeRunnerManager) Active() int                     { return 0 }
func (*fakeRunnerManager) Reconcile(context.Context) error { return nil }
func (manager *fakeRunnerManager) Run(_ context.Context, request runner.LaunchRequest, _ <-chan struct{}) error {
	manager.requests <- request
	return nil
}
