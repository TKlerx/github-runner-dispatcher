package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCheckIsSuccessfulAndSideEffectFree(t *testing.T) {
	var calls, writes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet {
			writes.Add(1)
		}
		switch {
		case r.URL.Path == "/repos/TKlerx/repo":
			_, _ = w.Write([]byte(`{"full_name":"TKlerx/repo","private":true}`))
		case r.URL.Path == "/repos/TKlerx/repo/actions/runs":
			_, _ = w.Write([]byte(`{"workflow_runs":[]}`))
		case r.URL.Path == "/repos/TKlerx/repo/actions/runners":
			_, _ = w.Write([]byte(`{"total_count":0,"runners":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	configPath := checkConfig(t, server.URL, true)
	var output, errorOutput bytes.Buffer

	exitCode := run(context.Background(), []string{"-config", configPath, "-check"}, strings.NewReader(""), &output, &errorOutput, nil)
	if exitCode != 0 {
		t.Fatalf("exit = %d, stderr = %s", exitCode, errorOutput.String())
	}
	if calls.Load() != 3 || writes.Load() != 0 {
		t.Fatalf("API calls = %d, mutating calls = %d", calls.Load(), writes.Load())
	}
	if !strings.Contains(output.String(), "valid") {
		t.Fatalf("output = %s", output.String())
	}
}

func TestCheckRejectsInvalidLocalConfigurationWithoutAPICalls(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	configPath := checkConfig(t, server.URL, false)
	var errorOutput bytes.Buffer

	exitCode := run(context.Background(), []string{"-config", configPath, "-check"}, strings.NewReader(""), &bytes.Buffer{}, &errorOutput, nil)
	if exitCode != 2 || calls.Load() != 0 {
		t.Fatalf("exit = %d, calls = %d, stderr = %s", exitCode, calls.Load(), errorOutput.String())
	}
}

func TestCheckReturnsGitHubExitCodeForPublicRepository(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"full_name":"TKlerx/repo","private":false}`))
	}))
	defer server.Close()
	configPath := checkConfig(t, server.URL, true)

	exitCode := run(context.Background(), []string{"-config", configPath, "-check"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, nil)
	if exitCode != 3 {
		t.Fatalf("exit = %d", exitCode)
	}
}

func TestCheckAcceptsPublicRepositoryWithPolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/TKlerx/repo":
			_, _ = w.Write([]byte(`{"full_name":"TKlerx/repo","private":false}`))
		case r.URL.Path == "/repos/TKlerx/repo/actions/runs":
			_, _ = w.Write([]byte(`{"workflow_runs":[]}`))
		case r.URL.Path == "/repos/TKlerx/repo/actions/runners":
			_, _ = w.Write([]byte(`{"total_count":0,"runners":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	configPath := checkConfig(t, server.URL, true)
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	content := strings.Replace(string(data), "    name: repo", `    name: repo
    visibility: public
    trusted_workflows:
      - workflow_path: .github/workflows/review.yml
        rules:
          - events: [pull_request]
            actors: ["*"]
            required_labels: [self-hosted]`, 1)
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	var output, errorOutput bytes.Buffer
	exitCode := run(context.Background(), []string{"-config", configPath, "-check"}, strings.NewReader(""), &output, &errorOutput, nil)
	if exitCode != 0 {
		t.Fatalf("exit = %d, stderr = %s", exitCode, errorOutput.String())
	}
}

func checkConfig(t *testing.T, apiURL string, createState bool) string {
	t.Helper()
	root := t.TempDir()
	template := filepath.Join(root, "template")
	state := filepath.Join(root, "state")
	if err := os.Mkdir(template, 0o700); err != nil {
		t.Fatal(err)
	}
	if createState {
		if err := os.Mkdir(state, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	runnerScript := "run.sh"
	osLabel := "Linux"
	if runtime.GOOS == "windows" {
		runnerScript = "run.cmd"
		osLabel = "Windows"
	}
	if err := os.WriteFile(filepath.Join(template, runnerScript), []byte("runner"), 0o700); err != nil {
		t.Fatal(err)
	}
	token := filepath.Join(root, "token")
	if err := os.WriteFile(token, []byte("test-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "config.yml")
	content := fmt.Sprintf(`participant_name: test
repositories:
  - owner: TKlerx
    name: repo
    runner_group_id: 1
labels: [self-hosted, %s, X64]
poll_interval: 10s
claim_delay: 0s
acquisition_timeout: 90s
capacity: 1
token_file: %s
runner_template_dir: %s
state_dir: %s
github_api_url: %s
github_api_version: "2026-03-10"
`, osLabel, filepath.ToSlash(token), filepath.ToSlash(template), filepath.ToSlash(state), apiURL)
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return configPath
}
