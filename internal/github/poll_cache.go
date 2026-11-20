package github

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
)

type cachedRuns struct {
	etag, link string
	body       []byte
}

// Only workflow-list polls use conditional requests. Final job/run authorization
// rechecks and all mutations still go straight to GitHub without a cache.
type workflowPollCache struct {
	base    http.RoundTripper
	mu      sync.Mutex
	entries map[string]cachedRuns
}

func (cache *workflowPollCache) RoundTrip(request *http.Request) (*http.Response, error) {
	status := request.URL.Query().Get("status")
	if request.Method != http.MethodGet || !strings.HasSuffix(request.URL.Path, "/actions/runs") || (status != "queued" && status != "in_progress") {
		return cache.base.RoundTrip(request)
	}
	key := request.URL.String()
	cache.mu.Lock()
	previous := cache.entries[key]
	cache.mu.Unlock()
	request = request.Clone(request.Context())
	if previous.etag != "" {
		request.Header.Set("If-None-Match", previous.etag)
	}
	response, err := cache.base.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode == http.StatusNotModified && previous.etag != "" {
		response.Body.Close()
		response.StatusCode, response.Status = http.StatusOK, "200 OK"
		response.Body = io.NopCloser(bytes.NewReader(previous.body))
		response.ContentLength = int64(len(previous.body))
		if response.Header.Get("Link") == "" {
			response.Header.Set("Link", previous.link)
		}
		return response, nil
	}
	if response.StatusCode != http.StatusOK {
		return response, nil
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseSize+1))
	response.Body.Close()
	if err != nil {
		return nil, err
	}
	response.Body = io.NopCloser(bytes.NewReader(data))
	cache.mu.Lock()
	defer cache.mu.Unlock()
	delete(cache.entries, key)
	if etag := response.Header.Get("ETag"); etag != "" && len(data) <= 1<<20 && json.Valid(data) {
		// ponytail: bounded per-client cache; eviction only costs a fresh response.
		if len(cache.entries) >= 128 {
			for evicted := range cache.entries {
				delete(cache.entries, evicted)
				break
			}
		}
		cache.entries[key] = cachedRuns{etag, response.Header.Get("Link"), data}
	}
	return response, nil
}
