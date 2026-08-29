package runner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrCapacity           = errors.New("runner capacity is full")
	ErrAcquisitionTimeout = errors.New("runner acquisition timed out")
)

type LaunchRequest struct {
	Repository    RepositoryIdentity
	ObservedJobID int64
	RunnerID      int64
	RunnerName    string
	JITConfig     string
}

type Manager struct {
	StateDir           string
	TemplateDir        string
	capacity           int32
	acquisitionTimeout time.Duration
	controller         ProcessController
	localActive        atomic.Int32
	recoveredActive    atomic.Int32
	mu                 sync.Mutex
	owned              map[string]bool
}

func NewManager(stateDir, templateDir string, capacity int, acquisitionTimeout time.Duration, controller ProcessController) (*Manager, error) {
	if capacity < 1 || acquisitionTimeout <= 0 || controller == nil {
		return nil, errors.New("invalid runner manager configuration")
	}
	if err := os.MkdirAll(filepath.Join(stateDir, "runners"), 0o700); err != nil {
		return nil, fmt.Errorf("create runner state directory: %w", err)
	}
	return &Manager{StateDir: stateDir, TemplateDir: templateDir, capacity: int32(capacity), acquisitionTimeout: acquisitionTimeout, controller: controller, owned: map[string]bool{}}, nil
}

func (manager *Manager) Active() int {
	return int(manager.localActive.Load() + manager.recoveredActive.Load())
}

func (manager *Manager) Run(ctx context.Context, request LaunchRequest, assigned <-chan struct{}) error {
	if manager.localActive.Add(1)+manager.recoveredActive.Load() > manager.capacity {
		manager.localActive.Add(-1)
		return ErrCapacity
	}
	defer manager.localActive.Add(-1)

	instanceID, err := randomID()
	if err != nil {
		return err
	}
	directory := filepath.Join(manager.StateDir, "runners", instanceID)
	manager.mu.Lock()
	manager.owned[instanceID] = true
	manager.mu.Unlock()
	defer func() {
		manager.mu.Lock()
		delete(manager.owned, instanceID)
		manager.mu.Unlock()
	}()
	if err := copyTree(manager.TemplateDir, directory); err != nil {
		_ = manager.removeInstance(directory)
		return fmt.Errorf("copy runner template: %w", err)
	}
	manifest := Manifest{
		SchemaVersion: ManifestSchemaVersion, InstanceID: instanceID, Repository: request.Repository,
		ObservedJobID: request.ObservedJobID, RunnerID: request.RunnerID, RunnerName: request.RunnerName,
		Phase: PhasePreparing, CreatedAt: time.Now().UTC(), AcquisitionDeadline: time.Now().Add(manager.acquisitionTimeout).UTC(),
	}
	if err := writeManifest(directory, &manifest); err != nil {
		_ = manager.removeInstance(directory)
		return err
	}

	runnerScript := "run.sh"
	if runtime.GOOS == "windows" {
		runnerScript = "run.cmd"
	}
	process, err := manager.controller.Start(ctx, StartSpec{
		Executable: filepath.Join(directory, runnerScript), WorkingDir: directory,
		Environment: childEnvironment(request.JITConfig),
	})
	if err != nil {
		manifest.Phase, manifest.LastError = PhaseFailed, "runner process failed to start"
		_ = writeManifest(directory, &manifest)
		return manager.cleanup(directory, &manifest, err)
	}
	identity := process.Identity()
	manifest.ProcessID, manifest.ProcessStartMarker, manifest.ProcessExecutable = identity.PID, identity.StartMarker, identity.Executable
	manifest.Phase = PhaseWaiting
	if err := writeManifest(directory, &manifest); err != nil {
		_ = manager.terminate(identity)
		return manager.cleanup(directory, &manifest, err)
	}

	wait := make(chan error, 1)
	go func() { wait <- process.Wait() }()
	timer := time.NewTimer(manager.acquisitionTimeout)
	defer timer.Stop()
	select {
	case err = <-wait:
		manifest.Phase = PhaseExited
	case <-assigned:
		manifest.Phase = PhaseAssigned
		if writeErr := writeManifest(directory, &manifest); writeErr != nil {
			_ = manager.terminate(identity)
			return manager.cleanup(directory, &manifest, writeErr)
		}
		select {
		case err = <-wait:
		case <-ctx.Done():
			err = ctx.Err()
			_ = manager.terminate(identity)
			<-wait
		}
		manifest.Phase = PhaseExited
	case <-timer.C:
		manifest.Phase = PhaseTimedOut
		err = ErrAcquisitionTimeout
		if terminateErr := manager.terminate(identity); terminateErr != nil {
			err = terminateErr
		} else {
			<-wait
		}
	case <-ctx.Done():
		err = ctx.Err()
		_ = manager.terminate(identity)
		<-wait
	}
	if writeErr := writeManifest(directory, &manifest); writeErr != nil && err == nil {
		err = writeErr
	}
	return manager.cleanup(directory, &manifest, err)
}

func (manager *Manager) terminate(identity ProcessIdentity) error {
	status, err := manager.controller.Inspect(identity)
	if err != nil {
		return err
	}
	if status == ProcessMissing {
		return nil
	}
	if status != ProcessMatches {
		return errors.New("refusing to terminate a process whose identity does not match")
	}
	return manager.controller.Terminate(identity)
}

func (manager *Manager) cleanup(directory string, manifest *Manifest, prior error) error {
	manifest.Phase = PhaseCleaning
	manifest.CleanupAttempts++
	_ = writeManifest(directory, manifest)
	if err := manager.removeInstance(directory); err != nil {
		manifest.Phase, manifest.LastError = PhaseCleanupFailed, "runner directory cleanup failed"
		_ = writeManifest(directory, manifest)
		return errors.Join(prior, err)
	}
	return prior
}

func (manager *Manager) Reconcile(ctx context.Context) error {
	root := filepath.Join(manager.StateDir, "runners")
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	recovered := int32(0)
	var problems []error
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		manager.mu.Lock()
		owned := manager.owned[entry.Name()]
		manager.mu.Unlock()
		if owned {
			continue
		}
		directory := filepath.Join(root, entry.Name())
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			recovered++
			problems = append(problems, fmt.Errorf("unknown runner state entry %s", entry.Name()))
			continue
		}
		manifest, readErr := readManifest(directory)
		if readErr != nil || manifest.InstanceID != entry.Name() || manifest.SchemaVersion != ManifestSchemaVersion {
			recovered++
			problems = append(problems, fmt.Errorf("invalid runner manifest %s", entry.Name()))
			continue
		}
		if manifest.Phase == PhaseCleanupFailed || manifest.Phase == PhaseCleaning || manifest.Phase == PhaseExited || manifest.Phase == PhaseFailed || manifest.Phase == PhaseTimedOut || manifest.ProcessID == 0 {
			if removeErr := manager.removeInstance(directory); removeErr != nil {
				recovered++
				problems = append(problems, removeErr)
			}
			continue
		}
		identity := ProcessIdentity{PID: manifest.ProcessID, StartMarker: manifest.ProcessStartMarker, Executable: manifest.ProcessExecutable}
		status, inspectErr := manager.controller.Inspect(identity)
		if inspectErr != nil {
			recovered++
			problems = append(problems, fmt.Errorf("inspect runner %s: %w", entry.Name(), inspectErr))
			continue
		}
		switch status {
		case ProcessMissing:
			if removeErr := manager.removeInstance(directory); removeErr != nil {
				recovered++
				problems = append(problems, removeErr)
			}
		case ProcessMismatched:
			recovered++
			problems = append(problems, fmt.Errorf("runner %s process identity does not match", entry.Name()))
		case ProcessMatches:
			if manifest.Phase == PhaseWaiting && !manifest.AcquisitionDeadline.IsZero() && time.Now().After(manifest.AcquisitionDeadline) {
				if terminateErr := manager.controller.Terminate(identity); terminateErr != nil {
					recovered++
					problems = append(problems, terminateErr)
				} else if removeErr := manager.removeInstance(directory); removeErr != nil {
					recovered++
					problems = append(problems, removeErr)
				}
			} else {
				recovered++
			}
		}
	}
	manager.recoveredActive.Store(recovered)
	return errors.Join(problems...)
}

func (manager *Manager) removeInstance(directory string) error {
	root := filepath.Clean(filepath.Join(manager.StateDir, "runners"))
	relative, err := filepath.Rel(root, filepath.Clean(directory))
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) || strings.ContainsRune(relative, os.PathSeparator) {
		return errors.New("refusing cleanup outside the managed runner root")
	}
	info, err := os.Lstat(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("refusing cleanup of a linked runner directory")
	}
	return os.RemoveAll(directory)
}

func readManifest(directory string) (Manifest, error) {
	file, err := os.Open(filepath.Join(directory, "manifest.json"))
	if err != nil {
		return Manifest{}, err
	}
	defer file.Close()
	var manifest Manifest
	decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func childEnvironment(jitConfig string) []string {
	environment := make([]string, 0, len(os.Environ())+1)
	for _, item := range os.Environ() {
		key := strings.SplitN(item, "=", 2)[0]
		if strings.EqualFold(key, "ACTIONS_RUNNER_INPUT_JITCONFIG") || strings.EqualFold(key, "GH_TOKEN") || strings.EqualFold(key, "GITHUB_TOKEN") {
			continue
		}
		environment = append(environment, item)
	}
	return append(environment, "ACTIONS_RUNNER_INPUT_JITCONFIG="+jitConfig)
}

func randomID() (string, error) {
	data := make([]byte, 8)
	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("create runner instance ID: %w", err)
	}
	return hex.EncodeToString(data), nil
}

func copyTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("runner template contains symbolic link %s", relative)
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("runner template contains non-regular file %s", relative)
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(source, destination string, mode fs.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	return errors.Join(copyErr, closeErr)
}

func writeManifest(directory string, manifest *Manifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".manifest-*.tmp")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, filepath.Join(directory, "manifest.json"))
}
