package setup

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/TKlerx/github-runner-dispatcher/internal/config"
	"go.yaml.in/yaml/v4"
)

type Executor interface {
	Run(context.Context, ...string) ([]byte, error)
}

var ErrGitHubCLI = errors.New("GitHub CLI failure")

type CLIExecutor struct{}

func (CLIExecutor) Run(ctx context.Context, args ...string) ([]byte, error) {
	output, err := exec.CommandContext(ctx, "gh", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("%w: gh command failed", ErrGitHubCLI)
	}
	return output, nil
}

type repository struct {
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Private  bool   `json:"private"`
	Archived bool   `json:"archived"`
	Status   string
}

func Run(ctx context.Context, configPath string, input io.Reader, output io.Writer, executor Executor) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "Configuration %s already exists. [c]ancel (default), [k]eep allowlist, or [r]eplace allowlist: ", configPath)
	scanner := bufio.NewScanner(input)
	if !scanner.Scan() {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(scanner.Text())) {
	case "", "c", "cancel":
		return nil
	case "k", "keep":
		repositories := make([]repository, len(cfg.Repositories))
		for i, item := range cfg.Repositories {
			repositories[i] = repository{Name: item.Name, FullName: item.Owner + "/" + item.Name, Private: true}
		}
		markWorkflowStatus(ctx, repositories, executor)
		printRepositories(output, repositories)
		printPATInstructions(output, cfg.ParticipantName, repositories)
		return nil
	case "r", "replace":
	default:
		return errors.New("invalid setup choice")
	}

	repositories, err := discoverRepositories(ctx, executor)
	if err != nil {
		return err
	}
	if len(repositories) == 0 {
		return errors.New("no owned private repositories found")
	}
	markWorkflowStatus(ctx, repositories, executor)
	printRepositories(output, repositories)
	fmt.Fprint(output, "Select repositories (for example 1,3-5): ")
	if !scanner.Scan() {
		return errors.New("repository selection is required")
	}
	indices, err := parseSelection(scanner.Text(), len(repositories))
	if err != nil {
		return err
	}
	selected := make([]repository, len(indices))
	for i, index := range indices {
		selected[i] = repositories[index]
	}
	if err := commonOwner(selected); err != nil {
		return err
	}
	fmt.Fprint(output, "Replace only the repository allowlist? [y/N]: ")
	if !scanner.Scan() || !strings.EqualFold(strings.TrimSpace(scanner.Text()), "y") {
		return nil
	}
	if err := replaceAllowlist(configPath, selected); err != nil {
		return err
	}
	printPATInstructions(output, cfg.ParticipantName, selected)
	return nil
}

func discoverRepositories(ctx context.Context, executor Executor) ([]repository, error) {
	data, err := executor.Run(ctx, "api", "--paginate", "--slurp", "/user/repos?visibility=private&affiliation=owner&per_page=100")
	if err != nil {
		return nil, fmt.Errorf("list private repositories: %w", err)
	}
	var pages [][]repository
	if err := json.Unmarshal(data, &pages); err != nil {
		return nil, fmt.Errorf("decode repository list: %w", err)
	}
	var repositories []repository
	seen := map[string]bool{}
	for _, page := range pages {
		for _, item := range page {
			parts := strings.Split(item.FullName, "/")
			if !item.Private || len(parts) != 2 || parts[0] == "" || parts[1] == "" || item.Name != parts[1] || strings.ContainsAny(parts[0]+parts[1], "\\?#") {
				return nil, errors.New("GitHub CLI returned an invalid repository")
			}
			key := strings.ToLower(item.FullName)
			if !seen[key] {
				seen[key] = true
				repositories = append(repositories, item)
			}
		}
	}
	sort.Slice(repositories, func(i, j int) bool {
		return strings.ToLower(repositories[i].FullName) < strings.ToLower(repositories[j].FullName)
	})
	return repositories, nil
}

func markWorkflowStatus(ctx context.Context, repositories []repository, executor Executor) {
	for i := range repositories {
		parts := strings.SplitN(repositories[i].FullName, "/", 2)
		if len(parts) != 2 {
			repositories[i].Status = "status unknown"
			continue
		}
		data, err := executor.Run(ctx, "api", fmt.Sprintf("/repos/%s/%s/actions/workflows?per_page=100", parts[0], parts[1]))
		if err != nil {
			repositories[i].Status = "status unknown"
			continue
		}
		var response struct {
			Workflows []struct {
				State string `json:"state"`
			} `json:"workflows"`
		}
		if json.Unmarshal(data, &response) != nil {
			repositories[i].Status = "status unknown"
			continue
		}
		repositories[i].Status = "no active workflows"
		for _, workflow := range response.Workflows {
			if workflow.State == "active" {
				repositories[i].Status = "active workflows"
				break
			}
		}
	}
}

func printRepositories(output io.Writer, repositories []repository) {
	for i, item := range repositories {
		status := item.Status
		if item.Archived {
			status = "archived; " + status
		}
		fmt.Fprintf(output, "%d. %s (%s)\n", i+1, item.FullName, status)
	}
}

func parseSelection(value string, count int) ([]int, error) {
	selected := map[int]bool{}
	for _, part := range strings.Split(strings.TrimSpace(value), ",") {
		bounds := strings.Split(strings.TrimSpace(part), "-")
		if len(bounds) > 2 || bounds[0] == "" {
			return nil, errors.New("invalid repository selection")
		}
		first, err := strconv.Atoi(bounds[0])
		if err != nil {
			return nil, errors.New("invalid repository selection")
		}
		last := first
		if len(bounds) == 2 {
			last, err = strconv.Atoi(bounds[1])
			if err != nil {
				return nil, errors.New("invalid repository selection")
			}
		}
		if first < 1 || last < first || last > count {
			return nil, errors.New("repository selection is out of range")
		}
		for index := first; index <= last; index++ {
			selected[index-1] = true
		}
	}
	if len(selected) == 0 {
		return nil, errors.New("select at least one repository")
	}
	indices := make([]int, 0, len(selected))
	for index := range selected {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	return indices, nil
}

func commonOwner(repositories []repository) error {
	if len(repositories) == 0 {
		return errors.New("select at least one repository")
	}
	owner := strings.SplitN(repositories[0].FullName, "/", 2)[0]
	for _, item := range repositories[1:] {
		if !strings.EqualFold(owner, strings.SplitN(item.FullName, "/", 2)[0]) {
			return errors.New("selected repositories must have the same owner")
		}
	}
	return nil
}

func replaceAllowlist(path string, repositories []repository) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return err
	}
	root := document.Content[0]
	for i := 0; i < len(root.Content); i += 2 {
		if root.Content[i].Value != "repositories" {
			continue
		}
		value := &yaml.Node{Kind: yaml.SequenceNode}
		for _, item := range repositories {
			parts := strings.SplitN(item.FullName, "/", 2)
			entry := &yaml.Node{Kind: yaml.MappingNode}
			entry.Content = append(entry.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: "owner"}, &yaml.Node{Kind: yaml.ScalarNode, Value: parts[0]},
				&yaml.Node{Kind: yaml.ScalarNode, Value: "name"}, &yaml.Node{Kind: yaml.ScalarNode, Value: parts[1]},
				&yaml.Node{Kind: yaml.ScalarNode, Value: "runner_group_id"}, &yaml.Node{Kind: yaml.ScalarNode, Value: "1", Tag: "!!int"},
			)
			value.Content = append(value.Content, entry)
		}
		root.Content[i+1] = value
		return atomicWriteYAML(path, &document)
	}
	return errors.New("configuration has no repositories field")
}

func atomicWriteYAML(path string, document *yaml.Node) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".runner-participant-*.tmp")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	encoder := yaml.NewEncoder(temporary)
	encoder.SetIndent(2)
	if err := encoder.Encode(document); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Chmod(info.Mode()); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, path)
}

func printPATInstructions(output io.Writer, participantName string, repositories []repository) {
	owner := strings.SplitN(repositories[0].FullName, "/", 2)[0]
	name := "runner-" + participantName
	if len(name) > 40 {
		name = name[:40]
	}
	values := url.Values{
		"name":           {name},
		"description":    {"On-demand GitHub Actions runner participant"},
		"target_name":    {owner},
		"actions":        {"read"},
		"administration": {"write"},
		"metadata":       {"read"},
	}
	fmt.Fprintf(output, "Create a fine-grained PAT: https://github.com/settings/personal-access-tokens/new?%s\n", values.Encode())
	fmt.Fprintln(output, "GitHub requires manual repository selection. Select exactly:")
	for _, item := range repositories {
		fmt.Fprintf(output, "- %s\n", item.FullName)
	}
}
