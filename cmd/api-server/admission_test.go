package main

import (
	"context"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CodeRushOJ/croj-sandbox/internal/sandbox"
	pb "github.com/CodeRushOJ/croj-sandbox/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestRecoveryInterceptorTurnsPanicIntoInternalError(t *testing.T) {
	_, err := recoveryUnaryServerInterceptor(
		context.Background(),
		validExecuteRequest(),
		&grpc.UnaryServerInfo{FullMethod: "/croj.sandbox.SandboxService/Execute"},
		func(context.Context, any) (any, error) { panic("test panic") },
	)
	if status.Code(err) != codes.Internal {
		t.Fatalf("recovery interceptor code = %s, want %s", status.Code(err), codes.Internal)
	}
}

func TestDefaultMaxConcurrencyIsPositive(t *testing.T) {
	if got := defaultMaxConcurrency(); got <= 0 {
		t.Fatalf("defaultMaxConcurrency() = %d, want > 0", got)
	}
}

func TestNewExecutionLimiterRejectsNonPositiveCapacity(t *testing.T) {
	for _, capacity := range []int{0, -1} {
		if _, err := newExecutionLimiter(capacity); err == nil {
			t.Fatalf("newExecutionLimiter(%d) returned nil error", capacity)
		}
	}
}

func TestExecuteRejectsOverloadWithoutCallingExecutor(t *testing.T) {
	executor := newBlockingExecutor()
	service := newTestSandboxService(t, executor, 1)
	firstDone := make(chan error, 1)
	go func() {
		_, err := service.Execute(context.Background(), validExecuteRequest())
		firstDone <- err
	}()
	<-executor.started

	_, err := service.Execute(context.Background(), validExecuteRequest())
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("second Execute code = %s, want %s", status.Code(err), codes.ResourceExhausted)
	}
	if calls := executor.calls.Load(); calls != 1 {
		t.Fatalf("executor calls = %d, want 1", calls)
	}
	stats := service.limiter.snapshot()
	if stats.capacity != 1 || stats.inFlight != 1 || stats.rejected != 1 {
		t.Fatalf("limiter stats = %+v, want capacity=1 inFlight=1 rejected=1", stats)
	}

	executor.release <- struct{}{}
	if err := <-firstDone; err != nil {
		t.Fatalf("first Execute returned error: %v", err)
	}
}

func TestExecuteReleasesSlotAfterCompletion(t *testing.T) {
	executor := newBlockingExecutor()
	service := newTestSandboxService(t, executor, 1)

	for attempt := 0; attempt < 2; attempt++ {
		done := make(chan error, 1)
		go func() {
			_, err := service.Execute(context.Background(), validExecuteRequest())
			done <- err
		}()
		<-executor.started
		executor.release <- struct{}{}
		if err := <-done; err != nil {
			t.Fatalf("Execute attempt %d returned error: %v", attempt, err)
		}
	}

	if calls := executor.calls.Load(); calls != 2 {
		t.Fatalf("executor calls = %d, want 2", calls)
	}
	if inFlight := service.limiter.snapshot().inFlight; inFlight != 0 {
		t.Fatalf("inFlight after completion = %d, want 0", inFlight)
	}
}

func TestExecuteReleasesSlotWhenExecutorPanics(t *testing.T) {
	executor := &panicOnceExecutor{}
	service := newTestSandboxService(t, executor, 1)

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("first Execute did not panic")
			}
		}()
		_, _ = service.Execute(context.Background(), validExecuteRequest())
	}()

	if inFlight := service.limiter.snapshot().inFlight; inFlight != 0 {
		t.Fatalf("inFlight after panic = %d, want 0", inFlight)
	}
	if _, err := service.Execute(context.Background(), validExecuteRequest()); err != nil {
		t.Fatalf("Execute after panic returned error: %v", err)
	}
}

func TestExecuteNeverExceedsConfiguredConcurrency(t *testing.T) {
	const capacity = 3
	const requests = 24
	executor := newBlockingExecutor()
	service := newTestSandboxService(t, executor, capacity)
	results := make(chan error, requests)
	var start sync.WaitGroup
	start.Add(1)

	for range requests {
		go func() {
			start.Wait()
			_, err := service.Execute(context.Background(), validExecuteRequest())
			results <- err
		}()
	}
	start.Done()
	for range capacity {
		<-executor.started
	}
	deadline := time.Now().Add(2 * time.Second)
	for service.limiter.snapshot().rejected != requests-capacity {
		if time.Now().After(deadline) {
			t.Fatalf("rejected=%d, want %d before releasing admitted calls", service.limiter.snapshot().rejected, requests-capacity)
		}
		runtime.Gosched()
	}

	for range capacity {
		executor.release <- struct{}{}
	}

	accepted := 0
	rejected := 0
	for range requests {
		err := <-results
		switch status.Code(err) {
		case codes.OK:
			accepted++
		case codes.ResourceExhausted:
			rejected++
		default:
			t.Fatalf("unexpected Execute error: %v", err)
		}
	}
	if accepted != capacity || rejected != requests-capacity {
		t.Fatalf("accepted=%d rejected=%d, want accepted=%d rejected=%d", accepted, rejected, capacity, requests-capacity)
	}
	if got := executor.maxActive.Load(); got > capacity {
		t.Fatalf("max active executions = %d, want <= %d", got, capacity)
	}
}

func TestCanceledRequestDoesNotCallExecutor(t *testing.T) {
	executor := newBlockingExecutor()
	service := newTestSandboxService(t, executor, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := service.Execute(ctx, validExecuteRequest())
	if status.Code(err) != codes.Canceled {
		t.Fatalf("Execute code = %s, want %s", status.Code(err), codes.Canceled)
	}
	if calls := executor.calls.Load(); calls != 0 {
		t.Fatalf("executor calls = %d, want 0", calls)
	}
}

func TestGracefulStopRejectsNewRPCsAndWaitsForAdmittedExecution(t *testing.T) {
	executor := newBlockingExecutor()
	grpcServer, _, err := newGRPCServer(executor, []string{"go"}, 1)
	if err != nil {
		t.Fatalf("newGRPCServer: %v", err)
	}
	t.Cleanup(grpcServer.Stop)
	listener := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() { _ = listener.Close() })
	go func() { _ = grpcServer.Serve(listener) }()
	connection, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial gRPC server: %v", err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	client := pb.NewSandboxServiceClient(connection)
	firstDone := make(chan error, 1)
	go func() {
		_, executeErr := client.Execute(context.Background(), validExecuteRequest())
		firstDone <- executeErr
	}()
	select {
	case <-executor.started:
	case err := <-firstDone:
		t.Fatalf("Execute returned before admission: %v", err)
	case <-time.After(time.Second):
		t.Fatal("Execute was not admitted within one second")
	}

	stopDone := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(stopDone)
	}()
	select {
	case <-stopDone:
		t.Fatal("GracefulStop returned before the admitted execution completed")
	case <-time.After(50 * time.Millisecond):
	}

	executor.release <- struct{}{}
	if err := <-firstDone; err != nil {
		t.Fatalf("admitted Execute returned error: %v", err)
	}
	select {
	case <-stopDone:
	case <-time.After(time.Second):
		t.Fatal("GracefulStop did not return after admitted execution completed")
	}

	rejectedCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err = client.Execute(rejectedCtx, validExecuteRequest())
	if code := status.Code(err); code != codes.Unavailable && code != codes.DeadlineExceeded {
		t.Fatalf("Execute after GracefulStop code = %s, want Unavailable or DeadlineExceeded", code)
	}
	if calls := executor.calls.Load(); calls != 1 {
		t.Fatalf("executor calls after GracefulStop = %d, want 1", calls)
	}
}

func newTestSandboxService(t *testing.T, executor sandboxExecutor, capacity int) *server {
	t.Helper()
	limiter, err := newExecutionLimiter(capacity)
	if err != nil {
		t.Fatalf("newExecutionLimiter: %v", err)
	}
	return &server{api: executor, supportedLangs: []string{"go"}, limiter: limiter}
}

func validExecuteRequest() *pb.ExecuteRequest {
	return &pb.ExecuteRequest{Language: "go", SourceCode: "package main\nfunc main() {}"}
}

type blockingExecutor struct {
	started   chan struct{}
	release   chan struct{}
	calls     atomic.Int64
	active    atomic.Int64
	maxActive atomic.Int64
}

func newBlockingExecutor() *blockingExecutor {
	return &blockingExecutor{
		started: make(chan struct{}, 64),
		release: make(chan struct{}, 64),
	}
}

func (e *blockingExecutor) Execute(sandbox.Request) sandbox.Response {
	e.calls.Add(1)
	active := e.active.Add(1)
	for {
		previous := e.maxActive.Load()
		if active <= previous || e.maxActive.CompareAndSwap(previous, active) {
			break
		}
	}
	e.started <- struct{}{}
	<-e.release
	e.active.Add(-1)
	return sandbox.Response{Status: "Accepted"}
}

type panicOnceExecutor struct {
	calls atomic.Int64
}

func (e *panicOnceExecutor) Execute(sandbox.Request) sandbox.Response {
	if e.calls.Add(1) == 1 {
		panic("test panic")
	}
	return sandbox.Response{Status: "Accepted"}
}
