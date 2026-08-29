package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"

	"github.com/TKlerx/github-runner-dispatcher/internal/config"
	ghapi "github.com/TKlerx/github-runner-dispatcher/internal/github"
	"github.com/TKlerx/github-runner-dispatcher/internal/participant"
	"github.com/TKlerx/github-runner-dispatcher/internal/runner"
	"github.com/TKlerx/github-runner-dispatcher/internal/setup"
)

var errGitHubCheck = errors.New("GitHub validation failed")

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, setup.CLIExecutor{}))
}

func run(ctx context.Context, args []string, input io.Reader, output, errorOutput io.Writer, executor setup.Executor) int {
	flags := flag.NewFlagSet("runner-participant", flag.ContinueOnError)
	flags.SetOutput(errorOutput)
	configPath := flags.String("config", "", "configuration file")
	check := flags.Bool("check", false, "validate configuration and access without side effects")
	configure := flags.Bool("setup", false, "select repositories and print PAT instructions")
	if err := flags.Parse(args); err != nil || *configPath == "" || (*check && *configure) {
		fmt.Fprintln(errorOutput, "usage: runner-participant -config <path> [-check | -setup]")
		return 2
	}
	if *configure {
		if err := setup.Run(ctx, *configPath, input, output, executor); err != nil {
			fmt.Fprintln(errorOutput, err)
			if errors.Is(err, setup.ErrGitHubCLI) {
				return 3
			}
			return 2
		}
		return 0
	}
	if err := checkConfiguration(ctx, *configPath); err != nil {
		fmt.Fprintln(errorOutput, err)
		if errors.Is(err, errGitHubCheck) {
			return 3
		}
		return 2
	}
	if *check {
		fmt.Fprintln(output, "configuration, local prerequisites, private repositories, and GitHub permissions are valid")
		return 0
	}
	if err := runParticipant(ctx, *configPath); err != nil {
		fmt.Fprintln(errorOutput, err)
		return 4
	}
	return 0
}

func runParticipant(ctx context.Context, path string) error {
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	token, err := config.LoadToken(cfg.TokenFile)
	if err != nil {
		return err
	}
	client, err := ghapi.NewClient(cfg.GitHubAPIURL, cfg.GitHubAPIVersion, token, http.DefaultClient)
	if err != nil {
		return err
	}
	manager, err := runner.NewManager(cfg.StateDir, cfg.RunnerTemplateDir, cfg.Capacity, cfg.AcquisitionTimeout, runner.NativeProcessController{})
	if err != nil {
		return err
	}
	service, err := participant.NewService(cfg, client, manager)
	if err != nil {
		return err
	}
	return service.Run(ctx)
}

func checkConfiguration(ctx context.Context, path string) error {
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	token, err := config.LoadToken(cfg.TokenFile)
	if err != nil {
		return err
	}
	if err := requireDirectory(cfg.StateDir, "state_dir"); err != nil {
		return err
	}
	if err := requireDirectory(cfg.RunnerTemplateDir, "runner_template_dir"); err != nil {
		return err
	}
	runnerScript := "run.sh"
	if runtime.GOOS == "windows" {
		runnerScript = "run.cmd"
	}
	info, err := os.Stat(filepath.Join(cfg.RunnerTemplateDir, runnerScript))
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("runner_template_dir must contain regular file %s", runnerScript)
	}
	client, err := ghapi.NewClient(cfg.GitHubAPIURL, cfg.GitHubAPIVersion, token, http.DefaultClient)
	if err != nil {
		return err
	}
	for _, item := range cfg.Repositories {
		repository := ghapi.Repository{Owner: item.Owner, Name: item.Name}
		if err := client.ValidatePrivateRepository(ctx, repository); err != nil {
			return fmt.Errorf("%w for %s/%s: %v", errGitHubCheck, item.Owner, item.Name, err)
		}
		if _, err := client.ListWorkflowRuns(ctx, repository, "queued"); err != nil {
			return fmt.Errorf("%w for %s/%s: %v", errGitHubCheck, item.Owner, item.Name, err)
		}
		if err := client.CheckAdministration(ctx, repository); err != nil {
			return fmt.Errorf("%w for %s/%s: %v", errGitHubCheck, item.Owner, item.Name, err)
		}
	}
	return nil
}

func requireDirectory(path, field string) error {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("%s must be an existing directory", field)
	}
	return nil
}
