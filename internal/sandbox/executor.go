// internal/sandbox/executor.go
package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
	"time"
)

// Executor handles executing commands with appropriate resource limits.
type Executor struct {
	cfg Config
}

// NewExecutor creates a new executor instance.
func NewExecutor(cfg Config) *Executor {
	return &Executor{cfg: cfg}
}

// Execute runs the provided command with resource constraints.
// runCmd: Command and arguments to execute (already processed for placeholders)
// env: Optional environment variables
// stdinData: Optional standard input data
func (e *Executor) Execute(ctx context.Context, runCmd []string, env map[string]string, stdinData *string) Result {
	if len(runCmd) == 0 {
		return NewResult(StatusSandboxError, fmt.Errorf("empty command provided to executor"))
	}

	log.Printf("Executing: %v", runCmd)
	execCmd := exec.CommandContext(ctx, runCmd[0], runCmd[1:]...)

	// Set environment variables if provided
	if len(env) > 0 {
		execEnv := execCmd.Environ() // Start with current environment
		for k, v := range env {
			execEnv = append(execEnv, fmt.Sprintf("%s=%s", k, v))
		}
		execCmd.Env = execEnv
	}

	// Setup stdin if provided
	if stdinData != nil {
		stdinPipe, err := execCmd.StdinPipe()
		if err != nil {
			return NewResult(StatusSandboxError, fmt.Errorf("failed to get stdin pipe: %w", err))
		}
		go func() {
			defer stdinPipe.Close()
			_, err := io.WriteString(stdinPipe, *stdinData)
			if err != nil {
				log.Printf("Error writing to stdin pipe: %v", err)
			}
		}()
	}

	// Setup stdout/stderr with limits
	var stdoutBuf, stderrBuf bytes.Buffer
	// 修复此处：使用 MaxStdoutSize 而不是 DefaultMaxStdoutSize
	stdoutWriter := NewLimitedWriter(&stdoutBuf, e.cfg.MaxStdoutSize)
	// 修复此处：使用 MaxStderrSize 而不是 DefaultMaxStderrSize
	stderrWriter := NewLimitedWriter(&stderrBuf, e.cfg.MaxStderrSize)
	execCmd.Stdout = stdoutWriter
	execCmd.Stderr = stderrWriter

	// Execute the command
	startTime := time.Now()
	runErr := execCmd.Run()
	duration := time.Since(startTime)

	// Build the result
	result := Result{
		ExitCode:       0, // Will be set below if available
		TimeUsedMillis: duration.Milliseconds(),
		MemoryUsedKB:   -1, // Not measured in v0.1
		Stdout:         stdoutBuf.String(),
		Stderr:         stderrBuf.String(),
	}

	// Determine status based on various conditions
	// 1. Check for context timeout
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		result.Status = StatusTimeLimitExceeded
		result.Error = ErrExecuteTimeout.Error()
		result.ExitCode = -1 // Invalid on timeout
		return result
	}

	// 2. Check for output limit exceeded
	var outputLimitErr error
	if stdoutWriter.(*LimitedWriter).Exceeded {
		// 修复此处：使用 MaxStdoutSize 而不是 DefaultMaxStdoutSize
		outputLimitErr = fmt.Errorf("%w (stdout, limit: %d bytes)", ErrOutputLimitExceeded, e.cfg.MaxStdoutSize)
	}
	if stderrWriter.(*LimitedWriter).Exceeded {
		// 修复此处：使用 MaxStderrSize 而不是 DefaultMaxStderrSize
		errAppend := fmt.Errorf("%w (stderr, limit: %d bytes)", ErrOutputLimitExceeded, e.cfg.MaxStderrSize)
		if outputLimitErr != nil {
			outputLimitErr = fmt.Errorf("%v; %v", outputLimitErr, errAppend)
		} else {
			outputLimitErr = errAppend
		}
	}

	if outputLimitErr != nil {
		result.Status = StatusOutputLimitExceeded
		result.Error = outputLimitErr.Error()
	}

	// 3. Check run errors and exit code
	if execCmd.ProcessState != nil {
		result.ExitCode = execCmd.ProcessState.ExitCode()
	}

	if runErr != nil && result.Status == "" {
		result.Status = StatusRuntimeError
		result.Error = fmt.Sprintf("Runtime error: %v (exit code: %d)", runErr, result.ExitCode)
	}

	// 4. Set as Accepted if no other status was determined
	if result.Status == "" {
		if result.ExitCode == 0 {
			result.Status = StatusAccepted
		} else {
			result.Status = StatusRuntimeError
			result.Error = fmt.Sprintf("Process exited with code %d", result.ExitCode)
		}
	}

	return result
}

// --- LimitedWriter ---

// LimitedWriter wraps an io.Writer but stops writing after a certain limit.
type LimitedWriter struct {
	w        io.Writer
	limit    int64
	written  int64
	mu       sync.Mutex
	Exceeded bool // Flag to indicate if the limit was reached
}

// NewLimitedWriter creates a new LimitedWriter.
func NewLimitedWriter(w io.Writer, limit int64) io.Writer {
	return &LimitedWriter{w: w, limit: limit}
}

func (lw *LimitedWriter) Write(p []byte) (n int, err error) {
	lw.mu.Lock()
	defer lw.mu.Unlock()

	remaining := lw.limit - lw.written
	if remaining <= 0 {
		if !lw.Exceeded {
			lw.Exceeded = true
		}
		return len(p), nil // Pretend we wrote everything
	}

	writeLen := int64(len(p))
	if writeLen > remaining {
		writeLen = remaining
		lw.Exceeded = true
	}

	n, err = lw.w.Write(p[:writeLen])
	lw.written += int64(n)

	if err == nil && lw.Exceeded {
		return len(p), nil
	}

	return n, err
}