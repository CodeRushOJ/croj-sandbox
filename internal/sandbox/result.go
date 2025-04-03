// internal/sandbox/result.go
package sandbox

import "errors"

// Status represents the outcome of a sandbox execution.
type Status string

const (
	StatusAccepted            Status = "Accepted"              // Code executed successfully within time/output limits.
	StatusCompileError        Status = "Compile Error"         // Code failed to compile locally.
	StatusRuntimeError        Status = "Runtime Error"         // Code compiled but exited with non-zero status locally.
	StatusTimeLimitExceeded   Status = "Time Limit Exceeded"   // Local execution time exceeded the limit.
	StatusMemoryLimitExceeded Status = "Memory Limit Exceeded" // Placeholder - Cannot be reliably enforced/detected locally in v0.1.
	StatusOutputLimitExceeded Status = "Output Limit Exceeded" // Stdout or Stderr exceeded the size limit.
	StatusSandboxError        Status = "Sandbox Error"         // Internal error within the sandbox system (e.g., file ops).
	StatusUnknown             Status = "Unknown"               // Unknown status.
)

// Result holds the outcome of a code execution in the sandbox.
type Result struct {
	Status         Status // Final status of the execution.
	ExitCode       int    // Exit code of the user's program (-1 if not run or error before exec).
	Stdout         string // Standard output from the user's program execution (potentially truncated).
	Stderr         string // Standard error from the user's program execution (potentially truncated).
	Error          string // Internal sandbox error message OR compile error output.
	TimeUsedMillis int64  // Time taken by the user's program execution in milliseconds (-1 if not run or TLE).
	MemoryUsedKB   int64  // Memory usage in Kilobytes (-1 in v0.1 - not measured locally).

	// Compile specific info
	CompileOutput string // Full output from the compilation phase (stderr).
}

// IsOK checks if the result status indicates successful compilation and execution within limits.
func (r *Result) IsOK() bool {
	return r.Status == StatusAccepted
}

// NewResult creates a basic result with a given status and potential error.
func NewResult(status Status, err error) Result {
	res := Result{
		Status:         status,
		ExitCode:       -1, // Default
		TimeUsedMillis: -1,
		MemoryUsedKB:   -1, // Mark as not measured
	}
	if err != nil {
		res.Error = err.Error() // Store general error here
	}
	return res
}

// Predefined sandbox errors (can be expanded)
var (
	ErrCompileTimeout    = errors.New("local compilation timed out")
	ErrCompileFailed     = errors.New("local compilation failed")
	ErrExecuteTimeout    = errors.New("local execution timed out")
	ErrHostTempDir       = errors.New("failed to manage host temporary directory")
	ErrBinaryNotFound    = errors.New("compiled binary not found")
	ErrOutputLimitExceeded = errors.New("output limit exceeded")
)