//go:build windows

package runner

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

const (
	processQueryLimitedInformation = 0x1000
	processTerminate               = 0x0001
)

var queryFullProcessImageName = syscall.NewLazyDLL("kernel32.dll").NewProc("QueryFullProcessImageNameW")

type NativeProcessController struct{}

type nativeProcess struct {
	command  *exec.Cmd
	identity ProcessIdentity
}

func (NativeProcessController) Start(ctx context.Context, spec StartSpec) (Process, error) {
	command := exec.CommandContext(ctx, "cmd.exe", "/d", "/s", "/c", spec.Executable)
	command.Dir, command.Env = spec.WorkingDir, spec.Environment
	if err := command.Start(); err != nil {
		return nil, err
	}
	identity, err := windowsIdentity(command.Process.Pid)
	if err != nil {
		_ = command.Process.Kill()
		return nil, err
	}
	return &nativeProcess{command: command, identity: identity}, nil
}

func (process *nativeProcess) Identity() ProcessIdentity { return process.identity }
func (process *nativeProcess) Wait() error               { return process.command.Wait() }

func (NativeProcessController) Inspect(expected ProcessIdentity) (ProcessStatus, error) {
	actual, err := windowsIdentity(expected.PID)
	if errors.Is(err, syscall.Errno(87)) {
		return ProcessMissing, nil
	}
	if err != nil {
		return ProcessMismatched, err
	}
	if actual.StartMarker != expected.StartMarker || !equalWindowsPath(actual.Executable, expected.Executable) {
		return ProcessMismatched, nil
	}
	return ProcessMatches, nil
}

func (NativeProcessController) Terminate(identity ProcessIdentity) error {
	handle, err := syscall.OpenProcess(processTerminate, false, uint32(identity.PID))
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(handle)
	return syscall.TerminateProcess(handle, 1)
}

func windowsIdentity(pid int) (ProcessIdentity, error) {
	handle, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return ProcessIdentity{}, err
	}
	defer syscall.CloseHandle(handle)
	var creation, exit, kernel, user syscall.Filetime
	if err := syscall.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err != nil {
		return ProcessIdentity{}, err
	}
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	result, _, callErr := queryFullProcessImageName.Call(uintptr(handle), 0, uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&size)))
	if result == 0 {
		return ProcessIdentity{}, callErr
	}
	return ProcessIdentity{PID: pid, StartMarker: strconv.FormatUint(uint64(creation.HighDateTime)<<32|uint64(creation.LowDateTime), 10), Executable: syscall.UTF16ToString(buffer[:size])}, nil
}

func equalWindowsPath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}
