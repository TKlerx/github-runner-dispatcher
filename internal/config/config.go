package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v4"
)

const (
	defaultPollInterval       = 10 * time.Second
	defaultAcquisitionTimeout = 90 * time.Second
	defaultGitHubAPIURL       = "https://api.github.com"
	defaultGitHubAPIVersion   = "2026-03-10"
)

type Repository struct {
	Owner            string            `yaml:"owner"`
	Name             string            `yaml:"name"`
	Visibility       string            `yaml:"visibility,omitempty"`
	RunnerGroupID    int64             `yaml:"runner_group_id,omitempty"`
	TrustedWorkflows []TrustedWorkflow `yaml:"trusted_workflows,omitempty"`
}

type TrustedWorkflow struct {
	WorkflowID   int64               `yaml:"workflow_id,omitempty"`
	WorkflowPath string              `yaml:"workflow_path,omitempty"`
	Rules        []AuthorizationRule `yaml:"rules"`
}

type AuthorizationRule struct {
	Events         []string `yaml:"events"`
	Actors         []string `yaml:"actors"`
	RequiredLabels []string `yaml:"required_labels"`
}

type Config struct {
	ParticipantName    string
	Repositories       []Repository
	Labels             []string
	PollInterval       time.Duration
	ClaimDelay         time.Duration
	AcquisitionTimeout time.Duration
	Capacity           int
	TokenFile          string
	RunnerTemplateDir  string
	StateDir           string
	GitHubAPIURL       string
	GitHubAPIVersion   string
}

type rawConfig struct {
	ParticipantName    string          `yaml:"participant_name"`
	Repositories       []rawRepository `yaml:"repositories"`
	Labels             []string        `yaml:"labels"`
	PollInterval       string          `yaml:"poll_interval"`
	ClaimDelay         string          `yaml:"claim_delay"`
	AcquisitionTimeout string          `yaml:"acquisition_timeout"`
	Capacity           *int            `yaml:"capacity"`
	TokenFile          string          `yaml:"token_file"`
	RunnerTemplateDir  string          `yaml:"runner_template_dir"`
	StateDir           string          `yaml:"state_dir"`
	GitHubAPIURL       string          `yaml:"github_api_url"`
	GitHubAPIVersion   string          `yaml:"github_api_version"`
}

type rawRepository struct {
	Owner            string            `yaml:"owner"`
	Name             string            `yaml:"name"`
	Visibility       string            `yaml:"visibility"`
	RunnerGroupID    *int64            `yaml:"runner_group_id"`
	TrustedWorkflows []TrustedWorkflow `yaml:"trusted_workflows"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	return Parse(data)
}

func Parse(data []byte) (Config, error) {
	var raw rawConfig
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&raw); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}

	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err != nil {
			return Config{}, fmt.Errorf("decode trailing YAML document: %w", err)
		}
		return Config{}, errors.New("configuration must contain exactly one YAML document")
	}

	applyDefaults(&raw)
	problems := &validationErrors{}
	cfg := Config{
		ParticipantName:   strings.TrimSpace(raw.ParticipantName),
		Labels:            append([]string(nil), raw.Labels...),
		Capacity:          1,
		TokenFile:         strings.TrimSpace(raw.TokenFile),
		RunnerTemplateDir: strings.TrimSpace(raw.RunnerTemplateDir),
		StateDir:          strings.TrimSpace(raw.StateDir),
		GitHubAPIURL:      strings.TrimSpace(raw.GitHubAPIURL),
		GitHubAPIVersion:  strings.TrimSpace(raw.GitHubAPIVersion),
	}
	if raw.Capacity != nil {
		cfg.Capacity = *raw.Capacity
	}
	cfg.Repositories = make([]Repository, len(raw.Repositories))
	for i, repository := range raw.Repositories {
		cfg.Repositories[i] = Repository{
			Owner: repository.Owner, Name: repository.Name, Visibility: repository.Visibility,
			RunnerGroupID: 1, TrustedWorkflows: repository.TrustedWorkflows,
		}
		if cfg.Repositories[i].Visibility == "" {
			cfg.Repositories[i].Visibility = "private"
		}
		if repository.RunnerGroupID != nil {
			cfg.Repositories[i].RunnerGroupID = *repository.RunnerGroupID
		}
	}
	cfg.PollInterval = parseDuration("poll_interval", raw.PollInterval, problems)
	cfg.ClaimDelay = parseDuration("claim_delay", raw.ClaimDelay, problems)
	cfg.AcquisitionTimeout = parseDuration("acquisition_timeout", raw.AcquisitionTimeout, problems)
	validate(&cfg, problems)
	if err := problems.err(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func LoadToken(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read token_file: %w", err)
	}
	token := strings.TrimRight(string(data), "\r\n")
	if token == "" {
		return "", errors.New("token_file is empty")
	}
	if strings.ContainsAny(token, " \t\r\n") {
		return "", errors.New("token_file must contain only the token and an optional trailing newline")
	}
	return token, nil
}

func applyDefaults(raw *rawConfig) {
	if raw.PollInterval == "" {
		raw.PollInterval = defaultPollInterval.String()
	}
	if raw.ClaimDelay == "" {
		raw.ClaimDelay = "0s"
	}
	if raw.AcquisitionTimeout == "" {
		raw.AcquisitionTimeout = defaultAcquisitionTimeout.String()
	}
	if raw.GitHubAPIURL == "" {
		raw.GitHubAPIURL = defaultGitHubAPIURL
	}
	if raw.GitHubAPIVersion == "" {
		raw.GitHubAPIVersion = defaultGitHubAPIVersion
	}
}

func parseDuration(field, value string, problems *validationErrors) time.Duration {
	duration, err := time.ParseDuration(value)
	if err != nil {
		problems.add("%s must be a Go duration: %v", field, err)
		return 0
	}
	return duration
}

func validate(cfg *Config, problems *validationErrors) {
	if cfg.ParticipantName == "" {
		problems.add("participant_name is required")
	} else if len(cfg.ParticipantName) > 64 || strings.ContainsAny(cfg.ParticipantName, " \t\r\n/\\") {
		problems.add("participant_name must be at most 64 characters without whitespace or path separators")
	}
	validateLabels(cfg.Labels, problems)
	validateRepositories(cfg.Repositories, cfg.Labels, problems)
	if cfg.PollInterval < 5*time.Second || cfg.PollInterval > 5*time.Minute {
		problems.add("poll_interval must be between 5s and 5m")
	}
	if cfg.ClaimDelay < 0 || cfg.ClaimDelay > 30*time.Minute {
		problems.add("claim_delay must be between 0s and 30m")
	}
	if cfg.AcquisitionTimeout < 30*time.Second || cfg.AcquisitionTimeout > 30*time.Minute {
		problems.add("acquisition_timeout must be between 30s and 30m")
	}
	if cfg.Capacity < 1 || cfg.Capacity > 4 {
		problems.add("capacity must be between 1 and 4")
	}
	validateURL(cfg.GitHubAPIURL, problems)
	if !validAPIVersion(cfg.GitHubAPIVersion) {
		problems.add("github_api_version must use YYYY-MM-DD")
	}
	validatePaths(cfg, problems)
}

func validateRepositories(repositories []Repository, participantLabels []string, problems *validationErrors) {
	if len(repositories) == 0 {
		problems.add("repositories must contain at least one private repository")
		return
	}
	owner := strings.TrimSpace(repositories[0].Owner)
	seen := make(map[string]struct{}, len(repositories))
	for i := range repositories {
		repository := &repositories[i]
		repository.Owner = strings.TrimSpace(repository.Owner)
		repository.Name = strings.TrimSpace(repository.Name)
		repository.Visibility = strings.ToLower(strings.TrimSpace(repository.Visibility))
		if repository.Owner == "" || repository.Name == "" {
			problems.add("repositories[%d] owner and name are required", i)
		}
		if strings.ContainsAny(repository.Owner+repository.Name, "/\\?#") {
			problems.add("repositories[%d] owner and name must not contain path or query separators", i)
		}
		if !strings.EqualFold(owner, repository.Owner) {
			problems.add("repositories must share one owner")
		}
		key := strings.ToLower(repository.Owner + "/" + repository.Name)
		if _, exists := seen[key]; exists {
			problems.add("duplicate repository %s/%s", repository.Owner, repository.Name)
		}
		seen[key] = struct{}{}
		if repository.RunnerGroupID < 1 {
			problems.add("repositories[%d].runner_group_id must be greater than zero", i)
		}
		if repository.Visibility != "private" && repository.Visibility != "public" {
			problems.add("repositories[%d].visibility must be private or public", i)
		}
		if repository.Visibility == "public" && len(repository.TrustedWorkflows) == 0 {
			problems.add("repositories[%d] public repositories require trusted_workflows", i)
		}
		validateTrustedWorkflows(i, repository.TrustedWorkflows, participantLabels, problems)
	}
}

func validateTrustedWorkflows(repositoryIndex int, workflows []TrustedWorkflow, participantLabels []string, problems *validationErrors) {
	identities := make(map[string]struct{}, len(workflows))
	for workflowIndex := range workflows {
		workflow := &workflows[workflowIndex]
		workflow.WorkflowPath = strings.TrimSpace(workflow.WorkflowPath)
		field := fmt.Sprintf("repositories[%d].trusted_workflows[%d]", repositoryIndex, workflowIndex)
		if (workflow.WorkflowID > 0) == (workflow.WorkflowPath != "") {
			problems.add("%s must set exactly one of workflow_id or workflow_path", field)
		}
		if workflow.WorkflowID < 0 {
			problems.add("%s.workflow_id must be greater than zero", field)
		}
		if workflow.WorkflowPath != "" && !validWorkflowPath(workflow.WorkflowPath) {
			problems.add("%s.workflow_path must be an exact .github/workflows/*.yml or *.yaml path", field)
		}
		identity := fmt.Sprintf("id:%d", workflow.WorkflowID)
		if workflow.WorkflowPath != "" {
			identity = "path:" + workflow.WorkflowPath
		}
		if _, exists := identities[identity]; exists {
			problems.add("%s duplicates trusted workflow identity %q", field, identity)
		}
		identities[identity] = struct{}{}
		validateRules(field, workflow.Rules, participantLabels, problems)
	}
}

func validateRules(field string, rules []AuthorizationRule, participantLabels []string, problems *validationErrors) {
	if len(rules) == 0 {
		problems.add("%s.rules must contain at least one authorization rule", field)
		return
	}
	seen := make(map[string]struct{}, len(rules))
	for ruleIndex := range rules {
		rule := &rules[ruleIndex]
		ruleField := fmt.Sprintf("%s.rules[%d]", field, ruleIndex)
		validatePolicyValues(ruleField+".events", rule.Events, false, problems)
		validatePolicyValues(ruleField+".actors", rule.Actors, true, problems)
		validatePolicyValues(ruleField+".required_labels", rule.RequiredLabels, false, problems)
		if containsFold(rule.Actors, "*") && len(rule.Actors) != 1 {
			problems.add("%s.actors wildcard must be the only actor", ruleField)
		}
		for _, label := range rule.RequiredLabels {
			if !containsFold(participantLabels, label) {
				problems.add("%s.required_labels contains label %q not advertised by participant", ruleField, label)
			}
		}
		identity := canonicalRule(*rule)
		if _, exists := seen[identity]; exists {
			problems.add("%s duplicates another authorization rule", ruleField)
		}
		seen[identity] = struct{}{}
	}
}

func validatePolicyValues(field string, values []string, allowWildcard bool, problems *validationErrors) {
	if len(values) == 0 {
		problems.add("%s must contain at least one value", field)
		return
	}
	seen := make(map[string]struct{}, len(values))
	for i := range values {
		values[i] = strings.TrimSpace(values[i])
		key := strings.ToLower(values[i])
		if values[i] == "" || len(values[i]) > 100 || strings.ContainsAny(values[i], " \t\r\n/\\?#") || (values[i] == "*" && !allowWildcard) {
			problems.add("%s contains invalid value %q", field, values[i])
		} else if _, exists := seen[key]; exists {
			problems.add("%s contains duplicate %q", field, values[i])
		}
		seen[key] = struct{}{}
	}
}

func validWorkflowPath(value string) bool {
	return strings.HasPrefix(value, ".github/workflows/") &&
		(strings.HasSuffix(value, ".yml") || strings.HasSuffix(value, ".yaml")) &&
		!strings.ContainsAny(value, "\\@?#") && !strings.Contains(value, "//") &&
		!strings.Contains(value, "/../") && !strings.Contains(value, "/./")
}

func canonicalRule(rule AuthorizationRule) string {
	parts := make([]string, 0, len(rule.Events)+len(rule.Actors)+len(rule.RequiredLabels))
	for _, value := range rule.Events {
		parts = append(parts, "event:"+value)
	}
	for _, value := range rule.Actors {
		parts = append(parts, "actor:"+strings.ToLower(value))
	}
	for _, value := range rule.RequiredLabels {
		parts = append(parts, "label:"+strings.ToLower(value))
	}
	sort.Strings(parts)
	return strings.Join(parts, "\x00")
}

func containsFold(values []string, wanted string) bool {
	for _, value := range values {
		if strings.EqualFold(value, wanted) {
			return true
		}
	}
	return false
}

func validateLabels(labels []string, problems *validationErrors) {
	osLabel, archLabel, err := hostLabels()
	if err != nil {
		problems.add("labels cannot be validated: %v", err)
		return
	}
	seen := make(map[string]struct{}, len(labels))
	for i, label := range labels {
		label = strings.TrimSpace(label)
		labels[i] = label
		key := strings.ToLower(label)
		if label == "" {
			problems.add("labels must not contain empty values")
		} else if _, exists := seen[key]; exists {
			problems.add("labels contain duplicate %q", label)
		}
		seen[key] = struct{}{}
	}
	if _, ok := seen["self-hosted"]; !ok {
		problems.add("labels must include self-hosted")
	}
	if _, ok := seen[strings.ToLower(osLabel)]; !ok {
		problems.add("labels must include actual operating-system label %s", osLabel)
	}
	if _, ok := seen[strings.ToLower(archLabel)]; !ok {
		problems.add("labels must include actual architecture label %s", archLabel)
	}
}

func hostLabels() (string, string, error) {
	var osLabel string
	switch runtime.GOOS {
	case "linux":
		osLabel = "Linux"
	case "windows":
		osLabel = "Windows"
	default:
		return "", "", fmt.Errorf("unsupported operating system %s", runtime.GOOS)
	}
	if runtime.GOARCH != "amd64" {
		return "", "", fmt.Errorf("unsupported architecture %s", runtime.GOARCH)
	}
	return osLabel, "X64", nil
}

func validateURL(value string, problems *validationErrors) {
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname()))) {
		problems.add("github_api_url must be an HTTPS URL or an HTTP loopback URL")
	}
}

func isLoopbackHost(host string) bool {
	return strings.EqualFold(host, "localhost") || host == "127.0.0.1" || host == "::1"
}

func validAPIVersion(value string) bool {
	if len(value) != len("2026-03-10") || value[4] != '-' || value[7] != '-' {
		return false
	}
	_, err := time.Parse("2006-01-02", value)
	return err == nil
}

func validatePaths(cfg *Config, problems *validationErrors) {
	paths := []struct {
		name  string
		value string
		dir   bool
	}{
		{"token_file", cfg.TokenFile, false},
		{"runner_template_dir", cfg.RunnerTemplateDir, true},
		{"state_dir", cfg.StateDir, true},
	}
	for _, item := range paths {
		if item.value == "" {
			problems.add("%s is required", item.name)
			continue
		}
		if !filepath.IsAbs(item.value) {
			problems.add("%s must be an absolute path", item.name)
			continue
		}
		if item.dir && isFilesystemRoot(item.value) {
			problems.add("%s must not be a filesystem root", item.name)
		}
		if err := validateLinkFreeAncestry(item.value); err != nil {
			problems.add("%s: %v", item.name, err)
		}
	}
	if pathsOverlap(cfg.RunnerTemplateDir, cfg.StateDir) {
		problems.add("runner_template_dir and state_dir must not overlap")
	}
	if pathWithin(cfg.TokenFile, cfg.StateDir) {
		problems.add("token_file must not be inside state_dir")
	}
}

func isFilesystemRoot(path string) bool {
	clean := filepath.Clean(path)
	return samePath(clean, filepath.VolumeName(clean)+string(os.PathSeparator))
}

func validateLinkFreeAncestry(path string) error {
	current := filepath.Clean(path)
	for {
		info, err := os.Lstat(current)
		if err == nil {
			if isLinkOrReparse(info) {
				return fmt.Errorf("path ancestry contains a symbolic link or reparse point: %s", current)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		parent := filepath.Dir(current)
		if samePath(parent, current) {
			return nil
		}
		current = parent
	}
}

func pathsOverlap(a, b string) bool {
	return pathWithin(a, b) || pathWithin(b, a)
}

func pathWithin(path, parent string) bool {
	if path == "" || parent == "" || !filepath.IsAbs(path) || !filepath.IsAbs(parent) {
		return false
	}
	relative, err := filepath.Rel(filepath.Clean(parent), filepath.Clean(path))
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)))
}

func samePath(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

type validationErrors []string

func (problems *validationErrors) add(format string, args ...any) {
	*problems = append(*problems, fmt.Sprintf(format, args...))
}

func (problems validationErrors) err() error {
	if len(problems) == 0 {
		return nil
	}
	return errors.New("invalid configuration:\n- " + strings.Join(problems, "\n- "))
}
