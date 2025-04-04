// internal/sandbox/runner.go
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
	"time"

	"github.com/CodeRushOJ/croj-sandbox/internal/util" // Import util which now includes compare
)

// Runner orchestrates the local code compilation and execution simulation using LanguageConfig.
type Runner struct {
	cfg      Config
	executor *Executor
}

// NewRunner creates a new local sandbox runner instance.
func NewRunner(cfg Config) (*Runner, error) {
	if err := util.EnsureDir(cfg.HostTempDir); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrHostTempDir, err)
	}
	executor := NewExecutor(cfg)
	log.Printf("Local sandbox runner initialized: HostTemp='%s'", cfg.HostTempDir)
	return &Runner{
		cfg:      cfg,
		executor: executor,
	}, nil
}

// Run compiles and executes source code for a given language locally using LanguageConfig.
func (r *Runner) Run(ctx context.Context, language, sourceCode string, stdinData *string, expectedOutput *string) Result {
	return r.RunWithConfig(ctx, language, sourceCode, stdinData, expectedOutput, r.cfg)
}

// RunWithConfig 使用自定义配置运行代码
func (r *Runner) RunWithConfig(ctx context.Context, language, sourceCode string, stdinData *string, expectedOutput *string, cfg Config) Result {
	util.DebugLog("Runner: 执行超时设置: %.2f秒, 用户指定: %v", 
		cfg.DefaultExecuteTimeLimit.Seconds(), cfg.UserSpecifiedTimeout)
	
	// 1. Get Language Configuration
	langCfg, ok := cfg.Languages[language]
	if (!ok) {
		err := fmt.Errorf("language configuration for '%s' not found", language)
		log.Printf("%v", err)
		return NewResult(StatusSandboxError, err)
	}

	// 2. Setup temporary directory
	hostRunDir, cleanup, err := util.SetupHostRunDir(cfg.HostTempDir)
	if err != nil {
		util.ErrorLog("创建临时目录失败: %v", err)
		return NewResult(StatusSandboxError, fmt.Errorf("%w: %w", ErrHostTempDir, err))
	}
	defer cleanup()

	// 3. Determine and write source file
	srcFileName := langCfg.Compile.SrcName
	if srcFileName == "" {
		return NewResult(StatusSandboxError, fmt.Errorf("language '%s' CompileConfig missing SrcName", language))
	}
	sourceFilePath := filepath.Join(hostRunDir, srcFileName)
	if err := os.WriteFile(sourceFilePath, []byte(sourceCode), 0644); err != nil {
		log.Printf("Error writing source code to %s: %v", sourceFilePath, err)
		return NewResult(StatusSandboxError, fmt.Errorf("failed to write source file: %w", err))
	}
	log.Printf("[%s] Source code saved to: %s", language, sourceFilePath)

	// --- 4. Compile Step ---
	var compileOutput string
	var compiledExePath string = sourceFilePath
	var compileErr error

	if langCfg.Compile.CompileCommand != "" {
		log.Printf("[%s] Starting compilation phase.", language)
		compileStartTime := time.Now()
		exeName := langCfg.Compile.ExeName
		if exeName == "" {
			return NewResult(StatusSandboxError, fmt.Errorf("language '%s' has CompileCommand but no ExeName", language))
		}
		if runtime.GOOS == "windows" && filepath.Ext(exeName) == "" && language != "java" {
			exeName += ".exe"
		}
		compiledExePath = filepath.Join(hostRunDir, exeName)
		placeholders := map[string]string{
			PlaceholderSrcPath: sourceFilePath, PlaceholderExePath: compiledExePath,
			PlaceholderWorkDir: hostRunDir, PlaceholderExeDir: filepath.Dir(compiledExePath),
		}
		compileCmdStr := util.ProcessCommandString(langCfg.Compile.CompileCommand, placeholders)
		if compileCmdStr == "" {
			return NewResult(StatusSandboxError, fmt.Errorf("processed compile command for '%s' is empty", language))
		}
		compileTimeout := langCfg.GetCompileTimeout(r.cfg.DefaultCompileTimeLimit)
		compileCtx, cancel := context.WithTimeout(ctx, compileTimeout)
		// #nosec G204
		cmd := exec.CommandContext(compileCtx, "sh", "-c", compileCmdStr)
		cmd.Dir = hostRunDir
		var stderr, stdout bytes.Buffer
		cmd.Stderr = &stderr
		cmd.Stdout = &stdout
		log.Printf("[%s] Executing Compile: sh -c \"%s\"", language, compileCmdStr)
		runCompileErr := cmd.Run()
		compileOutput = stdout.String() + stderr.String()
		compileDuration := time.Since(compileStartTime)
		cancel() // Cancel context

		if runCompileErr != nil {
			if errors.Is(compileCtx.Err(), context.DeadlineExceeded) {
				compileErr = fmt.Errorf("%w (limit: %v)", ErrCompileTimeout, compileTimeout)
				log.Printf("[%s] Compile timeout after %v. Output: %s", language, compileDuration, compileOutput)
			} else {
				compileErr = fmt.Errorf("%w: %v", ErrCompileFailed, runCompileErr)
				log.Printf("[%s] Compile failed after %v: %v. Output: %s", language, compileDuration, runCompileErr, compileOutput)
			}
		} else {
			// Verify executable existence (heuristic)
			_, statErr := os.Stat(compiledExePath)
			isCompiledLang := language == "go" || language == "cpp" // Add other compiled languages here
			if statErr != nil && isCompiledLang {
				compileErr = fmt.Errorf("%w '%s': %w", ErrBinaryNotFound, compiledExePath, statErr)
				log.Printf("[%s] %v", language, compileErr)
			} else {
				log.Printf("[%s] Compile successful in %v.", language, compileDuration)
			}
		}
	} else {
		log.Printf("[%s] Skipping compilation phase.", language)
	}

	// Handle Compile Error Result
	if compileErr != nil {
		res := NewResult(StatusCompileError, compileErr)
		res.CompileOutput = compileOutput
		if !errors.Is(compileErr, ErrCompileTimeout) {
			res.Error = compileOutput
		}
		return res
	}

	// --- 5. Execute Step ---
	util.InfoLog("[%s] 开始执行阶段", language)
	memLimitBytes := langCfg.GetMemoryLimit(cfg.DefaultExecuteMemoryLimit)
	memLimitKB := memLimitBytes / 1024
	
	// 从语言配置中获取运行时间限制，但考虑用户是否指定了超时
	timeoutDuration := langCfg.GetExecuteTimeout(cfg.DefaultExecuteTimeLimit, cfg.UserSpecifiedTimeout)
	util.DebugLog("[%s] 设置时间限制: %.2f秒 (用户指定: %v)", 
		language, timeoutDuration.Seconds(), cfg.UserSpecifiedTimeout)
	
	// 处理命令模板
	runPlaceholders := map[string]string{
		PlaceholderExePath: compiledExePath, PlaceholderWorkDir: hostRunDir,
		PlaceholderSrcPath: sourceFilePath, PlaceholderExeDir: filepath.Dir(compiledExePath),
		PlaceholderMaxMemory: fmt.Sprintf("%d", memLimitKB),
	}
	runCmdParts, templateErr := util.ProcessCommandTemplate(langCfg.Run.Command, runPlaceholders)
	if templateErr != nil {
		err := fmt.Errorf("failed to process run command template for '%s': %w", language, templateErr)
		log.Printf("%v", err)
		res := NewResult(StatusSandboxError, err)
		res.CompileOutput = compileOutput
		return res
	}
	
	// 确保超时设置被正确传递到执行器
	runCfg := cfg
	runCfg.DefaultExecuteTimeLimit = timeoutDuration
	util.DebugLog("[%s] 传递到执行器的超时设置: %.2f seconds", language, runCfg.DefaultExecuteTimeLimit.Seconds())
	
	executor := NewExecutor(runCfg)
	execResult := executor.Execute(ctx, runCmdParts, langCfg.Run.Env, stdinData)
	execResult.CompileOutput = compileOutput // Add compile output regardless of exec status

	// --- 6. Output Comparison Step ---
	// Only compare if execution was successful so far (status Accepted) and expected output is provided.
	if execResult.Status == StatusAccepted && expectedOutput != nil {
		log.Printf("[%s] Comparing output...", language)
		match := util.CompareOutputs(execResult.Stdout, *expectedOutput)
		if !match {
			log.Printf("[%s] Output mismatch!", language)
			log.Printf("[%s] Expected: %q", language, util.NormalizeString(*expectedOutput))
			log.Printf("[%s] Actual: %q", language, util.NormalizeString(execResult.Stdout))
			execResult.Status = StatusWrongAnswer
			// Add more detail to the error field
			execResult.Error = ErrOutputMismatch.Error()
		} else {
			log.Printf("[%s] Output matches expected.", language)
			// Status remains Accepted
		}
	} else if execResult.Status == StatusAccepted && expectedOutput == nil {
		log.Printf("[%s] Skipping output comparison (no expected output provided).", language)
	} else if expectedOutput != nil {
		log.Printf("[%s] Skipping output comparison (execution status is %s, not Accepted)", language, execResult.Status)
	}

	util.InfoLog("[%s] 最终执行结果: %s", language, execResult.Status)
	return execResult
}

// Close placeholder
func (r *Runner) Close() error {
	log.Println("Closing sandbox runner (no-op in local version)")
	return nil
}