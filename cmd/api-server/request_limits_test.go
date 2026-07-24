package main

import (
	"context"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/CodeRushOJ/croj-sandbox/internal/sandbox"
	pb "github.com/CodeRushOJ/croj-sandbox/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/mem"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestExecuteRequestCodecRejectsOversizedUnaryBeforeExecutor(t *testing.T) {
	api := &countingSandboxAPI{}
	grpcServer, _, err := newGRPCServer(api, []string{"go"}, 1)
	if err != nil {
		t.Fatalf("newGRPCServer: %v", err)
	}
	listener := bufconn.Listen(8 << 20)
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(grpcServer.Stop)
	t.Cleanup(func() { _ = listener.Close() })

	connection, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial gRPC server: %v", err)
	}
	t.Cleanup(func() { _ = connection.Close() })

	_, err = pb.NewSandboxServiceClient(connection).Execute(context.Background(), &pb.ExecuteRequest{
		Language:   "go",
		SourceCode: strings.Repeat("x", (4<<20)+1),
	})
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("Execute code = %s, want %s (err=%v)", status.Code(err), codes.ResourceExhausted, err)
	}
	if calls := api.calls.Load(); calls != 0 {
		t.Fatalf("executor calls = %d, want 0", calls)
	}
}

func TestRequestLimitCodecChecksMessageTypeBeforeUnmarshal(t *testing.T) {
	tests := []struct {
		name          string
		size          int
		target        any
		wantDelegated bool
	}{
		{
			name:          "execute at limit",
			size:          maxExecuteMessageBytes,
			target:        &pb.ExecuteRequest{},
			wantDelegated: true,
		},
		{
			name:          "execute above limit",
			size:          maxExecuteMessageBytes + 1,
			target:        &pb.ExecuteRequest{},
			wantDelegated: false,
		},
		{
			name:          "batch above unary limit",
			size:          maxExecuteMessageBytes + 1,
			target:        &pb.ExecuteBatchV1Request{},
			wantDelegated: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			delegate := &countingCodec{}
			codec := requestLimitCodec{delegate: delegate}
			data := mem.BufferSlice{mem.SliceBuffer(make([]byte, test.size))}
			if err := codec.Unmarshal(data, test.target); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if got := delegate.unmarshalCalls.Load() > 0; got != test.wantDelegated {
				t.Fatalf("delegate called = %v, want %v", got, test.wantDelegated)
			}
		})
	}
}

func TestExecutePayloadByteLimits(t *testing.T) {
	tests := []struct {
		name    string
		request *pb.ExecuteRequest
		want    codes.Code
	}{
		{
			name:    "source at limit",
			request: &pb.ExecuteRequest{SourceCode: strings.Repeat("s", maxSourceCodeBytes)},
			want:    codes.OK,
		},
		{
			name:    "source above limit",
			request: &pb.ExecuteRequest{SourceCode: strings.Repeat("s", maxSourceCodeBytes+1)},
			want:    codes.ResourceExhausted,
		},
		{
			name:    "stdin at limit",
			request: &pb.ExecuteRequest{Stdin: strings.Repeat("i", maxStdinBytes)},
			want:    codes.OK,
		},
		{
			name:    "stdin above limit",
			request: &pb.ExecuteRequest{Stdin: strings.Repeat("i", maxStdinBytes+1)},
			want:    codes.ResourceExhausted,
		},
		{
			name:    "expected output at limit",
			request: &pb.ExecuteRequest{ExpectedOutput: strings.Repeat("e", maxExpectedOutputBytes)},
			want:    codes.OK,
		},
		{
			name:    "expected output above limit",
			request: &pb.ExecuteRequest{ExpectedOutput: strings.Repeat("e", maxExpectedOutputBytes+1)},
			want:    codes.ResourceExhausted,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := status.Code(validateExecutePayload(test.request)); got != test.want {
				t.Fatalf("validateExecutePayload code = %s, want %s", got, test.want)
			}
		})
	}
}

func TestBatchPayloadByteLimits(t *testing.T) {
	t.Run("case id", func(t *testing.T) {
		atLimit := validBatchRequest()
		atLimit.Cases[0].CaseId = strings.Repeat("c", maxBatchCaseIDBytes)
		if err := validateBatchPayload(atLimit); err != nil {
			t.Fatalf("case_id at limit: %v", err)
		}
		above := validBatchRequest()
		above.Cases[0].CaseId = strings.Repeat("c", maxBatchCaseIDBytes+1)
		if got := status.Code(validateBatchPayload(above)); got != codes.ResourceExhausted {
			t.Fatalf("case_id above limit code = %s, want %s", got, codes.ResourceExhausted)
		}
	})

	t.Run("case count", func(t *testing.T) {
		atLimit := validBatchRequest()
		atLimit.Cases = make([]*pb.ExecuteBatchV1Case, maxBatchCasesV1)
		for index := range atLimit.Cases {
			atLimit.Cases[index] = &pb.ExecuteBatchV1Case{CaseId: strconv.Itoa(index)}
		}
		if err := validateBatchPayload(atLimit); err != nil {
			t.Fatalf("case count at limit: %v", err)
		}
		above := validBatchRequest()
		above.Cases = append(atLimit.Cases, &pb.ExecuteBatchV1Case{CaseId: "overflow"})
		if got := status.Code(validateBatchPayload(above)); got != codes.ResourceExhausted {
			t.Fatalf("case count above limit code = %s, want %s", got, codes.ResourceExhausted)
		}
	})

	t.Run("per field", func(t *testing.T) {
		tests := []struct {
			name   string
			limit  int
			mutate func(*pb.ExecuteBatchV1Request, int)
		}{
			{"source", maxSourceCodeBytes, func(request *pb.ExecuteBatchV1Request, size int) {
				request.SourceCode = strings.Repeat("s", size)
			}},
			{"stdin", maxStdinBytes, func(request *pb.ExecuteBatchV1Request, size int) {
				request.Cases[0].Stdin = strings.Repeat("i", size)
			}},
			{"expected output", maxExpectedOutputBytes, func(request *pb.ExecuteBatchV1Request, size int) {
				request.Cases[0].ExpectedOutput = strings.Repeat("e", size)
			}},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				atLimit := validBatchRequest()
				test.mutate(atLimit, test.limit)
				if err := validateBatchPayload(atLimit); err != nil {
					t.Fatalf("field at limit: %v", err)
				}
				above := validBatchRequest()
				test.mutate(above, test.limit+1)
				if got := status.Code(validateBatchPayload(above)); got != codes.ResourceExhausted {
					t.Fatalf("field above limit code = %s, want %s", got, codes.ResourceExhausted)
				}
			})
		}
	})

	t.Run("aggregate", func(t *testing.T) {
		request := &pb.ExecuteBatchV1Request{SourceCode: strings.Repeat("s", maxSourceCodeBytes)}
		const payloadCases = 15
		request.Cases = make([]*pb.ExecuteBatchV1Case, payloadCases)
		idBytes := 0
		for index := range request.Cases {
			id := "case-" + strconv.Itoa(index)
			idBytes += len(id)
			request.Cases[index] = &pb.ExecuteBatchV1Case{
				CaseId:         id,
				ExpectedOutput: strings.Repeat("e", maxExpectedOutputBytes),
			}
		}
		last := request.Cases[len(request.Cases)-1]
		last.ExpectedOutput = last.ExpectedOutput[:len(last.ExpectedOutput)-idBytes]
		if got := batchPayloadBytes(request); got != maxBatchPayloadBytesV1 {
			t.Fatalf("aggregate bytes = %d, want %d", got, maxBatchPayloadBytesV1)
		}
		if err := validateBatchPayload(request); err != nil {
			t.Fatalf("aggregate at limit: %v", err)
		}
		last.ExpectedOutput += "x"
		if got := status.Code(validateBatchPayload(request)); got != codes.ResourceExhausted {
			t.Fatalf("aggregate above limit code = %s, want %s", got, codes.ResourceExhausted)
		}
	})
}

func validBatchRequest() *pb.ExecuteBatchV1Request {
	return &pb.ExecuteBatchV1Request{
		Language:   "go",
		SourceCode: "package main",
		Cases:      []*pb.ExecuteBatchV1Case{{CaseId: "case-1"}},
	}
}

type countingSandboxAPI struct {
	calls atomic.Int64
}

func (api *countingSandboxAPI) ExecuteContext(context.Context, sandbox.Request) sandbox.Response {
	api.calls.Add(1)
	return sandbox.Response{Status: string(sandbox.StatusAccepted)}
}

type countingCodec struct {
	unmarshalCalls atomic.Int64
}

func (*countingCodec) Marshal(any) (mem.BufferSlice, error) {
	return nil, nil
}

func (codec *countingCodec) Unmarshal(mem.BufferSlice, any) error {
	codec.unmarshalCalls.Add(1)
	return nil
}

func (*countingCodec) Name() string {
	return "proto"
}
