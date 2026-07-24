package sandbox

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestPrivilegedLinuxExecutorAppliesChildIsolation(t *testing.T) {
	if os.Getenv("CROJ_RUN_EXECUTOR_ISOLATION_TEST") != "1" {
		t.Skip("set CROJ_RUN_EXECUTOR_ISOLATION_TEST=1 in the privileged Linux test container")
	}
	launcher := os.Getenv("CROJ_TEST_SANDBOX_EXEC")
	if launcher == "" {
		t.Fatal("CROJ_TEST_SANDBOX_EXEC is required")
	}
	t.Setenv("CROJ_SECRET_SENTINEL", "must-not-reach-contestant")

	cfg := DefaultConfig()
	cfg.Language = "go"
	cfg.WorkingDir = t.TempDir()
	cfg.SandboxExecPath = launcher
	cfg.DefaultExecuteTimeLimit = 3 * time.Second
	cfg.DefaultExecuteMemoryLimit = 64 * 1024 * 1024

	result := NewExecutor(cfg).Execute(
		context.Background(),
		[]string{
			"/bin/sh",
			"-c",
			`id -u; awk '/^(CapEff|NoNewPrivs|Seccomp):/ {print}' /proc/self/status; cat /proc/self/cgroup; test -z "$CROJ_SECRET_SENTINEL"`,
		},
		nil,
		nil,
	)
	if result.Status != StatusAccepted {
		t.Fatalf("isolated command failed: %+v", result)
	}
	lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	if len(lines) < 5 {
		t.Fatalf("isolation output is incomplete: %q", result.Stdout)
	}
	uid, err := strconv.Atoi(lines[0])
	if err != nil || uid < 200000 {
		t.Fatalf("contestant uid = %q, want an isolated uid", lines[0])
	}
	for _, expected := range []string{"CapEff:\t0000000000000000", "NoNewPrivs:\t1", "Seccomp:\t2", ".croj-jobs/execute_"} {
		if !strings.Contains(result.Stdout, expected) {
			t.Fatalf("isolation output %q does not contain %q", result.Stdout, expected)
		}
	}
}

func TestPrivilegedLinuxExecutorBlocksNetworkSockets(t *testing.T) {
	if os.Getenv("CROJ_RUN_EXECUTOR_ISOLATION_TEST") != "1" {
		t.Skip("set CROJ_RUN_EXECUTOR_ISOLATION_TEST=1 in the privileged Linux test container")
	}
	cfg := DefaultConfig()
	cfg.Language = "python"
	cfg.WorkingDir = t.TempDir()
	cfg.SandboxExecPath = os.Getenv("CROJ_TEST_SANDBOX_EXEC")
	cfg.DefaultExecuteTimeLimit = 3 * time.Second
	cfg.DefaultExecuteMemoryLimit = 64 * 1024 * 1024

	result := NewExecutor(cfg).Execute(
		context.Background(),
		[]string{
			"python3",
			"-c",
			"import errno, socket\n" +
				"try:\n" +
				"    socket.socket()\n" +
				"except PermissionError as error:\n" +
				"    raise SystemExit(0 if error.errno == errno.EPERM else 1)\n" +
				"raise SystemExit(2)\n",
		},
		nil,
		nil,
	)
	if result.Status != StatusAccepted {
		t.Fatalf("socket policy was not enforced: %+v", result)
	}
}

func TestPrivilegedLinuxExecutorKillsDaemonProcessTreeOnParentExit(t *testing.T) {
	if os.Getenv("CROJ_RUN_EXECUTOR_ISOLATION_TEST") != "1" {
		t.Skip("set CROJ_RUN_EXECUTOR_ISOLATION_TEST=1 in the privileged Linux test container")
	}
	cfg := DefaultConfig()
	cfg.Language = "go"
	cfg.WorkingDir = t.TempDir()
	cfg.SandboxExecPath = os.Getenv("CROJ_TEST_SANDBOX_EXEC")
	cfg.DefaultExecuteTimeLimit = 3 * time.Second
	cfg.DefaultExecuteMemoryLimit = 64 * 1024 * 1024
	pidFile := cfg.WorkingDir + "/daemon.pid"

	result := NewExecutor(cfg).Execute(
		context.Background(),
		[]string{
			"/bin/sh",
			"-c",
			"sleep 300 </dev/null >/dev/null 2>&1 & echo $! > daemon.pid",
		},
		nil,
		nil,
	)
	if result.Status != StatusAccepted {
		t.Fatalf("daemon parent failed: %+v", result)
	}
	rawPID, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("read daemon PID: %v", err)
	}
	daemonPID, err := strconv.Atoi(strings.TrimSpace(string(rawPID)))
	if err != nil {
		t.Fatalf("parse daemon PID %q: %v", rawPID, err)
	}
	t.Cleanup(func() { _ = unix.Kill(daemonPID, syscall.SIGKILL) })

	deadline := time.Now().Add(2 * time.Second)
	for {
		err = unix.Kill(daemonPID, 0)
		if errors.Is(err, syscall.ESRCH) {
			break
		}
		if err != nil {
			t.Fatalf("probe daemon PID %d: %v", daemonPID, err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("daemon PID %d survived executor cleanup", daemonPID)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestPrivilegedLinuxRunnerCompilesAndExecutesInsideIsolation(t *testing.T) {
	if os.Getenv("CROJ_RUN_EXECUTOR_ISOLATION_TEST") != "1" {
		t.Skip("set CROJ_RUN_EXECUTOR_ISOLATION_TEST=1 in the privileged Linux test container")
	}
	launcher := os.Getenv("CROJ_TEST_SANDBOX_EXEC")
	if launcher == "" {
		t.Fatal("CROJ_TEST_SANDBOX_EXEC is required")
	}
	runRoot, err := os.MkdirTemp("", "croj-sandbox-runner-integration-")
	if err != nil {
		t.Fatalf("create integration run root: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(runRoot) })

	cfg := DefaultConfig()
	cfg.HostTempDir = runRoot
	cfg.SandboxExecPath = launcher
	cfg.DefaultCompileTimeLimit = 10 * time.Second
	cfg.DefaultExecuteTimeLimit = 3 * time.Second
	cfg.DefaultExecuteMemoryLimit = 256 * 1024 * 1024
	runner, err := NewRunner(cfg)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	expected := "isolated-compile-ok"

	result := runner.RunWithConfig(
		context.Background(),
		"go",
		"package main\nimport \"fmt\"\nfunc main() { fmt.Print(\"isolated-compile-ok\") }\n",
		nil,
		&expected,
		cfg,
	)

	if result.Status != StatusAccepted {
		t.Fatalf("compile/run lifecycle failed: %+v", result)
	}
	if result.Stdout != expected {
		t.Fatalf("stdout = %q, want %q", result.Stdout, expected)
	}
}
