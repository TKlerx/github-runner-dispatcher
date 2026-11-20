package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConditionalWorkflowPollsRevalidatePagesAndChangedMetadata(t *testing.T) {
	var version atomic.Int32
	version.Store(1)
	var calls atomic.Int32
	client, server := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertHeaders(t, r)
		calls.Add(1)
		page := 1
		if r.URL.Query().Get("page") == "2" {
			page = 2
		}
		etag := fmt.Sprintf(`W/"%d-%d"`, version.Load(), page)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified) // Pagination must survive absent Link.
			return
		}
		w.Header().Set("ETag", etag)
		if page == 1 {
			w.Header().Set("Link", fmt.Sprintf(`<http://%s%s?status=queued&per_page=100&page=2>; rel="next"`, r.Host, r.URL.Path))
		}
		fmt.Fprintf(w, `{"workflow_runs":[{"id":%d,"actor":{"login":"trusted"}}]}`, int(version.Load())*10+page)
	}))
	defer server.Close()
	for _, generation := range []int32{1, 1, 2} {
		version.Store(generation)
		runs, err := client.ListWorkflowRuns(context.Background(), Repository{"TKlerx", "repo"}, "queued")
		if err != nil || len(runs) != 2 || runs[0].ID != int64(generation*10+1) || runs[1].ID != int64(generation*10+2) || runs[0].Actor.Login != "trusted" {
			t.Fatalf("poll generation %d = %#v, %v", generation, runs, err)
		}
		runs[0].Actor.Login = "caller mutation must not change the cache"
	}
	if calls.Load() != 6 {
		t.Fatalf("each page must be revalidated on every poll: calls=%d", calls.Load())
	}
}

func TestPollingCacheDoesNotHidePermissionErrorsOrCacheFinalRechecks(t *testing.T) {
	var lists atomic.Int32
	client, server := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/actions/runs") {
			if lists.Add(1) == 1 {
				w.Header().Set("ETag", `"initial"`)
				fmt.Fprint(w, `{"workflow_runs":[{"id":7}]}`)
				return
			}
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message":"do-not-log-this-body"}`)
			return
		}
		if r.Header.Get("If-None-Match") != "" {
			t.Error("final job/run recheck received a cache validator")
		}
		fmt.Fprint(w, `{"id":7,"status":"completed","actor":{"login":"changed"}}`)
	}))
	defer server.Close()
	repo := Repository{"TKlerx", "repo"}
	if _, err := client.ListWorkflowRuns(context.Background(), repo, "queued"); err != nil {
		t.Fatal(err)
	}
	_, err := client.ListWorkflowRuns(context.Background(), repo, "queued")
	var apiError *APIError
	if !errors.As(err, &apiError) || apiError.StatusCode != 403 || strings.Contains(err.Error(), "do-not-log") {
		t.Fatalf("permission failure must not replay cached runs or log body: %v", err)
	}
	for range 2 {
		job, err := client.GetJob(context.Background(), repo, 7)
		if err != nil || job.Status != "completed" {
			t.Fatalf("fresh job = %#v, %v", job, err)
		}
		run, err := client.GetWorkflowRun(context.Background(), repo, 7)
		if err != nil || run.Actor.Login != "changed" {
			t.Fatalf("fresh run = %#v, %v", run, err)
		}
	}
}

func TestUnsolicitedNotModifiedFailsClosed(t *testing.T) {
	client, server := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotModified) }))
	defer server.Close()
	if _, err := client.ListWorkflowRuns(context.Background(), Repository{"TKlerx", "repo"}, "queued"); err == nil {
		t.Fatal("accepted 304 without a previously validated response")
	}
}

func TestConcurrentPollCacheIsBoundedAndSeparatesQueries(t *testing.T) {
	client, server := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		etag := `"` + r.URL.String() + `"`
		if validator := r.Header.Get("If-None-Match"); validator != "" {
			if validator != etag {
				t.Error("validator crossed a repository or status boundary")
			}
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		fmt.Fprint(w, `{"workflow_runs":[]}`)
	}))
	defer server.Close()
	var workers sync.WaitGroup
	for worker := range 4 {
		workers.Go(func() {
			for i := range 40 {
				for _, status := range []string{"queued", "in_progress"} {
					if _, err := client.ListWorkflowRuns(context.Background(), Repository{"owner", fmt.Sprintf("repo-%d-%d", worker, i)}, status); err != nil {
						t.Error(err)
					}
				}
			}
		})
	}
	workers.Wait()
	cache := client.httpClient.Transport.(*workflowPollCache)
	if len(cache.entries) > 128 {
		t.Fatalf("unbounded cache: %d entries", len(cache.entries))
	}
}

func TestPrimaryQuotaFailureReturnsResetWithoutEarlyRetry(t *testing.T) {
	var calls atomic.Int32
	reset := time.Now().Add(time.Hour).Unix()
	client, server := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", fmt.Sprint(reset))
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	_, err := client.GetRepository(context.Background(), Repository{"TKlerx", "repo"})
	var apiError *APIError
	if !errors.As(err, &apiError) || apiError.RateLimitReset.Unix() != reset || calls.Load() != 1 {
		t.Fatalf("quota failure = %v, calls=%d", err, calls.Load())
	}
	// Assignment monitoring and mutations share the same credential budget.
	_, err = client.GetJob(context.Background(), Repository{"TKlerx", "repo"}, 7)
	if !errors.As(err, &apiError) || apiError.RateLimitReset.Unix() != reset || calls.Load() != 1 {
		t.Fatalf("another caller retried before reset: %v, calls=%d", err, calls.Load())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.GetJob(ctx, Repository{"TKlerx", "repo"}, 7); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation while throttled = %v", err)
	}
}

func TestLargeWorkflowResponsesAreNotRetained(t *testing.T) {
	client, server := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != "" {
			t.Error("oversized workflow response was retained")
		}
		w.Header().Set("ETag", `"large"`)
		fmt.Fprintf(w, `{"workflow_runs":[],"ignored":%q}`, strings.Repeat("x", 1<<20))
	}))
	defer server.Close()
	for range 2 {
		if _, err := client.ListWorkflowRuns(context.Background(), Repository{"owner", "repo"}, "queued"); err != nil {
			t.Fatal(err)
		}
	}
}
