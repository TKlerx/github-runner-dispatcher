package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAppliesDefaults(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg, err := Parse([]byte(validYAML(t, root, "")))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if cfg.PollInterval.String() != "10s" || cfg.ClaimDelay != 0 || cfg.AcquisitionTimeout.String() != "1m30s" {
		t.Fatalf("unexpected durations: poll=%s claim=%s timeout=%s", cfg.PollInterval, cfg.ClaimDelay, cfg.AcquisitionTimeout)
	}
	if cfg.Capacity != 1 || cfg.Repositories[0].RunnerGroupID != 1 {
		t.Fatalf("unexpected defaults: capacity=%d runner_group_id=%d", cfg.Capacity, cfg.Repositories[0].RunnerGroupID)
	}
	if cfg.Repositories[0].Visibility != "private" || len(cfg.Repositories[0].TrustedWorkflows) != 0 {
		t.Fatalf("legacy repository defaults = %#v", cfg.Repositories[0])
	}
	if cfg.GitHubAPIURL != "https://api.github.com" || cfg.GitHubAPIVersion != "2026-03-10" {
		t.Fatalf("unexpected GitHub defaults: %q %q", cfg.GitHubAPIURL, cfg.GitHubAPIVersion)
	}
}

func TestParseRejectsUnknownAndTrailingDocuments(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for name, suffix := range map[string]string{
		"unknown field":     "\nsecret_token: nope\n",
		"trailing document": "\n---\nparticipant_name: second\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(validYAML(t, root, suffix)))
			if err == nil {
				t.Fatal("Parse() unexpectedly succeeded")
			}
		})
	}
}

func TestParseAggregatesValidationErrors(t *testing.T) {
	t.Parallel()

	root := filepath.ToSlash(t.TempDir())
	_, err := Parse([]byte(`
participant_name: ""
repositories:
  - owner: TKlerx
    name: repo
  - owner: Other
    name: repo
labels: [self-hosted, WrongOS, WrongArch]
poll_interval: nope
claim_delay: -1s
acquisition_timeout: 0s
capacity: 9
token_file: relative-token
runner_template_dir: ` + root + `/template
state_dir: ` + root + `/state
`))
	if err == nil {
		t.Fatal("Parse() unexpectedly succeeded")
	}

	message := err.Error()
	for _, want := range []string{
		"participant_name",
		"repositories must share one owner",
		"labels",
		"poll_interval",
		"claim_delay",
		"acquisition_timeout",
		"capacity",
		"token_file",
	} {
		if !strings.Contains(message, want) {
			t.Errorf("error %q does not contain %q", message, want)
		}
	}
}

func TestParseRejectsDuplicateRepository(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	yaml := strings.Replace(validYAML(t, root, ""), "labels:", "  - owner: tklerx\n    name: REPO\nlabels:", 1)
	_, err := Parse([]byte(yaml))
	if err == nil || !strings.Contains(err.Error(), "duplicate repository") {
		t.Fatalf("Parse() error = %v, want duplicate repository", err)
	}
}

func TestParseAcceptsTrustedWorkflowRules(t *testing.T) {
	t.Parallel()

	cfg, err := Parse([]byte(trustedPolicyYAML(t, t.TempDir(), "public")))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	workflows := cfg.Repositories[0].TrustedWorkflows
	if cfg.Repositories[0].Visibility != "public" || len(workflows) != 2 || len(workflows[0].Rules) != 2 {
		t.Fatalf("repository policy = %#v", cfg.Repositories[0])
	}
}

func TestParseRejectsUnsafeTrustedWorkflowPolicies(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	base := trustedPolicyYAML(t, root, "public")
	cases := map[string]struct {
		old  string
		new  string
		want string
	}{
		"both workflow identities": {"      - workflow_id: 123\n", "      - workflow_id: 123\n        workflow_path: .github/workflows/review.yml\n", "exactly one"},
		"escaping path":            {"workflow_path: .github/workflows/issues.yml", "workflow_path: .github/workflows/../issues.yml", "exact .github/workflows"},
		"mixed wildcard":           {"actors: [\"*\"]", "actors: [\"*\", trusted]", "wildcard must be the only"},
		"unadvertised label":       {"required_labels: [dedicated]", "required_labels: [other]", "not advertised"},
		"duplicate workflow":       {"      - workflow_path: .github/workflows/issues.yml", "      - workflow_path: .github/workflows/issues.yml\n        rules:\n          - events: [issues]\n            actors: [trusted]\n            required_labels: [dedicated]\n      - workflow_path: .github/workflows/issues.yml", "duplicates trusted workflow"},
		"unknown nested field":     {"actors: [trusted]", "actors: [trusted]\n            payload_actor: trusted", "field payload_actor not found"},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			yaml := strings.Replace(base, test.old, test.new, 1)
			_, err := Parse([]byte(yaml))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Parse() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestParseRejectsPublicRepositoryWithoutPolicy(t *testing.T) {
	t.Parallel()

	yaml := strings.Replace(validYAML(t, t.TempDir(), ""), "    name: repo", "    name: repo\n    visibility: public", 1)
	_, err := Parse([]byte(yaml))
	if err == nil || !strings.Contains(err.Error(), "public repositories require") {
		t.Fatalf("Parse() error = %v, want public policy requirement", err)
	}
}

func TestParseRejectsExplicitZeroValues(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	yaml := strings.Replace(validYAML(t, root, ""), "name: repo", "name: repo\n    runner_group_id: 0", 1)
	yaml += "capacity: 0\n"
	_, err := Parse([]byte(yaml))
	if err == nil || !strings.Contains(err.Error(), "runner_group_id") || !strings.Contains(err.Error(), "capacity") {
		t.Fatalf("Parse() error = %v, want explicit zero errors", err)
	}
}

func TestParseRejectsOutOfRangeDurations(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for field, value := range map[string]string{
		"poll_interval":       "4s",
		"claim_delay":         "31m",
		"acquisition_timeout": "29s",
	} {
		t.Run(field, func(t *testing.T) {
			yaml := validYAML(t, root, "") + field + ": " + value + "\n"
			_, err := Parse([]byte(yaml))
			if err == nil || !strings.Contains(err.Error(), field) {
				t.Fatalf("Parse() error = %v, want %s range error", err, field)
			}
		})
	}
}

func TestParseRejectsMismatchedPlatformLabels(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	yaml := validYAML(t, root, "")
	osLabel, _, err := hostLabels()
	if err != nil {
		t.Fatal(err)
	}
	wrongOS := "Windows"
	if osLabel == wrongOS {
		wrongOS = "Linux"
	}
	yaml = strings.Replace(yaml, osLabel, wrongOS, 1)

	_, err = Parse([]byte(yaml))
	if err == nil || !strings.Contains(err.Error(), "actual operating-system label") {
		t.Fatalf("Parse() error = %v, want operating-system mismatch", err)
	}
}

func TestParseRejectsLinkedPathAncestry(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatal(err)
	}
	linkedDir := filepath.Join(root, "linked")
	if err := os.Symlink(realDir, linkedDir); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}

	yaml := validYAML(t, root, "")
	yaml = strings.Replace(yaml, filepath.ToSlash(filepath.Join(root, "state")), filepath.ToSlash(filepath.Join(linkedDir, "state")), 1)
	_, err := Parse([]byte(yaml))
	if err == nil || !strings.Contains(err.Error(), "state_dir") || !strings.Contains(err.Error(), "symbolic link or reparse point") {
		t.Fatalf("Parse() error = %v, want unsafe state path", err)
	}
}

func TestLoadToken(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "token")
	if err := os.WriteFile(path, []byte("github_pat_example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	token, err := LoadToken(path)
	if err != nil || token != "github_pat_example" {
		t.Fatalf("LoadToken() = %q, %v", token, err)
	}

	if err := os.WriteFile(path, []byte("bad token"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadToken(path); err == nil {
		t.Fatal("LoadToken() accepted whitespace")
	}
}

func validYAML(t *testing.T, root, suffix string) string {
	t.Helper()
	osLabel, archLabel, err := hostLabels()
	if err != nil {
		t.Fatal(err)
	}
	root = filepath.ToSlash(root)
	return `
participant_name: test-participant
repositories:
  - owner: TKlerx
    name: repo
labels: [self-hosted, ` + osLabel + `, ` + archLabel + `]
token_file: ` + root + `/token
runner_template_dir: ` + root + `/template
state_dir: ` + root + `/state
` + suffix
}

func trustedPolicyYAML(t *testing.T, root, visibility string) string {
	t.Helper()
	osLabel, archLabel, err := hostLabels()
	if err != nil {
		t.Fatal(err)
	}
	root = filepath.ToSlash(root)
	return `
participant_name: test-participant
repositories:
  - owner: TKlerx
    name: repo
    visibility: ` + visibility + `
    trusted_workflows:
      - workflow_id: 123
        rules:
          - events: [pull_request]
            actors: ["*"]
            required_labels: [dedicated]
          - events: [repository_dispatch]
            actors: [trusted]
            required_labels: [dedicated]
      - workflow_path: .github/workflows/issues.yml
        rules:
          - events: [issues, issue_comment]
            actors: [trusted]
            required_labels: [dedicated]
labels: [self-hosted, ` + osLabel + `, ` + archLabel + `, dedicated]
token_file: ` + root + `/token
runner_template_dir: ` + root + `/template
state_dir: ` + root + `/state
`
}
