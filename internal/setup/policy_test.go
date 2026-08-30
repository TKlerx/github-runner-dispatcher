package setup

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TKlerx/github-runner-dispatcher/internal/config"
)

func TestMutatePolicyAddReconcileAndRemove(t *testing.T) {
	root := t.TempDir()
	configPath := writeConfig(t, root)
	policyPath := writePolicy(t, root, "new", "private", ".github/workflows/one.yml")

	if err := MutatePolicy(configPath, policyPath, "add"); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(configPath)
	if err != nil || len(cfg.Repositories) != 2 || cfg.Repositories[1].Name != "new" {
		t.Fatalf("added config = %#v, %v", cfg.Repositories, err)
	}
	if err := MutatePolicy(configPath, policyPath, "add"); err == nil {
		t.Fatal("duplicate add succeeded")
	}

	policyPath = writePolicy(t, root, "new", "public", ".github/workflows/two.yml")
	if err := MutatePolicy(configPath, policyPath, "reconcile"); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(configPath)
	if err := MutatePolicy(configPath, policyPath, "reconcile"); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(configPath)
	if !bytes.Equal(first, second) {
		t.Fatal("repeated reconciliation changed configuration bytes")
	}
	cfg, err = config.Load(configPath)
	if err != nil || cfg.Repositories[1].Visibility != "public" || cfg.Repositories[1].TrustedWorkflows[0].WorkflowPath != ".github/workflows/two.yml" {
		t.Fatalf("reconciled config = %#v, %v", cfg.Repositories, err)
	}

	if err := MutatePolicy(configPath, policyPath, "remove"); err != nil {
		t.Fatal(err)
	}
	cfg, err = config.Load(configPath)
	if err != nil || len(cfg.Repositories) != 1 || cfg.Repositories[0].Name != "old" {
		t.Fatalf("removed config = %#v, %v", cfg.Repositories, err)
	}
}

func TestMutatePolicyFailureLeavesConfigurationUnchanged(t *testing.T) {
	root := t.TempDir()
	configPath := writeConfig(t, root)
	policyPath := filepath.Join(root, "invalid.yml")
	if err := os.WriteFile(policyPath, []byte("repository:\n  owner: TKlerx\n  name: bad\n  visibility: public\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(configPath)
	if err := MutatePolicy(configPath, policyPath, "reconcile"); err == nil || !strings.Contains(err.Error(), "public repositories require") {
		t.Fatalf("MutatePolicy() error = %v", err)
	}
	after, _ := os.ReadFile(configPath)
	if !bytes.Equal(before, after) {
		t.Fatal("configuration changed after rejected mutation")
	}
}

func TestMutatePolicyRejectsUnknownFieldsAndTrailingDocuments(t *testing.T) {
	root := t.TempDir()
	configPath := writeConfig(t, root)
	for name, content := range map[string]string{
		"unknown":  "repository:\n  owner: TKlerx\n  name: new\n  payload_actor: trusted\n",
		"trailing": "repository:\n  owner: TKlerx\n  name: new\n---\nrepository:\n  owner: TKlerx\n  name: other\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(root, name+".yml")
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := MutatePolicy(configPath, path, "reconcile"); err == nil {
				t.Fatal("invalid policy succeeded")
			}
		})
	}
}

func writePolicy(t *testing.T, root, name, visibility, workflowPath string) string {
	t.Helper()
	path := filepath.Join(root, "policy.yml")
	content := `repository:
  owner: TKlerx
  name: ` + name + `
  visibility: ` + visibility + `
  runner_group_id: 1
  trusted_workflows:
    - workflow_path: ` + workflowPath + `
      rules:
        - events: [pull_request]
          actors: ["*"]
          required_labels: [self-hosted]
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
