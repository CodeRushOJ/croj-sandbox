package security

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
	"time"
)

func TestCgroupIDForProcessIncludesSanitizedInstance(t *testing.T) {
	t.Setenv("CROJ_SANDBOX_INSTANCE_ID", "pod/123 unsafe")

	got := CgroupIDForProcess("execute", 42)
	want := "execute_pod_123_unsafe_42"
	if got != want {
		t.Fatalf("CgroupIDForProcess() = %q, want %q", got, want)
	}
}

func TestCleanupCgroupV2KillsWaitsForEmptyThenRemoves(t *testing.T) {
	cgroupPath := filepath.Join(t.TempDir(), "request")
	actions := make([]string, 0, 4)
	eventReads := 0
	ops := cgroupFileOps{
		writeFile: func(path string, data []byte, _ os.FileMode) error {
			actions = append(actions, "write:"+filepath.Base(path)+"="+string(data))
			return nil
		},
		readFile: func(path string) ([]byte, error) {
			actions = append(actions, "read:"+filepath.Base(path))
			eventReads++
			if eventReads == 1 {
				return []byte("populated 1\nfrozen 0\n"), nil
			}
			return []byte("populated 0\nfrozen 0\n"), nil
		},
		remove: func(path string) error {
			actions = append(actions, "remove:"+filepath.Base(path))
			return nil
		},
		sleep: func(time.Duration) {},
	}
	manager := &CgroupManager{Version: 2, BasePath: cgroupPath, Initialized: true}

	if err := cleanupCgroupV2WithOps(manager, ops, time.Second); err != nil {
		t.Fatalf("cleanup cgroup: %v", err)
	}
	want := []string{
		"write:cgroup.kill=1",
		"read:cgroup.events",
		"read:cgroup.events",
		"remove:request",
	}
	if !reflect.DeepEqual(actions, want) {
		t.Fatalf("cleanup actions = %v, want %v", actions, want)
	}
}

func TestMoveProcessesIgnoresExitedPIDAndRechecksMembership(t *testing.T) {
	podPath := t.TempDir()
	supervisorPath := filepath.Join(podPath, ".croj-supervisor")
	reads := 0
	writes := 0
	err := moveProcessesToSupervisorWithIO(
		podPath,
		supervisorPath,
		func(string) ([]byte, error) {
			reads++
			if reads == 1 {
				return []byte("123\n"), nil
			}
			return nil, nil
		},
		func(string, []byte, os.FileMode) error {
			writes++
			return syscall.ESRCH
		},
	)
	if err != nil {
		t.Fatalf("move exited PID: %v", err)
	}
	if reads != 2 || writes != 1 {
		t.Fatalf("reads=%d writes=%d, want reads=2 writes=1", reads, writes)
	}
}

func TestMoveProcessesFailsClosedForNonESRCHWriteError(t *testing.T) {
	podPath := t.TempDir()
	wantErr := errors.New("write denied")
	err := moveProcessesToSupervisorWithIO(
		podPath,
		filepath.Join(podPath, ".croj-supervisor"),
		func(string) ([]byte, error) { return []byte("123\n"), nil },
		func(string, []byte, os.FileMode) error { return wantErr },
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("move error = %v, want wrapped %v", err, wantErr)
	}
}
