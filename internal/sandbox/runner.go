// internal/sandbox/runner.go
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/CodeRushOJ/croj-sandbox/internal/util" // Import local util package
)

// Runner orchestrates the local code compilation and execution simulation.
type Runner struct {
	cfg      Config
	compiler *Compiler
	executor *Executor
}

// NewRunner creates a new local sandbox runner instance.
func NewRunner(cfg Config) (*Runner, error) {
	// Ensure HostTempDir exists (moved here from util for single point of check)
	if err := util.EnsureDir(cfg.HostTempDir); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrHostTempDir, err)
	}

	compiler := NewCompiler(cfg)
	executor := NewExecutor(cfg)

	log.Printf("Local sandbox runner initialized: CompileTimeout=%v, ExecTimeout=%v, MaxStdout=%dKB, MaxStderr=%dKB",
		cfg.CompileTimeout, cfg.ExecTimeout, cfg.MaxStdoutSize/1024, cfg.MaxStderrSize/1024)

	return &Runner{
		cfg:      cfg,
		compiler: compiler,
		executor: executor,
	}, nil
}

// Run compiles the source code locally and then executes the binary locally.
// stdinData is optional standard input for the user program.
func (r *Runner) Run(ctx context.Context, sourceCode string, stdinData *string) Result {
	// 1. Setup temporary directory for this run
	hostRunDir, cleanup, err := util.SetupHostRunDir(r.cfg.HostTempDir)
	if err != nil {
		log.Printf("Error setting up host run dir: %v", err)
		return NewResult(StatusSandboxError, fmt.Errorf("%w: %w", ErrHostTempDir, err))
	}
	defer cleanup() // Ensure cleanup happens

	// --- 2. Compile Code Locally ---
	binaryPath, compileOutput, compileErr := r.compiler.Compile(ctx, sourceCode, hostRunDir)

	// Handle compilation result
	if compileErr != nil {
		log.Printf("Compilation failed: %v", compileErr)
		// Check if it was specifically a timeout or general compile error
		status := StatusCompileError
		errToReport := compileErr
		if errors.Is(compileErr, ErrCompileTimeout) {
            // Keep status as CompileError, but use the specific timeout error
            errToReport = ErrCompileTimeout
		}

		res := NewResult(status, errToReport)
		res.CompileOutput = compileOutput
		// For CE, put compiler stderr into the main Error field for easier display
		if status == StatusCompileError && !errors.Is(compileErr, ErrCompileTimeout) {
             res.Error = compileOutput
        }
		return res
	}
	log.Printf("Compilation successful, binary at %s", binaryPath)

	// --- 3. Execute Binary Locally ---
	// Pass the main context 'ctx' which might have an overall deadline,
	// executor uses its own ExecTimeout internally via context derived from this.
	execResult := r.executor.Execute(ctx, binaryPath, stdinData)

	// Add compile output to the final result (if not already CE)
	if execResult.Status != StatusCompileError {
		execResult.CompileOutput = compileOutput
	}

	log.Printf("Execution finished with status: %s", execResult.Status)
	return execResult
}

// Close is a placeholder in v0.1 as there are no persistent resources like Docker client.
func (r *Runner) Close() error {
	log.Println("Closing sandbox runner (no-op in local v0.1)")
	return nil
}