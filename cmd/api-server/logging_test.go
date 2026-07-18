package main

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"

	"github.com/CodeRushOJ/croj-sandbox/internal/sandbox"
	pb "github.com/CodeRushOJ/croj-sandbox/proto"
)

const (
	grpcSentinelSource   = "GRPC_SENTINEL_SOURCE_d12d72"
	grpcSentinelStdin    = "GRPC_SENTINEL_STDIN_92c52c"
	grpcSentinelExpected = "GRPC_SENTINEL_EXPECTED_bab891"
	grpcSentinelStdout   = "GRPC_SENTINEL_STDOUT_3a768c"
	grpcSentinelStderr   = "GRPC_SENTINEL_STDERR_c64d2b"
	grpcSentinelCompile  = "GRPC_SENTINEL_COMPILE_f0279d"
	grpcSentinelError    = "GRPC_SENTINEL_ERROR_8d098f"
)

type payloadReturningAPI struct {
	t *testing.T
}

func (a payloadReturningAPI) Execute(req sandbox.Request) sandbox.Response {
	a.t.Helper()
	if req.SourceCode != grpcSentinelSource || req.Stdin == nil || *req.Stdin != grpcSentinelStdin || req.ExpectedOutput == nil || *req.ExpectedOutput != grpcSentinelExpected {
		a.t.Fatalf("gRPC payload mapping changed: %#v", req)
	}
	return sandbox.Response{
		Status:       string(sandbox.StatusCompileError),
		ExitCode:     1,
		Stdout:       grpcSentinelStdout,
		Stderr:       grpcSentinelStderr,
		Error:        grpcSentinelError,
		CompileError: grpcSentinelCompile,
	}
}

func TestGRPCServiceKeepsPayloadInResponseAndOutOfLogs(t *testing.T) {
	logs := captureAPIServerLogs(t)
	limiter, err := newExecutionLimiter(1)
	if err != nil {
		t.Fatalf("newExecutionLimiter: %v", err)
	}
	srv := &server{
		api:            payloadReturningAPI{t: t},
		supportedLangs: []string{"go"},
		limiter:        limiter,
	}

	resp, err := srv.Execute(context.Background(), &pb.ExecuteRequest{
		Language:       "go",
		SourceCode:     grpcSentinelSource,
		Stdin:          grpcSentinelStdin,
		ExpectedOutput: grpcSentinelExpected,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if resp.Stdout != grpcSentinelStdout || resp.Stderr != grpcSentinelStderr || resp.CompileError != grpcSentinelCompile || resp.Error != grpcSentinelError {
		t.Fatalf("gRPC response payload changed: %#v", resp)
	}
	assertNoGRPCSentinels(t, logs.String())
}

func TestGRPCServiceDoesNotLogUnsupportedLanguageValue(t *testing.T) {
	logs := captureAPIServerLogs(t)
	limiter, err := newExecutionLimiter(1)
	if err != nil {
		t.Fatalf("newExecutionLimiter: %v", err)
	}
	srv := &server{supportedLangs: []string{"go"}, limiter: limiter}
	unsupported := "GRPC_SENTINEL_UNSUPPORTED_3b139d\nforged=true"

	resp, err := srv.Execute(context.Background(), &pb.ExecuteRequest{Language: unsupported})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if resp.Error == "" {
		t.Fatal("unsupported language response has no error")
	}
	if strings.Contains(logs.String(), unsupported) || strings.Contains(logs.String(), "GRPC_SENTINEL_UNSUPPORTED_3b139d") {
		t.Fatalf("service logs contain untrusted language value:\n%s", logs.String())
	}
}

func captureAPIServerLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	oldWriter := log.Writer()
	oldFlags := log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(oldWriter)
		log.SetFlags(oldFlags)
	})
	return &logs
}

func assertNoGRPCSentinels(t *testing.T, output string) {
	t.Helper()
	for _, sentinel := range []string{
		grpcSentinelSource,
		grpcSentinelStdin,
		grpcSentinelExpected,
		grpcSentinelStdout,
		grpcSentinelStderr,
		grpcSentinelCompile,
		grpcSentinelError,
	} {
		if strings.Contains(output, sentinel) {
			t.Errorf("service logs contain gRPC payload %q:\n%s", sentinel, output)
		}
	}
}
