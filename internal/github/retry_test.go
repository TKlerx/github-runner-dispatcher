package github

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientRetriesTransientServerAndRateLimitResponses(t *testing.T) {
	for name, status := range map[string]int{"server": http.StatusServiceUnavailable, "rate-limit": http.StatusForbidden} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			client, server := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if calls.Add(1) == 1 {
					if status == http.StatusForbidden {
						w.Header().Set("Retry-After", "0")
					}
					w.WriteHeader(status)
					return
				}
				_, _ = w.Write([]byte(`{"full_name":"TKlerx/repo","private":true}`))
			}))
			defer server.Close()
			if err := client.ValidatePrivateRepository(context.Background(), Repository{Owner: "TKlerx", Name: "repo"}); err != nil {
				t.Fatal(err)
			}
			if calls.Load() != 2 {
				t.Fatalf("calls = %d", calls.Load())
			}
		})
	}
}

func TestClientRetriesTransientNetworkFailure(t *testing.T) {
	var calls atomic.Int32
	transport := roundTripperFunc(func(*http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			return nil, errors.New("temporary network failure")
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: http.NoBody}, nil
	})
	client, err := NewClient("https://api.github.test", "2026-03-10", "secret", &http.Client{Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	err = client.ValidatePrivateRepository(context.Background(), Repository{Owner: "TKlerx", Name: "repo"})
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("expected retry followed by safe decode error, got %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d", calls.Load())
	}
}

func TestClientRetryWaitHonorsCancellation(t *testing.T) {
	client, server := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := client.ValidatePrivateRepository(ctx, Repository{Owner: "TKlerx", Name: "repo"})
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > time.Second {
		t.Fatalf("error = %v, elapsed = %s", err, time.Since(started))
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (function roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
