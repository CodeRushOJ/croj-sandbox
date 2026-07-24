package sandbox

import (
	"context"
	"errors"
	"testing"
	"time"
)

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
