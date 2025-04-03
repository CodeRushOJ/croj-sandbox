// internal/sandbox/config.go
package sandbox

import "time"

const (
	// --- Execution Limits (Local Simulation) ---
	DefaultCompileTimeout = 10 * time.Second // Timeout for local 'go build'
	DefaultExecTimeout    = 3 * time.Second  // Timeout for the user program execution
	DefaultMaxStdoutKB    = 64               // Max stdout size in Kilobytes
	DefaultMaxStderrKB    = 64               // Max stderr size in Kilobytes

	// --- Host Environment ---
	DefaultHostTempDir = "/tmp/croj-sandbox-local-runs" // Host directory for temp files
	DefaultSrcFileName = "main.go"                    // Temporary source file name on host
)

// Config holds the configuration for the local sandbox simulation.
type Config struct {
	// Execution Limits
	CompileTimeout time.Duration // Timeout for the compilation step
	ExecTimeout    time.Duration // Timeout for the execution step
	MaxStdoutSize  int64         // Maximum stdout size in bytes
	MaxStderrSize  int64         // Maximum stderr size in bytes

	// Host Environment
	HostTempDir string // Temporary directory on the host machine
	SrcFileName string // Temporary source file name on host during compilation
}

// DefaultConfig returns a new Config struct with default values.
func DefaultConfig() Config {
	return Config{
		CompileTimeout: DefaultCompileTimeout,
		ExecTimeout:    DefaultExecTimeout,
		MaxStdoutSize:  DefaultMaxStdoutKB * 1024,
		MaxStderrSize:  DefaultMaxStderrKB * 1024,
		HostTempDir:    DefaultHostTempDir,
		SrcFileName:    DefaultSrcFileName,
	}
}