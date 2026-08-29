package setup

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/TKlerx/github-runner-dispatcher/internal/config"
)

func TestRunReplacesOnlySelectedRepositories(t *testing.T) {
	root := t.TempDir()
	configPath := writeConfig(t, root)
	executor := &fakeExecutor{run: func(args []string) ([]byte, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "/user/repos?"):
			return []byte(`[[{"name":"zeta","full_name":"TKlerx/zeta","private":true,"archived":true},{"name":"alpha","full_name":"TKlerx/alpha","private":true,"archived":false}],[{"name":"beta","full_name":"TKlerx/beta","private":true,"archived":false}]]`), nil
		case strings.Contains(joined, "/alpha/actions/workflows"):
			return []byte(`{"workflows":[{"state":"active"}]}`), nil
		case strings.Contains(joined, "/beta/actions/workflows"):
			return []byte(`{"workflows":[{"state":"disabled_manually"}]}`), nil
		case strings.Contains(joined, "/zeta/actions/workflows"):
			return nil, errors.New("lookup failed")
		default:
			return nil, errors.New("unexpected gh command")
		}
	}}
	var output bytes.Buffer

	err := Run(context.Background(), configPath, strings.NewReader("r\n1-3\ny\n"), &output, executor)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Repositories) != 3 || cfg.Repositories[0].Name != "alpha" || cfg.Repositories[1].Name != "beta" || cfg.Repositories[2].Name != "zeta" {
		t.Fatalf("repositories = %#v", cfg.Repositories)
	}
	if cfg.ParticipantName != "test-participant" || cfg.Capacity != 1 {
		t.Fatalf("non-repository configuration changed: %#v", cfg)
	}
	text := output.String()
	for _, want := range []string{"already exists", "no active workflows", "archived", "status unknown", "actions=read", "administration=write", "metadata=read", "TKlerx/alpha", "TKlerx/beta", "TKlerx/zeta"} {
		if !strings.Contains(text, want) {
			t.Errorf("output does not contain %q:\n%s", want, text)
		}
	}
	for _, call := range executor.calls {
		if strings.Contains(strings.Join(call, " "), "auth token") {
			t.Fatalf("setup attempted token access: %v", call)
		}
	}
}

func TestRunCancelLeavesExistingConfigUntouched(t *testing.T) {
	root := t.TempDir()
	configPath := writeConfig(t, root)
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	executor := &fakeExecutor{run: func([]string) ([]byte, error) {
		return nil, errors.New("must not run")
	}}
	var output bytes.Buffer

	if err := Run(context.Background(), configPath, strings.NewReader("\n"), &output, executor); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("configuration changed after cancellation")
	}
	if len(executor.calls) != 0 {
		t.Fatalf("gh calls after cancellation = %v", executor.calls)
	}
}

func TestRunKeepLeavesExistingConfigUntouched(t *testing.T) {
	root := t.TempDir()
	configPath := writeConfig(t, root)
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	executor := &fakeExecutor{run: func(args []string) ([]byte, error) {
		if strings.Contains(strings.Join(args, " "), "/old/actions/workflows") {
			return []byte(`{"workflows":[]}`), nil
		}
		return nil, errors.New("unexpected gh command")
	}}
	var output bytes.Buffer

	if err := Run(context.Background(), configPath, strings.NewReader("k\n"), &output, executor); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("configuration changed while keeping allowlist")
	}
	if !strings.Contains(output.String(), "TKlerx/old") || !strings.Contains(output.String(), "no active workflows") {
		t.Fatalf("output = %s", output.String())
	}
}

func TestParseSelection(t *testing.T) {
	selected, err := parseSelection("1,3-4,4", 5)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{0, 2, 3}
	if len(selected) != len(want) {
		t.Fatalf("selected = %v", selected)
	}
	for i := range want {
		if selected[i] != want[i] {
			t.Fatalf("selected = %v", selected)
		}
	}
	if _, err := parseSelection("0,6", 5); err == nil {
		t.Fatal("parseSelection accepted out-of-range values")
	}
}

func TestRunRejectsMixedOwnersBeforeWriting(t *testing.T) {
	root := t.TempDir()
	configPath := writeConfig(t, root)
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	executor := &fakeExecutor{run: func(args []string) ([]byte, error) {
		if strings.Contains(strings.Join(args, " "), "/user/repos?") {
			return []byte(`[[{"name":"one","full_name":"TKlerx/one","private":true},{"name":"two","full_name":"someone/two","private":true}]]`), nil
		}
		return []byte(`{"workflows":[]}`), nil
	}}

	err = Run(context.Background(), configPath, strings.NewReader("r\n1-2\ny\n"), &bytes.Buffer{}, executor)
	if err == nil || !strings.Contains(err.Error(), "same owner") {
		t.Fatalf("Run() error = %v", err)
	}
	after, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("configuration changed after mixed-owner rejection")
	}
}

type fakeExecutor struct {
	calls [][]string
	run   func([]string) ([]byte, error)
}

func (executor *fakeExecutor) Run(_ context.Context, args ...string) ([]byte, error) {
	executor.calls = append(executor.calls, append([]string(nil), args...))
	return executor.run(args)
}

func writeConfig(t *testing.T, root string) string {
	t.Helper()
	osLabel := "Linux"
	if runtime.GOOS == "windows" {
		osLabel = "Windows"
	}
	path := filepath.Join(root, "config.yml")
	content := `participant_name: test-participant
repositories:
  - owner: TKlerx
    name: old
    runner_group_id: 1
labels: [self-hosted, ` + osLabel + `, X64]
poll_interval: 10s
claim_delay: 0s
acquisition_timeout: 90s
capacity: 1
token_file: ` + filepath.ToSlash(filepath.Join(root, "missing-token")) + `
runner_template_dir: ` + filepath.ToSlash(filepath.Join(root, "template")) + `
state_dir: ` + filepath.ToSlash(filepath.Join(root, "state")) + `
github_api_url: https://api.github.com
github_api_version: "2026-03-10"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
