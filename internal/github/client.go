package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	userAgent       = "github-runner-dispatcher/dev"
	maxResponseSize = 8 << 20
	maxPages        = 100
)

var ErrPublicRepository = errors.New("public repositories are unsupported")

type Repository struct {
	Owner string
	Name  string
}

type WorkflowRun struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
}

type Job struct {
	ID         int64    `json:"id"`
	RunID      int64    `json:"run_id"`
	Name       string   `json:"name"`
	Status     string   `json:"status"`
	Conclusion string   `json:"conclusion"`
	Labels     []string `json:"labels"`
	RunnerID   int64    `json:"runner_id"`
	RunnerName string   `json:"runner_name"`
}

type JITConfigRequest struct {
	Name          string   `json:"name"`
	RunnerGroupID int64    `json:"runner_group_id"`
	Labels        []string `json:"labels"`
	WorkFolder    string   `json:"work_folder"`
}

type JITConfig struct {
	Runner struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"runner"`
	EncodedJITConfig string `json:"encoded_jit_config"`
}

type APIError struct {
	Method     string
	Path       string
	StatusCode int
	RequestID  string
}

func (err *APIError) Error() string {
	message := fmt.Sprintf("GitHub API %s %s returned %d", err.Method, err.Path, err.StatusCode)
	if err.RequestID != "" {
		message += " (request " + err.RequestID + ")"
	}
	return message
}

type Client struct {
	baseURL    *url.URL
	apiVersion string
	token      string
	httpClient *http.Client
}

func NewClient(baseURL, apiVersion, token string, httpClient *http.Client) (*Client, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return nil, errors.New("invalid GitHub API URL")
	}
	if apiVersion == "" {
		return nil, errors.New("GitHub API version is required")
	}
	if token == "" {
		return nil, errors.New("GitHub token is required")
	}
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	clientCopy := *httpClient
	if clientCopy.Timeout == 0 {
		clientCopy.Timeout = 30 * time.Second
	}
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{
		baseURL:    parsed,
		apiVersion: apiVersion,
		token:      token,
		httpClient: &clientCopy,
	}, nil
}

func (client *Client) ValidatePrivateRepository(ctx context.Context, repository Repository) error {
	endpoint, err := client.repositoryEndpoint(repository)
	if err != nil {
		return err
	}
	var response struct {
		FullName string `json:"full_name"`
		Private  bool   `json:"private"`
	}
	if _, err := client.doJSON(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
		return err
	}
	if !response.Private {
		return fmt.Errorf("%w: %s/%s", ErrPublicRepository, repository.Owner, repository.Name)
	}
	return nil
}

func (client *Client) CheckAdministration(ctx context.Context, repository Repository) error {
	endpoint, err := client.repositoryEndpoint(repository, "actions", "runners")
	if err != nil {
		return err
	}
	query := endpoint.Query()
	query.Set("per_page", "1")
	endpoint.RawQuery = query.Encode()
	var response struct {
		TotalCount int `json:"total_count"`
	}
	_, err = client.doJSON(ctx, http.MethodGet, endpoint, nil, &response)
	return err
}

func (client *Client) ListWorkflowRuns(ctx context.Context, repository Repository, status string) ([]WorkflowRun, error) {
	if status != "queued" && status != "in_progress" {
		return nil, fmt.Errorf("unsupported workflow run status %q", status)
	}
	endpoint, err := client.repositoryEndpoint(repository, "actions", "runs")
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("status", status)
	query.Set("per_page", "100")
	endpoint.RawQuery = query.Encode()

	var runs []WorkflowRun
	for page := 0; endpoint != nil; page++ {
		if page >= maxPages {
			return nil, errors.New("GitHub pagination exceeded 100 pages")
		}
		var response struct {
			Runs []WorkflowRun `json:"workflow_runs"`
		}
		headers, err := client.doJSON(ctx, http.MethodGet, endpoint, nil, &response)
		if err != nil {
			return nil, err
		}
		runs = append(runs, response.Runs...)
		endpoint, err = client.nextPage(endpoint, headers.Get("Link"))
		if err != nil {
			return nil, err
		}
	}
	return runs, nil
}

func (client *Client) ListJobs(ctx context.Context, repository Repository, runID int64) ([]Job, error) {
	if runID < 1 {
		return nil, errors.New("run ID must be greater than zero")
	}
	endpoint, err := client.repositoryEndpoint(repository, "actions", "runs", strconv.FormatInt(runID, 10), "jobs")
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("filter", "latest")
	query.Set("per_page", "100")
	endpoint.RawQuery = query.Encode()

	var jobs []Job
	for page := 0; endpoint != nil; page++ {
		if page >= maxPages {
			return nil, errors.New("GitHub pagination exceeded 100 pages")
		}
		var response struct {
			Jobs []Job `json:"jobs"`
		}
		headers, err := client.doJSON(ctx, http.MethodGet, endpoint, nil, &response)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, response.Jobs...)
		endpoint, err = client.nextPage(endpoint, headers.Get("Link"))
		if err != nil {
			return nil, err
		}
	}
	return jobs, nil
}

func (client *Client) GetJob(ctx context.Context, repository Repository, jobID int64) (Job, error) {
	if jobID < 1 {
		return Job{}, errors.New("job ID must be greater than zero")
	}
	endpoint, err := client.repositoryEndpoint(repository, "actions", "jobs", strconv.FormatInt(jobID, 10))
	if err != nil {
		return Job{}, err
	}
	var job Job
	_, err = client.doJSON(ctx, http.MethodGet, endpoint, nil, &job)
	return job, err
}

func (client *Client) GenerateJITConfig(ctx context.Context, repository Repository, request JITConfigRequest) (JITConfig, error) {
	endpoint, err := client.repositoryEndpoint(repository, "actions", "runners", "generate-jitconfig")
	if err != nil {
		return JITConfig{}, err
	}
	var response JITConfig
	_, err = client.doJSON(ctx, http.MethodPost, endpoint, request, &response)
	return response, err
}

func (client *Client) repositoryEndpoint(repository Repository, suffix ...string) (*url.URL, error) {
	if repository.Owner == "" || repository.Name == "" || strings.ContainsAny(repository.Owner+repository.Name, "/\\?#") {
		return nil, errors.New("invalid repository owner or name")
	}
	parts := append([]string{"repos", repository.Owner, repository.Name}, suffix...)
	return client.baseURL.JoinPath(parts...), nil
}

func (client *Client) doJSON(ctx context.Context, method string, endpoint *url.URL, body, destination any) (http.Header, error) {
	var requestBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode GitHub request: %w", err)
		}
		requestBody = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), requestBody)
	if err != nil {
		return nil, fmt.Errorf("create GitHub request: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Authorization", "Bearer "+client.token)
	request.Header.Set("X-GitHub-Api-Version", client.apiVersion)
	request.Header.Set("User-Agent", userAgent)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("GitHub API %s %s failed: %w", method, endpoint.EscapedPath(), err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return nil, &APIError{
			Method:     method,
			Path:       endpoint.EscapedPath(),
			StatusCode: response.StatusCode,
			RequestID:  response.Header.Get("X-GitHub-Request-Id"),
		}
	}
	if destination != nil {
		data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseSize+1))
		if err != nil {
			return nil, fmt.Errorf("read GitHub API %s %s response: %w", method, endpoint.EscapedPath(), err)
		}
		if len(data) > maxResponseSize {
			return nil, fmt.Errorf("GitHub API %s %s response exceeds %d bytes", method, endpoint.EscapedPath(), maxResponseSize)
		}
		if err := json.Unmarshal(data, destination); err != nil {
			return nil, fmt.Errorf("decode GitHub API %s %s response: %w", method, endpoint.EscapedPath(), err)
		}
	}
	return response.Header.Clone(), nil
}

func (client *Client) nextPage(current *url.URL, header string) (*url.URL, error) {
	if header == "" {
		return nil, nil
	}
	for _, value := range strings.Split(header, ",") {
		parts := strings.Split(value, ";")
		if len(parts) < 2 || !strings.Contains(strings.Join(parts[1:], ";"), `rel="next"`) {
			continue
		}
		target := strings.TrimSpace(parts[0])
		if len(target) < 2 || target[0] != '<' || target[len(target)-1] != '>' {
			return nil, errors.New("invalid GitHub pagination URL")
		}
		parsed, err := url.Parse(target[1 : len(target)-1])
		if err != nil {
			return nil, errors.New("invalid GitHub pagination URL")
		}
		parsed = current.ResolveReference(parsed)
		if parsed.User != nil || !strings.EqualFold(parsed.Scheme, client.baseURL.Scheme) || !strings.EqualFold(parsed.Host, client.baseURL.Host) {
			return nil, errors.New("GitHub pagination URL changed API origin")
		}
		return parsed, nil
	}
	return nil, nil
}
