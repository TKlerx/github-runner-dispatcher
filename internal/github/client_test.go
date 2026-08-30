package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestValidatePrivateRepository(t *testing.T) {
	t.Parallel()

	for name, private := range map[string]bool{"private": true, "public": false} {
		t.Run(name, func(t *testing.T) {
			client, server := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assertHeaders(t, r)
				if r.URL.Path != "/repos/TKlerx/repo" {
					t.Fatalf("path = %q", r.URL.Path)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"full_name": "TKlerx/repo", "private": private})
			}))
			defer server.Close()

			err := client.ValidatePrivateRepository(context.Background(), Repository{Owner: "TKlerx", Name: "repo"})
			if private && err != nil {
				t.Fatalf("ValidatePrivateRepository() error = %v", err)
			}
			if !private && !errors.Is(err, ErrPublicRepository) {
				t.Fatalf("ValidatePrivateRepository() error = %v, want ErrPublicRepository", err)
			}
		})
	}
}

func TestListWorkflowRunsPaginates(t *testing.T) {
	t.Parallel()

	var server *httptest.Server
	client, server := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertHeaders(t, r)
		if r.URL.Query().Get("status") != "queued" || r.URL.Query().Get("per_page") != "100" {
			t.Fatalf("query = %q", r.URL.RawQuery)
		}
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte(`{"workflow_runs":[{"id":2,"status":"queued"}]}`))
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s/repos/TKlerx/repo/actions/runs?status=queued&per_page=100&page=2>; rel="next"`, server.URL))
		_, _ = w.Write([]byte(`{"workflow_runs":[{"id":1,"status":"queued"}]}`))
	}))
	defer server.Close()

	runs, err := client.ListWorkflowRuns(context.Background(), Repository{Owner: "TKlerx", Name: "repo"}, "queued")
	if err != nil {
		t.Fatalf("ListWorkflowRuns() error = %v", err)
	}
	if len(runs) != 2 || runs[0].ID != 1 || runs[1].ID != 2 {
		t.Fatalf("runs = %#v", runs)
	}
}

func TestListJobsAndGetJob(t *testing.T) {
	t.Parallel()

	client, server := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertHeaders(t, r)
		switch r.URL.Path {
		case "/repos/TKlerx/repo/actions/runs/7/jobs":
			if r.URL.Query().Get("filter") != "latest" {
				t.Fatalf("filter = %q", r.URL.Query().Get("filter"))
			}
			_, _ = w.Write([]byte(`{"jobs":[{"id":11,"run_id":7,"name":"test","status":"queued","labels":["self-hosted","Linux","X64"]}]}`))
		case "/repos/TKlerx/repo/actions/jobs/11":
			_, _ = w.Write([]byte(`{"id":11,"run_id":7,"name":"test","status":"in_progress","labels":["self-hosted","Linux","X64"],"runner_id":9,"runner_name":"participant-abcd"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	jobs, err := client.ListJobs(context.Background(), Repository{Owner: "TKlerx", Name: "repo"}, 7)
	if err != nil || len(jobs) != 1 || len(jobs[0].Labels) != 3 {
		t.Fatalf("ListJobs() = %#v, %v", jobs, err)
	}
	job, err := client.GetJob(context.Background(), Repository{Owner: "TKlerx", Name: "repo"}, 11)
	if err != nil || job.RunnerID != 9 || job.RunnerName != "participant-abcd" {
		t.Fatalf("GetJob() = %#v, %v", job, err)
	}
}

func TestGenerateJITConfig(t *testing.T) {
	t.Parallel()

	client, server := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertHeaders(t, r)
		if r.Method != http.MethodPost || r.URL.Path != "/repos/TKlerx/repo/actions/runners/generate-jitconfig" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var request JITConfigRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Name != "participant-abcd" || request.RunnerGroupID != 1 || request.WorkFolder != "_work" || len(request.Labels) != 3 {
			t.Fatalf("request = %#v", request)
		}
		_, _ = w.Write([]byte(`{"runner":{"id":42,"name":"participant-abcd"},"encoded_jit_config":"secret-jit"}`))
	}))
	defer server.Close()

	result, err := client.GenerateJITConfig(context.Background(), Repository{Owner: "TKlerx", Name: "repo"}, JITConfigRequest{
		Name: "participant-abcd", RunnerGroupID: 1, Labels: []string{"self-hosted", "Linux", "X64"}, WorkFolder: "_work",
	})
	if err != nil || result.Runner.ID != 42 || result.EncodedJITConfig != "secret-jit" {
		t.Fatalf("GenerateJITConfig() = %#v, %v", result, err)
	}
}

func TestCheckAdministrationUsesReadOnlyEndpoint(t *testing.T) {
	t.Parallel()

	client, server := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertHeaders(t, r)
		if r.Method != http.MethodGet || r.URL.Path != "/repos/TKlerx/repo/actions/runners" || r.URL.Query().Get("per_page") != "1" {
			t.Fatalf("request = %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"total_count":0,"runners":[]}`))
	}))
	defer server.Close()

	if err := client.CheckAdministration(context.Background(), Repository{Owner: "TKlerx", Name: "repo"}); err != nil {
		t.Fatalf("CheckAdministration() error = %v", err)
	}
}

func TestDeleteRunner(t *testing.T) {
	t.Parallel()

	client, server := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertHeaders(t, r)
		if r.Method != http.MethodDelete || r.URL.Path != "/repos/TKlerx/repo/actions/runners/42" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	if err := client.DeleteRunner(context.Background(), Repository{Owner: "TKlerx", Name: "repo"}, 42); err != nil {
		t.Fatalf("DeleteRunner() error = %v", err)
	}
}

func TestAPIErrorDoesNotLeakTokenOrResponseBody(t *testing.T) {
	t.Parallel()

	client, server := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-GitHub-Request-Id", "safe-request-id")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"secret-token"}`))
	}))
	defer server.Close()

	err := client.ValidatePrivateRepository(context.Background(), Repository{Owner: "TKlerx", Name: "repo"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusForbidden {
		t.Fatalf("error = %#v", err)
	}
	if strings.Contains(err.Error(), "secret-token") || !strings.Contains(err.Error(), "safe-request-id") {
		t.Fatalf("unsafe or unhelpful error = %q", err)
	}
}

func TestPaginationRejectsAnotherOrigin(t *testing.T) {
	t.Parallel()

	var evilCalls atomic.Int32
	evil := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		evilCalls.Add(1)
	}))
	defer evil.Close()

	client, server := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Link", fmt.Sprintf(`<%s/steal>; rel="next"`, evil.URL))
		_, _ = w.Write([]byte(`{"workflow_runs":[]}`))
	}))
	defer server.Close()

	_, err := client.ListWorkflowRuns(context.Background(), Repository{Owner: "TKlerx", Name: "repo"}, "queued")
	if err == nil || !strings.Contains(err.Error(), "pagination URL") {
		t.Fatalf("ListWorkflowRuns() error = %v", err)
	}
	if evilCalls.Load() != 0 {
		t.Fatalf("cross-origin server received %d calls", evilCalls.Load())
	}
}

func TestClientDoesNotFollowRedirects(t *testing.T) {
	t.Parallel()

	var redirectedCalls atomic.Int32
	redirected := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirectedCalls.Add(1)
	}))
	defer redirected.Close()

	client, server := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", redirected.URL)
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()

	err := client.ValidatePrivateRepository(context.Background(), Repository{Owner: "TKlerx", Name: "repo"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusFound {
		t.Fatalf("error = %#v, want redirect APIError", err)
	}
	if redirectedCalls.Load() != 0 {
		t.Fatalf("redirect target received %d calls", redirectedCalls.Load())
	}
}

func testClient(t *testing.T, handler http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	client, err := NewClient(server.URL, "2026-03-10", "secret-token", server.Client())
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	return client, server
}

func assertHeaders(t *testing.T, r *http.Request) {
	t.Helper()
	if r.Header.Get("Authorization") != "Bearer secret-token" {
		t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
	}
	if r.Header.Get("Accept") != "application/vnd.github+json" || r.Header.Get("X-GitHub-Api-Version") != "2026-03-10" {
		t.Errorf("GitHub headers = %#v", r.Header)
	}
	if !strings.HasPrefix(r.Header.Get("User-Agent"), "github-runner-dispatcher/") {
		t.Errorf("User-Agent = %q", r.Header.Get("User-Agent"))
	}
}
