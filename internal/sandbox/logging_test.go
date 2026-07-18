package sandbox

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"

	"github.com/CodeRushOJ/croj-sandbox/internal/util"
)

const (
	sentinelSource   = "SENTINEL_SOURCE_7e9e06"
	sentinelStdin    = "SENTINEL_STDIN_10bd26"
	sentinelExpected = "SENTINEL_EXPECTED_8dfb4f"
	sentinelStdout   = "SENTINEL_STDOUT_1b4cc8"
	sentinelStderr   = "SENTINEL_STDERR_744fe7"
	sentinelCompile  = "SENTINEL_COMPILE_20d12c"
)

type stubCommandExecutor struct {
	result Result
}

func (s stubCommandExecutor) Execute(context.Context, []string, map[string]string, *string) Result {
	return s.result
}

func TestRunnerLogsNeverContainJudgingPayload(t *testing.T) {
	for _, debug := range []bool{false, true} {
		debug := debug
		t.Run(map[bool]string{false: "default", true: "debug"}[debug], func(t *testing.T) {
			logs := captureServiceLogs(t, debug)
			cfg := DefaultConfig()
			cfg.HostTempDir = t.TempDir()
			cfg.Languages = map[string]LanguageConfig{
				"sentinel": {
					Compile: CompileConfig{SrcName: "main.txt"},
					Run:     RunConfig{Command: "sentinel-runner {{SRC_PATH}}"},
				},
			}
			runner := &Runner{
				cfg: cfg,
				executor: stubCommandExecutor{result: Result{
					Status:   StatusAccepted,
					ExitCode: 0,
					Stdout:   sentinelStdout,
					Stderr:   sentinelStderr,
				}},
			}

			stdin := sentinelStdin
			expected := sentinelExpected
			result := runner.RunWithConfig(
				context.Background(),
				"sentinel",
				sentinelSource,
				&stdin,
				&expected,
				cfg,
			)

			if result.Status != StatusWrongAnswer {
				t.Fatalf("status = %q, want %q", result.Status, StatusWrongAnswer)
			}
			if result.Stdout != sentinelStdout || result.Stderr != sentinelStderr {
				t.Fatalf("response payload changed: %#v", result)
			}
			assertNoSentinels(t, logs.String())
		})
	}
}

func TestRunnerCompileDiagnosticsStayInResponseAndOutOfLogs(t *testing.T) {
	for _, debug := range []bool{false, true} {
		debug := debug
		t.Run(map[bool]string{false: "default", true: "debug"}[debug], func(t *testing.T) {
			logs := captureServiceLogs(t, debug)
			cfg := DefaultConfig()
			cfg.HostTempDir = t.TempDir()
			cfg.Languages = map[string]LanguageConfig{
				"sentinel": {
					Compile: CompileConfig{
						SrcName:        "main.txt",
						ExeName:        "main",
						CompileCommand: "cat {{SRC_PATH}} >&2; exit 1",
					},
				},
			}
			stdin := sentinelStdin
			expected := sentinelExpected
			runner := &Runner{cfg: cfg, executor: stubCommandExecutor{}}

			result := runner.RunWithConfig(
				context.Background(),
				"sentinel",
				sentinelSource+"\n"+sentinelCompile,
				&stdin,
				&expected,
				cfg,
			)

			if result.Status != StatusCompileError {
				t.Fatalf("status = %q, want %q", result.Status, StatusCompileError)
			}
			if !strings.Contains(result.CompileOutput, sentinelSource) || !strings.Contains(result.CompileOutput, sentinelCompile) {
				t.Fatalf("compile diagnostics missing from response: %q", result.CompileOutput)
			}
			assertNoSentinels(t, logs.String())
		})
	}
}

func TestLegacyCompilerDiagnosticsStayInResponseAndOutOfLogs(t *testing.T) {
	for _, debug := range []bool{false, true} {
		debug := debug
		t.Run(map[bool]string{false: "default", true: "debug"}[debug], func(t *testing.T) {
			logs := captureServiceLogs(t, debug)
			cfg := DefaultConfig()
			cfg.SrcFileName = "main.go"
			compiler := NewCompiler(cfg)
			source := "package main\nimport _ \"" + sentinelCompile + "\"\n// " + sentinelSource + "\nfunc main() {}\n"

			_, diagnostics, err := compiler.Compile(context.Background(), source, t.TempDir())

			if err == nil {
				t.Fatal("Compile returned nil error for invalid import")
			}
			if !strings.Contains(diagnostics, sentinelCompile) {
				t.Fatalf("compile diagnostics missing from response: %q", diagnostics)
			}
			assertNoSentinels(t, logs.String())
		})
	}
}

func TestExecutorDebugLogsDoNotExpandCommandOrStdin(t *testing.T) {
	for _, debug := range []bool{false, true} {
		debug := debug
		t.Run(map[bool]string{false: "default", true: "debug"}[debug], func(t *testing.T) {
			logs := captureServiceLogs(t, debug)
			cfg := DefaultConfig()
			executor := NewExecutor(cfg)
			stdin := sentinelStdin

			result := executor.Execute(
				context.Background(),
				[]string{sentinelSource, sentinelStdout, sentinelStderr},
				nil,
				&stdin,
			)

			if result.Status != StatusSandboxError {
				t.Fatalf("status = %q, want %q", result.Status, StatusSandboxError)
			}
			assertNoSentinels(t, logs.String())
		})
	}
}

func TestRunnerInitializationDoesNotLogTempRoot(t *testing.T) {
	for _, debug := range []bool{false, true} {
		debug := debug
		t.Run(map[bool]string{false: "default", true: "debug"}[debug], func(t *testing.T) {
			logs := captureServiceLogs(t, debug)
			cfg := DefaultConfig()
			cfg.HostTempDir = t.TempDir() + "/" + sentinelSource

			runner, err := NewRunner(cfg)
			if err != nil {
				t.Fatalf("NewRunner: %v", err)
			}
			if err := runner.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			assertNoSentinels(t, logs.String())
		})
	}
}

func captureServiceLogs(t *testing.T, debug bool) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	oldWriter := log.Writer()
	oldFlags := log.Flags()
	oldDebug := util.DebugMode
	log.SetOutput(&logs)
	log.SetFlags(0)
	if debug {
		t.Setenv("CROJ_DEBUG", "1")
	} else {
		t.Setenv("CROJ_DEBUG", "0")
	}
	util.InitDebugMode()
	t.Cleanup(func() {
		log.SetOutput(oldWriter)
		log.SetFlags(oldFlags)
		util.DebugMode = oldDebug
	})
	return &logs
}

func assertNoSentinels(t *testing.T, output string) {
	t.Helper()
	for _, sentinel := range []string{
		sentinelSource,
		sentinelStdin,
		sentinelExpected,
		sentinelStdout,
		sentinelStderr,
		sentinelCompile,
	} {
		if strings.Contains(output, sentinel) {
			t.Errorf("service logs contain judging payload %q:\n%s", sentinel, output)
		}
	}
}
