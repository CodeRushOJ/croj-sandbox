package sandbox

import (
	"context"
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
