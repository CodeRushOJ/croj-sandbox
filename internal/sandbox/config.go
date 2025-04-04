// internal/sandbox/config.go
package sandbox

import (
    "time"
)

// Command template placeholders
const (
	PlaceholderSrcPath  = "{{SRC_PATH}}"  // Source code file path
	PlaceholderExePath  = "{{EXE_PATH}}"  // Executable/output file path
	PlaceholderWorkDir  = "{{WORK_DIR}}"  // Working directory path
	PlaceholderExeDir   = "{{EXE_DIR}}"   // Directory containing the executable
	PlaceholderMaxMemory = "{{MAX_MEM}}"  // Maximum memory in KB
)

const (
	// --- Default Execution Limits ---
	DefaultCompileTimeLimitSec = 10 // Default compile timeout in seconds
	DefaultExecuteTimeLimitSec = 3  // Default execution timeout in seconds
	DefaultMaxStdoutKB         = 64 // Default max stdout size in KB
	DefaultMaxStderrKB         = 64 // Default max stderr size in KB
	DefaultMemoryLimitMB       = 512 // Default memory limit in MB

	// --- Host Environment ---
	DefaultHostTempDir = "/tmp/croj-sandbox-local-runs" // Default host temp directory
)

// CompileConfig defines how to compile a language source file
type CompileConfig struct {
	SrcName       string `json:"srcName"`       // Source file name (e.g., "main.go")
	ExeName       string `json:"exeName"`       // Output executable name
	CompileCommand string `json:"command"`      // Compile command template
	TimeoutSec    int    `json:"timeoutSec"`   // Compile timeout in seconds (0 = use default)
}

// RunConfig defines how to run a compiled or interpreted language
type RunConfig struct {
	Command    string            `json:"command"`    // Run command template
	Env        map[string]string `json:"env"`        // Environment variables
	TimeoutSec int               `json:"timeoutSec"` // Execution timeout in seconds (0 = use default)
	MemoryMB   int               `json:"memoryMB"`   // Memory limit in MB (0 = use default)
}

// LanguageConfig holds configuration for a specific programming language
type LanguageConfig struct {
	Compile CompileConfig `json:"compile"` // Compilation settings
	Run     RunConfig     `json:"run"`     // Execution settings
}

// GetCompileTimeout returns the compile timeout, using default if not set
func (lc *LanguageConfig) GetCompileTimeout(defaultTimeout time.Duration) time.Duration {
	if lc.Compile.TimeoutSec <= 0 {
		return defaultTimeout
	}
	return time.Duration(lc.Compile.TimeoutSec) * time.Second
}

// GetExecuteTimeout returns the execution timeout, using default if not set
// userSpecified 参数表示用户是否指定了自定义超时
func (lc *LanguageConfig) GetExecuteTimeout(defaultTimeout time.Duration, userSpecified ...bool) time.Duration {
	// 如果用户指定了超时并且第一个布尔参数为true，则优先使用用户指定的值
	if len(userSpecified) > 0 && userSpecified[0] {
		return defaultTimeout
	}
	
	// 否则检查语言配置
	if lc.Run.TimeoutSec <= 0 {
		return defaultTimeout
	}
	return time.Duration(lc.Run.TimeoutSec) * time.Second
}

// GetMemoryLimit returns the memory limit in bytes, using default if not set
func (lc *LanguageConfig) GetMemoryLimit(defaultLimit int64) int64 {
	if lc.Run.MemoryMB <= 0 {
		return defaultLimit
	}
	return int64(lc.Run.MemoryMB) * 1024 * 1024 // Convert to bytes
}

// Config holds the configuration for the sandbox system.
type Config struct {
	// Host Environment
	HostTempDir            string                    `json:"hostTempDir"`
	DefaultCompileTimeLimit time.Duration            `json:"defaultCompileTimeLimit"`
	DefaultExecuteTimeLimit time.Duration            `json:"defaultExecuteTimeLimit"`
	DefaultExecuteMemoryLimit int64                  `json:"defaultExecuteMemoryLimit"`
	MaxStdoutSize          int64                     `json:"maxStdoutSize"`
	MaxStderrSize          int64                     `json:"maxStderrSize"`
	Languages              map[string]LanguageConfig `json:"languages"`
	
	// 保留旧的字段名称以兼容API
	CompileTimeout         time.Duration  // 兼容字段
	ExecTimeout            time.Duration  // 兼容字段
	SrcFileName            string         // 兼容字段

	// 是否使用用户指定的超时（优先级高于语言配置）
	UserSpecifiedTimeout bool

	// 安全相关设置
	Language           string // 执行的编程语言
	StrictSecurity     bool   // 使用严格的安全限制
	NoSecurity         bool   // 完全禁用安全限制
	DisableNetworking  bool   // 禁用网络访问
	DisableFileWrite   bool   // 禁用文件写入（只读模式）
	AllowedPaths       []string // 允许访问的路径列表
	SeccompProfile     string // 自定义seccomp配置文件路径
}

// DefaultConfig returns a new Config struct with default values and language settings.
func DefaultConfig() Config {
	cfg := Config{
		HostTempDir:            DefaultHostTempDir,
		DefaultCompileTimeLimit: time.Duration(DefaultCompileTimeLimitSec) * time.Second,
		DefaultExecuteTimeLimit: time.Duration(DefaultExecuteTimeLimitSec) * time.Second,
		DefaultExecuteMemoryLimit: int64(DefaultMemoryLimitMB) * 1024 * 1024,
		MaxStdoutSize:          int64(DefaultMaxStdoutKB) * 1024,
		MaxStderrSize:          int64(DefaultMaxStderrKB) * 1024,
		Languages:              make(map[string]LanguageConfig),
		
		// 为了兼容API，保留旧字段值
		CompileTimeout:         time.Duration(DefaultCompileTimeLimitSec) * time.Second,
		ExecTimeout:            time.Duration(DefaultExecuteTimeLimitSec) * time.Second,
		SrcFileName:            "main.go",

		// 默认安全设置
		StrictSecurity:    true,
		NoSecurity:        false,
		DisableNetworking: true,
		DisableFileWrite:  false,
		AllowedPaths:      []string{"/tmp"},
	}

	// 添加所有支持的语言配置
	ConfigureDefaultLanguages(&cfg)

	return cfg
}