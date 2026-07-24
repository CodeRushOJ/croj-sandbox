package sandbox

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestExecuteContextUsesLanguageCompileBudget(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HostTempDir = t.TempDir()
	recorder := &deadlineRecordingCommandExecutor{}
	api := &SandboxAPI{
		runner: &Runner{cfg: cfg, compiler: recorder},
		cfg:    cfg,
	}

	api.ExecuteContext(context.Background(), Request{
		Language:   "go",
		SourceCode: "package main\nfunc main() {}",
	})

	if recorder.remaining < 235*time.Second {
		t.Fatalf("Go compiler context remaining = %v, want at least 235s", recorder.remaining)
	}
}

func TestUnaryWallClockLimitRemainsServerBounded(t *testing.T) {
	if got := requestWallClockLimit(10*time.Minute, 30*time.Second); got != 5*time.Minute {
		t.Fatalf("unary wall clock limit = %v, want 5m", got)
	}
}

func TestExecuteContextPropagatesCancellationToExecutor(t *testing.T) {
	cfg := Config{
		HostTempDir:               t.TempDir(),
		DefaultCompileTimeLimit:   time.Second,
		DefaultExecuteTimeLimit:   time.Second,
		DefaultExecuteMemoryLimit: 64 << 20,
		Languages: map[string]LanguageConfig{
			"test": {
				Compile: CompileConfig{SrcName: "main.src"},
				Run:     RunConfig{Command: "test-runner"},
			},
		},
	}
	executor := &contextWaitingCommandExecutor{
		started: make(chan struct{}),
		done:    make(chan struct{}),
	}
	api := &SandboxAPI{
		runner: &Runner{cfg: cfg, executor: executor},
		cfg:    cfg,
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan Response, 1)
	go func() {
		result <- api.ExecuteContext(ctx, Request{Language: "test", SourceCode: "source"})
	}()
	<-executor.started
	cancel()
	response := <-result

	if !errors.Is(executor.err, context.Canceled) {
		t.Fatalf("executor context error = %v, want %v", executor.err, context.Canceled)
	}
	if response.Status != string(StatusSandboxError) {
		t.Fatalf("response status = %q, want %q", response.Status, StatusSandboxError)
	}
}

type contextWaitingCommandExecutor struct {
	started chan struct{}
	done    chan struct{}
	err     error
}

type deadlineRecordingCommandExecutor struct {
	remaining time.Duration
}

func (executor *deadlineRecordingCommandExecutor) Execute(
	ctx context.Context,
	_ []string,
	_ map[string]string,
	_ *string,
) Result {
	deadline, ok := ctx.Deadline()
	if !ok {
		return NewResult(StatusSandboxError, errors.New("compiler context has no deadline"))
	}
	executor.remaining = time.Until(deadline)
	return NewResult(StatusSandboxError, errors.New("stop after recording deadline"))
}

func (executor *contextWaitingCommandExecutor) Execute(
	ctx context.Context,
	_ []string,
	_ map[string]string,
	_ *string,
) Result {
	close(executor.started)
	<-ctx.Done()
	executor.err = ctx.Err()
	close(executor.done)
	return NewResult(StatusSandboxError, ctx.Err())
}
