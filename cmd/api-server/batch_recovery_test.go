package main

import (
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRecoveryStreamInterceptorConvertsBatchPanicToInternal(t *testing.T) {
	err := recoveryStreamServerInterceptor(
		nil,
		&batchEventStream{},
		&grpc.StreamServerInfo{FullMethod: "/sandbox.SandboxService/ExecuteBatchV1", IsServerStream: true},
		func(any, grpc.ServerStream) error {
			panic("hidden-payload-sentinel")
		},
	)
	if status.Code(err) != codes.Internal {
		t.Fatalf("panic code = %s, want %s", status.Code(err), codes.Internal)
	}
}
