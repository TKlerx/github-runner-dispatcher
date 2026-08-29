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
	Owner         string `yaml:"owner"`
	Name          string `yaml:"name"`
	RunnerGroupID int64  `yaml:"runner_group_id,omitempty"`
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
	Owner         string `yaml:"owner"`
	Name          string `yaml:"name"`
	RunnerGroupID *int64 `yaml:"runner_group_id"`
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
		cfg.Repositories[i] = Repository{Owner: repository.Owner, Name: repository.Name, RunnerGroupID: 1}
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
	validateRepositories(cfg.Repositories, problems)
	validateLabels(cfg.Labels, problems)
	if cfg.PollInterval <= 0 {
		problems.add("poll_interval must be greater than zero")
	}
	if cfg.ClaimDelay < 0 {
		problems.add("claim_delay must not be negative")
	}
	if cfg.AcquisitionTimeout <= 0 {
		problems.add("acquisition_timeout must be greater than zero")
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

func validateRepositories(repositories []Repository, problems *validationErrors) {
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
		if repository.Owner == "" || repository.Name == "" {
			problems.add("repositories[%d] owner and name are required", i)
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
	}
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
