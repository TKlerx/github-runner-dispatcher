//go:build linux

package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

type NativeProcessController struct{}

type nativeProcess struct {
	command  *exec.Cmd
	identity ProcessIdentity
}

func (NativeProcessController) Start(ctx context.Context, spec StartSpec) (Process, error) {
	command := exec.CommandContext(ctx, spec.Executable)
	command.Dir, command.Env = spec.WorkingDir, spec.Environment
	if err := command.Start(); err != nil {
		return nil, err
	}
	identity, err := linuxIdentity(command.Process.Pid)
	if err != nil {
		_ = command.Process.Kill()
		return nil, err
	}
	return &nativeProcess{command: command, identity: identity}, nil
}

func (process *nativeProcess) Identity() ProcessIdentity { return process.identity }
func (process *nativeProcess) Wait() error               { return process.command.Wait() }

func (NativeProcessController) Inspect(expected ProcessIdentity) (ProcessStatus, error) {
	actual, err := linuxIdentity(expected.PID)
	if errors.Is(err, os.ErrNotExist) {
		return ProcessMissing, nil
	}
	if err != nil {
		return ProcessMismatched, err
	}
	if actual.StartMarker != expected.StartMarker || filepath.Clean(actual.Executable) != filepath.Clean(expected.Executable) {
		return ProcessMismatched, nil
	}
	return ProcessMatches, nil
}

func (NativeProcessController) Terminate(identity ProcessIdentity) error {
	process, err := os.FindProcess(identity.PID)
	if err != nil {
		return err
	}
	return process.Signal(syscall.SIGTERM)
}

func linuxIdentity(pid int) (ProcessIdentity, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return ProcessIdentity{}, err
	}
	closing := strings.LastIndexByte(string(data), ')')
	if closing < 0 {
		return ProcessIdentity{}, errors.New("invalid process stat")
	}
	fields := strings.Fields(string(data)[closing+1:])
	if len(fields) < 20 {
		return ProcessIdentity{}, errors.New("invalid process stat")
	}
	executable, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		return ProcessIdentity{}, err
	}
	if _, err := strconv.ParseUint(fields[19], 10, 64); err != nil {
		return ProcessIdentity{}, errors.New("invalid process start marker")
	}
	return ProcessIdentity{PID: pid, StartMarker: fields[19], Executable: executable}, nil
}
