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

// Executor handles running a pre-compiled binary directly on the host machine.
type Executor struct {
	cfg Config
}

// NewExecutor creates a new Executor instance.
func NewExecutor(cfg Config) *Executor {
	return &Executor{cfg: cfg}
}

// Execute runs the binary located at hostBinaryPath directly on the host.
// It applies time and output limits.
func (e *Executor) Execute(ctx context.Context, hostBinaryPath string, stdinData *string) Result {
	execCtx, cancel := context.WithTimeout(ctx, e.cfg.ExecTimeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, hostBinaryPath)

	// Prepare stdin pipe if data is provided
	if stdinData != nil {
		stdinPipe, err := cmd.StdinPipe()
		if err != nil {
			return NewResult(StatusSandboxError, fmt.Errorf("failed to get stdin pipe: %w", err))
		}
		// Start a goroutine to write stdin data and close the pipe
		go func() {
			defer stdinPipe.Close()
			_, err := io.WriteString(stdinPipe, *stdinData)
			if err != nil {
				log.Printf("Error writing to stdin pipe: %v", err)
			}
		}()
	}

	// Buffers to capture stdout and stderr with limits
	var stdoutBuf, stderrBuf bytes.Buffer
	stdoutWriter := NewLimitedWriter(&stdoutBuf, e.cfg.MaxStdoutSize)
	stderrWriter := NewLimitedWriter(&stderrBuf, e.cfg.MaxStderrSize)
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter

	startTime := time.Now()
	log.Printf("Executing on host: %s", hostBinaryPath)

	runErr := cmd.Run() // This blocks until completion or context cancellation

	duration := time.Since(startTime)
	timeUsedMillis := duration.Milliseconds() // Approximate time used

	stdoutStr := stdoutBuf.String()
	stderrStr := stderrBuf.String()

	result := Result{ // Start building the result
		Stdout:         stdoutStr,
		Stderr:         stderrStr,
		TimeUsedMillis: timeUsedMillis,
		MemoryUsedKB:   -1, // Not measured in v0.1
		ExitCode:       cmd.ProcessState.ExitCode(), // Get exit code directly
	}


	// --- Determine Final Status ---

	// 1. Check for Execution Timeout (context error)
	if errors.Is(execCtx.Err(), context.DeadlineExceeded) {
		log.Printf("Execution timeout after %v", duration)
		result.Status = StatusTimeLimitExceeded
		result.Error = ErrExecuteTimeout.Error()
		result.TimeUsedMillis = e.cfg.ExecTimeout.Milliseconds() // Report max on TLE
		result.ExitCode = -1 // Exit code is unreliable on timeout kill
        // Kill the process forcefully if context cancellation didn't already
        if cmd.Process != nil {
           cmd.Process.Kill()
        }
		return result // Return immediately on TLE
	}

	// 2. Check for Output Limit Exceeded
    var outputLimitErr error
	if stdoutWriter.Exceeded {
        outputLimitErr = fmt.Errorf("%w (stdout)", ErrOutputLimitExceeded)
		log.Printf("Stdout limit exceeded (max %d bytes)", e.cfg.MaxStdoutSize)
	}
    if stderrWriter.Exceeded {
        errAppend := fmt.Errorf("%w (stderr)", ErrOutputLimitExceeded)
        if outputLimitErr != nil {
            outputLimitErr = fmt.Errorf("%w; %w", outputLimitErr, errAppend)
        } else {
            outputLimitErr = errAppend
        }
		log.Printf("Stderr limit exceeded (max %d bytes)", e.cfg.MaxStderrSize)
	}
    if outputLimitErr != nil {
        result.Status = StatusOutputLimitExceeded
        result.Error = outputLimitErr.Error()
        // Even if output limited, we might still have a runtime error exit code
        // Let runtime error check override if exit code is non-zero
    }


	// 3. Check for Runtime Errors (based on exit code or signals)
	if runErr != nil {
		// If it wasn't a timeout (checked above), it's likely a runtime error
		log.Printf("Execution failed: %v (Exit Code: %d)", runErr, result.ExitCode)
        // Prioritize Runtime Error status if exit code is non-zero, even if OLE occurred
		if result.ExitCode != 0 {
            result.Status = StatusRuntimeError
            // Include runErr details in the main error field if status wasn't already set by OLE
            if result.Error == "" {
                 result.Error = fmt.Sprintf("Runtime error: %v", runErr)
            } else {
                 result.Error = fmt.Sprintf("%s; Runtime error: %v", result.Error, runErr)
            }
        } else if result.Status == "" { // If exit code is 0 but cmd.Run() errored (less common), mark as Sandbox Error?
            result.Status = StatusSandboxError
            result.Error = fmt.Sprintf("Execution command error despite exit code 0: %v", runErr)
        }

	}


	// 4. Check if Accepted (if no other status was set)
	if result.Status == "" {
        if result.ExitCode == 0 {
            log.Printf("Execution successful in %v", duration)
            result.Status = StatusAccepted
        } else {
            // Should have been caught by runErr != nil check, but as a fallback
            log.Printf("Execution finished with non-zero exit code %d but no run error captured?", result.ExitCode)
            result.Status = StatusRuntimeError
            result.Error = fmt.Sprintf("Exited with code %d", result.ExitCode)
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
func NewLimitedWriter(w io.Writer, limit int64) *LimitedWriter {
	return &LimitedWriter{w: w, limit: limit}
}

func (lw *LimitedWriter) Write(p []byte) (n int, err error) {
	lw.mu.Lock()
	defer lw.mu.Unlock()

	remaining := lw.limit - lw.written
	if remaining <= 0 {
		if !lw.Exceeded {
			lw.Exceeded = true
			// Optionally log here that the limit was hit
			// log.Printf("Output limit %d bytes reached", lw.limit)
		}
		return len(p), nil // Pretend we wrote everything to discard excess
	}

	writeLen := int64(len(p))
	if writeLen > remaining {
		writeLen = remaining
		lw.Exceeded = true
		// Optionally log here
		// log.Printf("Output limit %d bytes reached", lw.limit)
	}

	n, err = lw.w.Write(p[:writeLen])
	lw.written += int64(n)

	// If the underlying write failed, return that error.
	// Otherwise, if we truncated, return nil error but the Exceeded flag is set.
    // We return len(p) if we truncated to signal upstream io.Copy to stop early if needed,
    // though cmd.Run doesn't directly use io.Copy here.
    if err == nil && lw.Exceeded {
        return len(p), nil
    }

	return n, err
}