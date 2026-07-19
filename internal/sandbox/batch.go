package sandbox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/CodeRushOJ/croj-sandbox/internal/util"
)

// BatchCase is one ordered execution against a request-local compiled artifact.
type BatchCase struct {
	ID                  string
	Stdin               *string
	ExpectedOutput      *string
	ExpectedTokenSHA256 *string
}

// BatchCaseResult associates a sandbox result with its opaque caller-provided ID.
type BatchCaseResult struct {
	ID string
	Result
}

// RunBatchWithConfig compiles sourceCode once, executes cases sequentially, and
// deletes the request-local artifact before returning.
func (r *Runner) RunBatchWithConfig(
	ctx context.Context,
	language string,
	sourceCode string,
	cases []BatchCase,
	stopOnFailure bool,
	cfg Config,
	emit func(BatchCaseResult) error,
) Result {
	langCfg, ok := cfg.Languages[language]
	if !ok {
		log.Printf("sandbox event=request_rejected category=unsupported_language")
		return NewResult(StatusSandboxError, fmt.Errorf("language configuration for %q not found", language))
	}
	if len(cases) == 0 {
		return NewResult(StatusSandboxError, errors.New("batch cases are required"))
	}
	if emit == nil {
		return NewResult(StatusSandboxError, errors.New("batch result emitter is required"))
	}

	hostRunDir, cleanup, err := util.SetupHostRunDir(cfg.HostTempDir)
	if err != nil {
		return NewResult(StatusSandboxError, fmt.Errorf("%w: %w", ErrHostTempDir, err))
	}
	defer cleanup()

	sourceFilePath := filepath.Join(hostRunDir, langCfg.Compile.SrcName)
	if langCfg.Compile.SrcName == "" {
		return NewResult(StatusSandboxError, fmt.Errorf("language %q compile config is missing srcName", language))
	}
	if err := os.WriteFile(sourceFilePath, []byte(sourceCode), 0o600); err != nil {
		return NewResult(StatusSandboxError, fmt.Errorf("stage source: %w", err))
	}

	compiledPath, compileOutput, compileResult := compileBatchArtifact(ctx, language, langCfg, cfg, hostRunDir, sourceFilePath)
	if compileResult.Status != StatusAccepted {
		compileResult.CompileOutput = compileOutput
		return compileResult
	}

	memLimitBytes := langCfg.GetMemoryLimit(cfg.DefaultExecuteMemoryLimit)
	timeoutDuration := langCfg.GetExecuteTimeout(cfg.DefaultExecuteTimeLimit, cfg.UserSpecifiedTimeout)
	runPlaceholders := map[string]string{
		PlaceholderExePath:   compiledPath,
		PlaceholderWorkDir:   hostRunDir,
		PlaceholderSrcPath:   sourceFilePath,
		PlaceholderExeDir:    filepath.Dir(compiledPath),
		PlaceholderMaxMemory: fmt.Sprintf("%d", memLimitBytes/1024),
	}
	runCommand, err := util.ProcessCommandTemplate(langCfg.Run.Command, runPlaceholders)
	if err != nil {
		return NewResult(StatusSandboxError, fmt.Errorf("process run command for %q: %w", language, err))
	}

	last := Result{Status: StatusAccepted, ExitCode: 0}
	for index, testCase := range cases {
		if err := ctx.Err(); err != nil {
			return NewResult(StatusSandboxError, err)
		}
		runCfg := cfg
		runCfg.DefaultExecuteTimeLimit = timeoutDuration
		runCfg.DefaultExecuteMemoryLimit = memLimitBytes
		runCfg.Language = language
		executor := r.executor
		if executor == nil {
			executor = NewExecutor(runCfg)
		}
		caseResult := executor.Execute(ctx, runCommand, langCfg.Run.Env, testCase.Stdin)
		if caseResult.Status == StatusAccepted && testCase.ExpectedOutput != nil && !util.CompareOutputs(caseResult.Stdout, *testCase.ExpectedOutput) {
			caseResult.Status = StatusWrongAnswer
			caseResult.Error = ErrOutputMismatch.Error()
		}
		if caseResult.Status == StatusAccepted && testCase.ExpectedTokenSHA256 != nil && tokenOutputSHA256(caseResult.Stdout) != *testCase.ExpectedTokenSHA256 {
			caseResult.Status = StatusWrongAnswer
			caseResult.Error = ErrOutputMismatch.Error()
		}
		log.Printf(
			"sandbox event=batch_case_finished language=%s case_index=%d verdict=%s exit_code=%d time_ms=%d memory_kb=%d",
			language,
			index,
			caseResult.Status,
			caseResult.ExitCode,
			caseResult.TimeUsedMillis,
			caseResult.MemoryUsedKB,
		)
		if err := emit(BatchCaseResult{ID: testCase.ID, Result: caseResult}); err != nil {
			return NewResult(StatusSandboxError, fmt.Errorf("emit batch case result: %w", err))
		}
		last = caseResult
		if stopOnFailure && caseResult.Status != StatusAccepted {
			return last
		}
	}
	return last
}

func tokenOutputSHA256(output string) string {
	hasher := sha256.New()
	var length [8]byte
	for _, token := range strings.Fields(output) {
		binary.BigEndian.PutUint64(length[:], uint64(len(token)))
		_, _ = hasher.Write(length[:])
		_, _ = hasher.Write([]byte(token))
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func compileBatchArtifact(
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
		return "", "", NewResult(StatusSandboxError, fmt.Errorf("language %q compile config is missing exeName", language))
	}
	executableName := langCfg.Compile.ExeName
	if runtime.GOOS == "windows" && filepath.Ext(executableName) == "" && language != "java" {
		executableName += ".exe"
	}
	compiledPath = filepath.Join(hostRunDir, executableName)
	compileCommand := util.ProcessCommandString(langCfg.Compile.CompileCommand, map[string]string{
		PlaceholderSrcPath: sourceFilePath,
		PlaceholderExePath: compiledPath,
		PlaceholderWorkDir: hostRunDir,
		PlaceholderExeDir:  filepath.Dir(compiledPath),
	})
	if compileCommand == "" {
		return "", "", NewResult(StatusSandboxError, fmt.Errorf("processed compile command for %q is empty", language))
	}
	compileTimeout := langCfg.GetCompileTimeout(cfg.DefaultCompileTimeLimit)
	compileCtx, cancel := context.WithTimeout(ctx, compileTimeout)
	defer cancel()
	// #nosec G204 -- commands come from administrator-owned language configuration.
	command := exec.CommandContext(compileCtx, "sh", "-c", compileCommand)
	command.Dir = hostRunDir
	var stdout, stderr bytes.Buffer
	stdoutLimit, stderrLimit := cfg.MaxStdoutSize, cfg.MaxStderrSize
	if stdoutLimit <= 0 {
		stdoutLimit = int64(DefaultMaxStdoutKB) * 1024
	}
	if stderrLimit <= 0 {
		stderrLimit = int64(DefaultMaxStderrKB) * 1024
	}
	command.Stdout = NewLimitedWriter(&stdout, stdoutLimit)
	command.Stderr = NewLimitedWriter(&stderr, stderrLimit)
	started := time.Now()
	log.Printf("sandbox event=batch_compile_started language=%s", language)
	err := command.Run()
	compileOutput := stdout.String() + stderr.String()
	durationMillis := time.Since(started).Milliseconds()
	if err != nil {
		if errors.Is(compileCtx.Err(), context.DeadlineExceeded) {
			log.Printf("sandbox event=batch_compile_finished language=%s category=timeout duration_ms=%d diagnostic_bytes=%d", language, durationMillis, len(compileOutput))
			return "", compileOutput, NewResult(StatusCompileError, fmt.Errorf("%w (limit: %v)", ErrCompileTimeout, compileTimeout))
		}
		log.Printf("sandbox event=batch_compile_finished language=%s category=compile_failed duration_ms=%d diagnostic_bytes=%d", language, durationMillis, len(compileOutput))
		return "", compileOutput, NewResult(StatusCompileError, ErrCompileFailed)
	}
	if _, err := os.Stat(compiledPath); err != nil {
		log.Printf("sandbox event=batch_compile_finished language=%s category=binary_missing duration_ms=%d diagnostic_bytes=%d", language, durationMillis, len(compileOutput))
		return "", compileOutput, NewResult(StatusCompileError, fmt.Errorf("%w: %w", ErrBinaryNotFound, err))
	}
	log.Printf("sandbox event=batch_compile_finished language=%s category=ok duration_ms=%d diagnostic_bytes=%d", language, durationMillis, len(compileOutput))
	return compiledPath, compileOutput, Result{Status: StatusAccepted, ExitCode: 0}
}
