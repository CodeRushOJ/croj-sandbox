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
	SourceCode     string  `json:"sourceCode"`     // Source code to execute
	Language       string  `json:"language"`       // Programming language (default: "go")
	Stdin          *string `json:"stdin"`          // Optional standard input
	Timeout        *int    `json:"timeout"`        // Optional custom timeout in seconds
	ExpectedOutput *string `json:"expectedOutput"` // Optional expected output for comparison
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

// Execute runs the provided code and returns the result
func (api *SandboxAPI) Execute(req Request) Response {
	// Set default language if not specified
	language := req.Language
	if language == "" {
		language = "go" // 默认使用Go语言
	}
	
	// Apply custom timeout if provided
	ctx := context.Background()
	var execTimeout time.Duration
	
	// 检查语言配置是否存在
	if langConfig, ok := api.cfg.Languages[language]; ok {
		execTimeout = langConfig.GetExecuteTimeout(api.cfg.DefaultExecuteTimeLimit)
	} else {
		// 回退到兼容字段
		execTimeout = api.cfg.ExecTimeout
	}
	
	// 应用自定义超时（如果提供）
	if req.Timeout != nil && *req.Timeout > 0 {
		customTimeout := time.Duration(*req.Timeout) * time.Second
		// 不超过合理限制
		if customTimeout <= 30*time.Second {
			execTimeout = customTimeout
		}
	}
	
	// Create context with timeout (估计编译时间+执行时间+额外缓冲)
	compileTimeout := api.cfg.DefaultCompileTimeLimit
	if api.cfg.CompileTimeout > 0 {
		compileTimeout = api.cfg.CompileTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, compileTimeout+execTimeout+5*time.Second)
	defer cancel()
	
	// 运行代码
	result := api.runner.Run(ctx, language, req.SourceCode, req.Stdin, req.ExpectedOutput)
	
	// 转换为API响应
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
