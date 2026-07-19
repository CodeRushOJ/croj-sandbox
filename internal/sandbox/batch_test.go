package sandbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type batchCommandExecutor struct {
	results []Result
	inputs  []string
}

func (executor *batchCommandExecutor) Execute(_ context.Context, _ []string, _ map[string]string, input *string) Result {
	if input != nil {
		executor.inputs = append(executor.inputs, *input)
	} else {
		executor.inputs = append(executor.inputs, "")
	}
	return executor.results[len(executor.inputs)-1]
}

func TestRunBatchCompilesOnceAndEmitsOrderedCaseResults(t *testing.T) {
	tempRoot := t.TempDir()
	compileCounter := filepath.Join(t.TempDir(), "compile-count")
	cfg := Config{
		HostTempDir:               tempRoot,
		DefaultCompileTimeLimit:   time.Second,
		DefaultExecuteTimeLimit:   time.Second,
		DefaultExecuteMemoryLimit: 64 * 1024 * 1024,
		MaxStdoutSize:             1024,
		MaxStderrSize:             1024,
		Languages: map[string]LanguageConfig{"test": {
			Compile: CompileConfig{
				SrcName:        "main.src",
				ExeName:        "main.bin",
				CompileCommand: fmt.Sprintf("cp {{SRC_PATH}} {{EXE_PATH}} && printf x >> %q", compileCounter),
			},
			Run: RunConfig{Command: "{{EXE_PATH}}"},
		}},
	}
	executor := &batchCommandExecutor{results: []Result{
		{Status: StatusAccepted, Stdout: "one", TimeUsedMillis: 7, MemoryUsedKB: 10},
		{Status: StatusAccepted, Stdout: "two", TimeUsedMillis: 9, MemoryUsedKB: 12},
	}}
	runner := &Runner{cfg: cfg, executor: executor}
	inputs := []string{"input-one", "input-two"}
	emitted := make([]BatchCaseResult, 0, 2)

	result := runner.RunBatchWithConfig(context.Background(), "test", "source", []BatchCase{
		{ID: "case-1", Stdin: &inputs[0]},
		{ID: "case-2", Stdin: &inputs[1]},
	}, false, cfg, func(caseResult BatchCaseResult) error {
		emitted = append(emitted, caseResult)
		return nil
	})

	if result.Status != StatusAccepted {
		t.Fatalf("batch status = %q, want %q: %+v", result.Status, StatusAccepted, result)
	}
	if got, err := os.ReadFile(compileCounter); err != nil || string(got) != "x" {
		t.Fatalf("compile counter = %q, err=%v; want one invocation", got, err)
	}
	if len(emitted) != 2 || emitted[0].ID != "case-1" || emitted[1].ID != "case-2" {
		t.Fatalf("emitted = %+v", emitted)
	}
	if fmt.Sprint(executor.inputs) != "[input-one input-two]" {
		t.Fatalf("execution inputs = %v", executor.inputs)
	}
	entries, err := os.ReadDir(tempRoot)
	if err != nil || len(entries) != 0 {
		t.Fatalf("request artifacts were not cleaned: entries=%v err=%v", entries, err)
	}
}

func TestRunBatchStopsAfterFirstContestantFailure(t *testing.T) {
	tempRoot := t.TempDir()
	cfg := Config{
		HostTempDir:               tempRoot,
		DefaultCompileTimeLimit:   time.Second,
		DefaultExecuteTimeLimit:   time.Second,
		DefaultExecuteMemoryLimit: 64 * 1024 * 1024,
		MaxStdoutSize:             1024,
		MaxStderrSize:             1024,
		Languages: map[string]LanguageConfig{"test": {
			Compile: CompileConfig{SrcName: "main.src"},
			Run:     RunConfig{Command: "test-runner"},
		}},
	}
	executor := &batchCommandExecutor{results: []Result{
		{Status: StatusWrongAnswer},
		{Status: StatusAccepted},
	}}
	runner := &Runner{cfg: cfg, executor: executor}
	input := "hidden"
	emitted := 0

	result := runner.RunBatchWithConfig(context.Background(), "test", "source", []BatchCase{
		{ID: "case-1", Stdin: &input},
		{ID: "case-2", Stdin: &input},
	}, true, cfg, func(BatchCaseResult) error {
		emitted++
		return nil
	})

	if result.Status != StatusWrongAnswer || emitted != 1 || len(executor.inputs) != 1 {
		t.Fatalf("result=%+v emitted=%d executions=%d", result, emitted, len(executor.inputs))
	}
	entries, err := os.ReadDir(tempRoot)
	if err != nil || len(entries) != 0 {
		t.Fatalf("request artifacts were not cleaned: entries=%v err=%v", entries, err)
	}
}
