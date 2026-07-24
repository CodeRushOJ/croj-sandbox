package sandbox

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/CodeRushOJ/croj-sandbox/internal/util"
)

// compileArtifact compiles one request through the same fail-closed executor
// used for contestant programs. Compiler processes therefore receive the same
// per-request UID, cgroup, seccomp, network, output, and deadline boundaries.
func (r *Runner) compileArtifact(
	ctx context.Context,
	language string,
	langCfg LanguageConfig,
	cfg Config,
	hostRunDir string,
	sourceFilePath string,
) (string, string, Result) {
	compiledPath := sourceFilePath
	if langCfg.Compile.CompileCommand == "" {
		return compiledPath, "", Result{Status: StatusAccepted, ExitCode: 0}
	}
	if langCfg.Compile.ExeName == "" {
		return "", "", NewResult(
			StatusSandboxError,
			fmt.Errorf("language %q compile config is missing exeName", language),
		)
	}

	executableName := langCfg.Compile.ExeName
	if runtime.GOOS == "windows" && filepath.Ext(executableName) == "" && language != "java" {
		executableName += ".exe"
	}
	compiledPath = filepath.Join(hostRunDir, executableName)
	compileTimeout := langCfg.GetCompileTimeout(cfg.DefaultCompileTimeLimit)
	compileCfg := isolatedCompilerConfig(cfg, language, hostRunDir, compileTimeout)
	compileCommand := util.ProcessCommandString(langCfg.Compile.CompileCommand, map[string]string{
		PlaceholderSrcPath:   sourceFilePath,
		PlaceholderExePath:   compiledPath,
		PlaceholderWorkDir:   hostRunDir,
		PlaceholderExeDir:    filepath.Dir(compiledPath),
		PlaceholderMaxMemory: fmt.Sprintf("%d", compileCfg.DefaultExecuteMemoryLimit/1024),
	})
	if compileCommand == "" {
		return "", "", NewResult(
			StatusSandboxError,
			fmt.Errorf("processed compile command for %q is empty", language),
		)
	}

	compileCtx, cancel := context.WithTimeout(ctx, compileTimeout)
	defer cancel()

	compiler := r.compiler
	if compiler == nil {
		compiler = NewExecutor(compileCfg)
	}

	started := time.Now()
	log.Printf("sandbox event=compile_started language=%s", language)
	compileResult := compiler.Execute(
		compileCtx,
		[]string{"sh", "-c", compileCommand},
		nil,
		nil,
	)
	compileOutput := compileResult.Stdout + compileResult.Stderr
	durationMillis := time.Since(started).Milliseconds()

	switch compileResult.Status {
	case StatusAccepted:
		if _, err := os.Stat(compiledPath); err != nil {
			log.Printf(
				"sandbox event=compile_finished language=%s category=binary_missing duration_ms=%d diagnostic_bytes=%d",
				language,
				durationMillis,
				len(compileOutput),
			)
			result := NewResult(StatusCompileError, fmt.Errorf("%w: %w", ErrBinaryNotFound, err))
			result.CompileOutput = compileOutput
			return "", compileOutput, result
		}
		log.Printf(
			"sandbox event=compile_finished language=%s category=ok duration_ms=%d diagnostic_bytes=%d",
			language,
			durationMillis,
			len(compileOutput),
		)
		return compiledPath, compileOutput, Result{Status: StatusAccepted, ExitCode: 0}
	case StatusSandboxError:
		log.Printf(
			"sandbox event=compile_finished language=%s category=isolation_failed duration_ms=%d diagnostic_bytes=%d",
			language,
			durationMillis,
			len(compileOutput),
		)
		compileResult.CompileOutput = compileOutput
		return "", compileOutput, compileResult
	case StatusTimeLimitExceeded:
		log.Printf(
			"sandbox event=compile_finished language=%s category=timeout duration_ms=%d diagnostic_bytes=%d",
			language,
			durationMillis,
			len(compileOutput),
		)
		result := NewResult(
			StatusCompileError,
			fmt.Errorf("%w (limit: %v)", ErrCompileTimeout, compileTimeout),
		)
		result.CompileOutput = compileOutput
		return "", compileOutput, result
	default:
		log.Printf(
			"sandbox event=compile_finished language=%s category=compile_failed duration_ms=%d diagnostic_bytes=%d",
			language,
			durationMillis,
			len(compileOutput),
		)
		result := NewResult(StatusCompileError, ErrCompileFailed)
		result.CompileOutput = compileOutput
		return "", compileOutput, result
	}
}

func isolatedCompilerConfig(cfg Config, language, workDir string, timeout time.Duration) Config {
	compileCfg := cfg
	compileCfg.Language = language
	compileCfg.WorkingDir = workDir
	compileCfg.DefaultExecuteTimeLimit = timeout
	compileCfg.UserSpecifiedTimeout = true
	compileCfg.DefaultExecuteMemoryLimit = cfg.DefaultCompileMemoryLimit
	if compileCfg.DefaultExecuteMemoryLimit <= 0 {
		compileCfg.DefaultExecuteMemoryLimit = int64(DefaultCompileMemoryLimitMB) * 1024 * 1024
	}
	return compileCfg
}
