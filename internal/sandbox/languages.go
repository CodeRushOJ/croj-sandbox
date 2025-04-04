// internal/sandbox/languages.go
package sandbox

import (
	"os"
	"path/filepath"
)

// Default environment variables, similar to go-judge
var defaultEnv = []string{"LANG=en_US.UTF-8", "LANGUAGE=en_US:en", "LC_ALL=en_US.UTF-8"}

// ConfigureDefaultLanguages adds default language configurations to the given config
func ConfigureDefaultLanguages(cfg *Config) {
	// Go
	cfg.Languages["go"] = LanguageConfig{
		Compile: CompileConfig{
			SrcName:        "main.go",
			ExeName:        "main", 
			CompileCommand: "go build -ldflags \"-s -w\" -o {{EXE_PATH}} {{SRC_PATH}}",
			TimeoutSec:     DefaultCompileTimeLimitSec,
		},
		Run: RunConfig{
			Command:    "{{EXE_PATH}}",
			Env:        make(map[string]string),
			TimeoutSec: DefaultExecuteTimeLimitSec,
			MemoryMB:   DefaultMemoryLimitMB,
		},
	}

	// C++
	cfg.Languages["cpp"] = LanguageConfig{
		Compile: CompileConfig{
			SrcName:        "main.cpp",
			ExeName:        "main",
			CompileCommand: "g++ -Wall -O2 -std=c++17 {{SRC_PATH}} -o {{EXE_PATH}}",
			TimeoutSec:     DefaultCompileTimeLimitSec,
		},
		Run: RunConfig{
			Command:    "{{EXE_PATH}}",
			Env:        make(map[string]string),
			TimeoutSec: DefaultExecuteTimeLimitSec,
			MemoryMB:   DefaultMemoryLimitMB,
		},
	}

	// Python 3
	cfg.Languages["python"] = LanguageConfig{
		Compile: CompileConfig{
			SrcName:        "main.py",
			ExeName:        "main.py", // 不编译，直接运行
		},
		Run: RunConfig{
			Command:    "python3 {{SRC_PATH}}",
			Env:        make(map[string]string),
			TimeoutSec: DefaultExecuteTimeLimitSec,
			MemoryMB:   DefaultMemoryLimitMB,
		},
	}

	// Java
	cfg.Languages["java"] = LanguageConfig{
		Compile: CompileConfig{
			SrcName:        "Main.java",
			ExeName:        "Main.class", 
			CompileCommand: "javac {{SRC_PATH}}",
			TimeoutSec:     DefaultCompileTimeLimitSec,
		},
		Run: RunConfig{
			Command:    "java -cp {{EXE_DIR}} Main",
			Env:        make(map[string]string),
			TimeoutSec: DefaultExecuteTimeLimitSec,
			MemoryMB:   DefaultMemoryLimitMB,
		},
	}

	// JavaScript (Node.js)
	cfg.Languages["javascript"] = LanguageConfig{
		Compile: CompileConfig{
			SrcName:        "main.js",
			ExeName:        "main.js", // 不编译，直接运行
		},
		Run: RunConfig{
			Command:    "node {{SRC_PATH}}",
			Env:        make(map[string]string),
			TimeoutSec: DefaultExecuteTimeLimitSec,
			MemoryMB:   DefaultMemoryLimitMB,
		},
	}

	// 更多语言可以按需添加
}

// GetGoPath 获取GOPATH环境变量
func GetGoPath() string {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			gopath = filepath.Join(homeDir, "go")
		}
	}
	return gopath
}