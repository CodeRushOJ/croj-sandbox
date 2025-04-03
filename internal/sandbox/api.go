// internal/sandbox/api.go
package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// Request represents a code execution request
type Request struct {
	SourceCode string  `json:"sourceCode"` // Go source code to execute
	Stdin      *string `json:"stdin"`      // Optional standard input
	Timeout    *int    `json:"timeout"`    // Optional custom timeout in seconds
}

// Response represents the execution result
type Response struct {
	Status       string `json:"status"`       // Execution status (e.g., "Accepted", "Runtime Error")
	ExitCode     int    `json:"exitCode"`     // Process exit code
	Stdout       string `json:"stdout"`       // Standard output content
	Stderr       string `json:"stderr"`       // Standard error content
	Error        string `json:"error"`        // Error message if any
	TimeUsed     int64  `json:"timeUsed"`     // Execution time in milliseconds
	CompileError string `json:"compileError"` // Compilation error if any
}

// SandboxAPI provides a simple API for the code execution sandbox
type SandboxAPI struct {
	runner *Runner
	cfg    Config
}

// NewSandboxAPI creates a new sandbox API instance with default configuration
func NewSandboxAPI() (*SandboxAPI, error) {
	return NewSandboxAPIWithConfig(DefaultConfig())
}

// NewSandboxAPIWithConfig creates a new sandbox API with custom configuration
func NewSandboxAPIWithConfig(cfg Config) (*SandboxAPI, error) {
	runner, err := NewRunner(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize sandbox runner: %w", err)
	}
	
	return &SandboxAPI{
		runner: runner,
		cfg:    cfg,
	}, nil
}

// Execute runs the provided Go code and returns the result
func (api *SandboxAPI) Execute(req Request) Response {
	// Apply custom timeout if provided
	ctx := context.Background()
	execTimeout := api.cfg.ExecTimeout
	
	if req.Timeout != nil && *req.Timeout > 0 {
		customTimeout := time.Duration(*req.Timeout) * time.Second
		// Don't exceed reasonable limits
		if customTimeout <= 30*time.Second {
			execTimeout = customTimeout
		}
	}
	
	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, api.cfg.CompileTimeout+execTimeout+5*time.Second)
	defer cancel()
	
	// Run the code
	result := api.runner.Run(ctx, req.SourceCode, req.Stdin)
	
	// Convert to API response
	response := Response{
		Status:       string(result.Status),
		ExitCode:     result.ExitCode,
		Stdout:       result.Stdout,
		Stderr:       result.Stderr,
		Error:        result.Error,
		TimeUsed:     result.TimeUsedMillis,
		CompileError: result.CompileOutput,
	}
	
	return response
}

// ExecuteJSON accepts a JSON request string and returns a JSON response
func (api *SandboxAPI) ExecuteJSON(jsonRequest string) (string, error) {
	var req Request
	if err := json.Unmarshal([]byte(jsonRequest), &req); err != nil {
		return "", fmt.Errorf("failed to parse request JSON: %w", err)
	}
	
	response := api.Execute(req)
	
	jsonResponse, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("failed to serialize response: %w", err)
	}
	
	return string(jsonResponse), nil
}

// Close releases resources held by the API
func (api *SandboxAPI) Close() error {
	log.Println("Closing sandbox API")
	return api.runner.Close()
}
