package sandbox

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestExecutorFailsClosedWhenSecurityLauncherIsMissing(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Language = "go"
	cfg.WorkingDir = t.TempDir()
	cfg.SandboxExecPath = "/definitely/missing/croj-sandbox-exec"
	cfg.DefaultExecuteTimeLimit = time.Second

	result := NewExecutor(cfg).Execute(context.Background(), []string{"/bin/true"}, nil, nil)

	if result.Status != StatusSandboxError {
		t.Fatalf("status = %q, want %q: %+v", result.Status, StatusSandboxError, result)
	}
}

func TestCgroupCleanupFailureOverridesAcceptedResult(t *testing.T) {
	cleanupErr := errors.New("cgroup remained populated")
	result := resultAfterCgroupCleanup(
		Result{Status: StatusAccepted, ExitCode: 0},
		cleanupErr,
	)
	if result.Status != StatusSandboxError {
		t.Fatalf("status = %q, want %q", result.Status, StatusSandboxError)
	}
	if !strings.Contains(result.Error, cleanupErr.Error()) {
		t.Fatalf("cleanup error was not surfaced: %+v", result)
	}
}
