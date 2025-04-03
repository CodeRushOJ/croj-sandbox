// internal/sandbox/compiler.go
package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Compiler handles the local compilation of Go source code.
type Compiler struct {
	cfg Config
}

// NewCompiler creates a new Compiler instance.
func NewCompiler(cfg Config) *Compiler {
	return &Compiler{cfg: cfg}
}

// Compile the given Go source code locally on the host machine.
// hostRunDir is the temporary directory for this run.
// Returns the path to the compiled binary, compiler output (stderr), and error.
func (c *Compiler) Compile(ctx context.Context, sourceCode, hostRunDir string) (binaryPath string, compileOutput string, err error) {
	// 1. Prepare paths
	srcFilePath := filepath.Join(hostRunDir, c.cfg.SrcFileName)
	// Use a predictable name based on SrcFileName, e.g., main.go -> main
	// Add .exe suffix on Windows hosts if needed, though target is usually Linux later
	binaryName := strings.TrimSuffix(c.cfg.SrcFileName, filepath.Ext(c.cfg.SrcFileName))
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath = filepath.Join(hostRunDir, binaryName)

	// 2. Write source code to temporary file
	if err := os.WriteFile(srcFilePath, []byte(sourceCode), 0644); err != nil {
		return "", "", fmt.Errorf("failed to write source code to %s: %w", srcFilePath, err)
	}

	// 3. Prepare compilation command
	// Use local 'go build'. No need for cross-compilation args in v0.1.
	// Strip debug info for smaller binary size.
	cmdArgs := []string{
		"build",
		"-ldflags", "-s -w",
		"-o", binaryPath,
		srcFilePath,
	}
	compileCtx, cancel := context.WithTimeout(ctx, c.cfg.CompileTimeout)
	defer cancel()

	cmd := exec.CommandContext(compileCtx, "go", cmdArgs...)
	cmd.Dir = hostRunDir // Run build command from the temp dir

	// 4. Execute compilation
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	startTime := time.Now()
	log.Printf("Compiling on host: go %s", strings.Join(cmdArgs, " "))

	err = cmd.Run()
	duration := time.Since(startTime)
	compileOutput = stderr.String() // Capture stderr always

	if err != nil {
		// Check for timeout first
		if errors.Is(compileCtx.Err(), context.DeadlineExceeded) {
			log.Printf("Compile timeout after %v. Stderr: %s", duration, compileOutput)
			return "", compileOutput, fmt.Errorf("%w: %v", ErrCompileTimeout, compileCtx.Err())
		}
		// Other compilation error (e.g., syntax error)
		log.Printf("Compile failed after %v: %v. Stderr: %s", duration, err, compileOutput)
		// Wrap the original error from cmd.Run
		return "", compileOutput, fmt.Errorf("%w: %v", ErrCompileFailed, err)
	}

	// 5. Verify binary exists
	if _, statErr := os.Stat(binaryPath); statErr != nil {
		log.Printf("Compiled binary not found at %s after successful compile command: %v", binaryPath, statErr)
		return "", compileOutput, fmt.Errorf("%w: %w", ErrBinaryNotFound, statErr)
	}

	log.Printf("Compile successful in %v. Binary at: %s", duration, binaryPath)
	return binaryPath, compileOutput, nil
}