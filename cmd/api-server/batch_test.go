package main

import (
	"context"
	"testing"

	"github.com/CodeRushOJ/croj-sandbox/internal/sandbox"
	pb "github.com/CodeRushOJ/croj-sandbox/proto"
	"google.golang.org/grpc/metadata"
)

type batchAPIStub struct {
	requests []sandbox.BatchRequest
}

func TestExecuteBatchV1DoesNotAcquireSecondAdmissionSlot(t *testing.T) {
	api := &batchAPIStub{}
	limiter, err := newExecutionLimiter(1)
	if err != nil {
		t.Fatal(err)
	}
	release, admitted := limiter.tryAcquire()
	if !admitted {
		t.Fatal("failed to occupy test slot")
	}
	defer release()
	service := &server{api: api, supportedLangs: []string{"go"}, limiter: limiter}
	stream := &batchEventStream{ctx: context.Background()}
	err = service.ExecuteBatchV1(&pb.ExecuteBatchV1Request{
		Language: "go",
		Cases:    []*pb.ExecuteBatchV1Case{{CaseId: "case-1"}},
	}, stream)
	if err != nil || len(api.requests) != 1 {
		t.Fatalf("error=%v API calls=%d, want success with one API call", err, len(api.requests))
	}
}

func (api *batchAPIStub) ExecuteContext(context.Context, sandbox.Request) sandbox.Response {
	return sandbox.Response{Status: string(sandbox.StatusAccepted)}
}

func (api *batchAPIStub) ExecuteBatch(ctx context.Context, request sandbox.BatchRequest, emit func(sandbox.BatchCaseResponse) error) sandbox.Response {
	api.requests = append(api.requests, request)
	for _, testCase := range request.Cases {
		if err := emit(sandbox.BatchCaseResponse{
			ID:       testCase.ID,
			Response: sandbox.Response{Status: string(sandbox.StatusAccepted), Stdout: testCase.ID},
		}); err != nil {
			return sandbox.Response{Status: string(sandbox.StatusSandboxError), Error: err.Error()}
		}
	}
	return sandbox.Response{Status: string(sandbox.StatusAccepted)}
}

type batchEventStream struct {
	ctx    context.Context
	events []*pb.ExecuteBatchV1Event
}

func (stream *batchEventStream) Context() context.Context { return stream.ctx }
func (*batchEventStream) SetHeader(metadata.MD) error     { return nil }
func (*batchEventStream) SendHeader(metadata.MD) error    { return nil }
func (*batchEventStream) SetTrailer(metadata.MD)          {}
func (*batchEventStream) SendMsg(any) error               { return nil }
func (*batchEventStream) RecvMsg(any) error               { return nil }
func (stream *batchEventStream) Send(event *pb.ExecuteBatchV1Event) error {
	stream.events = append(stream.events, event)
	return nil
}

func TestExecuteBatchV1StreamsOrderedResultsAndCompletion(t *testing.T) {
	api := &batchAPIStub{}
	limiter, err := newExecutionLimiter(1)
	if err != nil {
		t.Fatal(err)
	}
	service := &server{api: api, supportedLangs: []string{"go"}, limiter: limiter}
	stream := &batchEventStream{ctx: context.Background()}

	err = service.ExecuteBatchV1(&pb.ExecuteBatchV1Request{
		Language:      "go",
		SourceCode:    "package main",
		Timeout:       1,
		MemoryLimit:   64,
		StopOnFailure: true,
		Cases: []*pb.ExecuteBatchV1Case{
			{CaseId: "case-1", Stdin: "one"},
			{CaseId: "case-2", Stdin: "two"},
		},
	}, stream)
	if err != nil {
		t.Fatal(err)
	}
	if len(api.requests) != 1 || len(api.requests[0].Cases) != 2 {
		t.Fatalf("batch API requests = %+v", api.requests)
	}
	if len(stream.events) != 3 {
		t.Fatalf("stream events = %+v", stream.events)
	}
	if stream.events[0].Kind != pb.ExecuteBatchV1Event_CASE_RESULT || stream.events[0].CaseId != "case-1" {
		t.Fatalf("first event = %+v", stream.events[0])
	}
	if stream.events[1].Kind != pb.ExecuteBatchV1Event_CASE_RESULT || stream.events[1].CaseId != "case-2" {
		t.Fatalf("second event = %+v", stream.events[1])
	}
	if stream.events[2].Kind != pb.ExecuteBatchV1Event_COMPLETED {
		t.Fatalf("completion event = %+v", stream.events[2])
	}
}
