package sandbox

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"
)

type fixedCommandExecutor struct {
	result Result
	calls  int
}

func (executor *fixedCommandExecutor) Execute(
	_ context.Context,
	_ []string,
	_ map[string]string,
	_ *string,
) Result {
	executor.calls++
	return executor.result
}

type localTestCommandExecutor struct {
	stdoutLimit int64
	stderrLimit int64
}

func (executor localTestCommandExecutor) Execute(
	ctx context.Context,
	command []string,
	_ map[string]string,
	_ *string,
) Result {
	if len(command) == 0 {
		return NewResult(StatusSandboxError, errors.New("empty test command"))
	}
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	var stdout, stderr bytes.Buffer
	stdoutLimit := executor.stdoutLimit
	if stdoutLimit <= 0 {
		stdoutLimit = int64(DefaultMaxStdoutKB) * 1024
	}
	stderrLimit := executor.stderrLimit
	if stderrLimit <= 0 {
		stderrLimit = int64(DefaultMaxStderrKB) * 1024
	}
	stdoutWriter := NewLimitedWriter(&stdout, stdoutLimit).(*LimitedWriter)
	stderrWriter := NewLimitedWriter(&stderr, stderrLimit).(*LimitedWriter)
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter

	err := cmd.Run()
	result := Result{
		Status:   StatusAccepted,
		ExitCode: 0,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		result.Status = StatusTimeLimitExceeded
		result.ExitCode = -1
		return result
	}
	if stdoutWriter.Exceeded || stderrWriter.Exceeded {
		result.Status = StatusOutputLimitExceeded
		result.ExitCode = -1
		return result
	}
	if err != nil {
		result.Status = StatusRuntimeError
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	return result
}

func TestRunWithConfigFailsClosedWhenCompilerIsolationFails(t *testing.T) {
	cfg := Config{
		HostTempDir:               t.TempDir(),
		DefaultCompileTimeLimit:   time.Second,
		DefaultExecuteTimeLimit:   time.Second,
		DefaultExecuteMemoryLimit: 64 * 1024 * 1024,
		MaxStdoutSize:             1024,
		MaxStderrSize:             1024,
		Languages: map[string]LanguageConfig{"test": {
			Compile: CompileConfig{
				SrcName:        "main.src",
				ExeName:        "main.bin",
				CompileCommand: "cp {{SRC_PATH}} {{EXE_PATH}}",
			},
			Run: RunConfig{Command: "{{EXE_PATH}}"},
		}},
	}
	isolationFailure := NewResult(StatusSandboxError, context.Canceled)
	compiler := &fixedCommandExecutor{result: isolationFailure}
	runtimeExecutor := &fixedCommandExecutor{
		result: Result{Status: StatusAccepted, ExitCode: 0},
	}
	runner := &Runner{
		cfg:      cfg,
		compiler: compiler,
		executor: runtimeExecutor,
	}

	result := runner.RunWithConfig(
		context.Background(),
		"test",
		"source",
		nil,
		nil,
		cfg,
	)

	if result.Status != StatusSandboxError {
		t.Fatalf("status = %q, want %q: %+v", result.Status, StatusSandboxError, result)
	}
	if compiler.calls != 1 {
		t.Fatalf("compiler calls = %d, want 1", compiler.calls)
	}
	if runtimeExecutor.calls != 0 {
		t.Fatalf("runtime executor calls = %d, want 0", runtimeExecutor.calls)
	}
}

func TestCompilerConfigUsesIndependentMemoryBudget(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DefaultExecuteMemoryLimit = 64 * 1024 * 1024
	cfg.DefaultCompileMemoryLimit = 1024 * 1024 * 1024

	compileCfg := isolatedCompilerConfig(cfg, "go", t.TempDir(), 240*time.Second)

	if got, want := compileCfg.DefaultExecuteMemoryLimit, int64(1024*1024*1024); got != want {
		t.Fatalf("compiler memory limit = %d, want %d", got, want)
	}
	if got, want := compileCfg.DefaultExecuteTimeLimit, 240*time.Second; got != want {
		t.Fatalf("compiler timeout = %v, want %v", got, want)
	}
	if got := cfg.DefaultExecuteMemoryLimit; got != 64*1024*1024 {
		t.Fatalf("runtime memory limit mutated to %d", got)
	}
}
