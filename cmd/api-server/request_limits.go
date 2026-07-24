package main

import (
	"fmt"

	pb "github.com/CodeRushOJ/croj-sandbox/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/mem"
	"google.golang.org/grpc/status"
)

const (
	maxExecuteMessageBytes = 4 << 20
	maxSourceCodeBytes     = 4 << 20
	maxStdinBytes          = 4 << 20
	maxExpectedOutputBytes = 4 << 20
	maxBatchCaseIDBytes    = 256
	maxBatchPayloadBytesV1 = 64 << 20
	oversizedExecuteMarker = "\x00croj-sandbox:oversized-execute-request"
)

type requestLimitCodec struct {
	delegate encoding.CodecV2
}

func newRequestLimitCodec() (encoding.CodecV2, error) {
	codec := encoding.GetCodecV2("proto")
	if codec == nil {
		return nil, fmt.Errorf("registered protobuf codec is unavailable")
	}
	return requestLimitCodec{delegate: codec}, nil
}

func (codec requestLimitCodec) Marshal(value any) (mem.BufferSlice, error) {
	return codec.delegate.Marshal(value)
}

func (codec requestLimitCodec) Unmarshal(data mem.BufferSlice, value any) error {
	if request, ok := value.(*pb.ExecuteRequest); ok && data.Len() > maxExecuteMessageBytes {
		request.Reset()
		request.Language = oversizedExecuteMarker
		return nil
	}
	return codec.delegate.Unmarshal(data, value)
}

func (codec requestLimitCodec) Name() string {
	return codec.delegate.Name()
}

func validateExecutePayload(request *pb.ExecuteRequest) error {
	if request == nil {
		return status.Error(codes.InvalidArgument, "execute request is required")
	}
	if request.Language == oversizedExecuteMarker {
		return status.Errorf(codes.ResourceExhausted, "serialized ExecuteRequest exceeds %d bytes", maxExecuteMessageBytes)
	}
	if len(request.SourceCode) > maxSourceCodeBytes {
		return payloadLimitError("source_code", maxSourceCodeBytes)
	}
	if len(request.Stdin) > maxStdinBytes {
		return payloadLimitError("stdin", maxStdinBytes)
	}
	if len(request.ExpectedOutput) > maxExpectedOutputBytes {
		return payloadLimitError("expected_output", maxExpectedOutputBytes)
	}
	return nil
}

func validateBatchPayload(request *pb.ExecuteBatchV1Request) error {
	if request == nil {
		return status.Error(codes.InvalidArgument, "batch request is required")
	}
	if len(request.Cases) == 0 {
		return status.Error(codes.InvalidArgument, "batch cases are required")
	}
	if len(request.Cases) > maxBatchCasesV1 {
		return status.Errorf(codes.ResourceExhausted, "batch has %d cases; maximum is %d", len(request.Cases), maxBatchCasesV1)
	}
	if len(request.SourceCode) > maxSourceCodeBytes {
		return payloadLimitError("source_code", maxSourceCodeBytes)
	}
	for _, testCase := range request.Cases {
		if testCase == nil {
			continue
		}
		if len(testCase.CaseId) > maxBatchCaseIDBytes {
			return payloadLimitError("case_id", maxBatchCaseIDBytes)
		}
		if len(testCase.Stdin) > maxStdinBytes {
			return payloadLimitError("stdin", maxStdinBytes)
		}
		if len(testCase.ExpectedOutput) > maxExpectedOutputBytes {
			return payloadLimitError("expected_output", maxExpectedOutputBytes)
		}
	}
	if size := batchPayloadBytes(request); size > maxBatchPayloadBytesV1 {
		return status.Errorf(codes.ResourceExhausted, "batch payload exceeds %d bytes", maxBatchPayloadBytesV1)
	}
	return nil
}

func batchPayloadBytes(request *pb.ExecuteBatchV1Request) int {
	if request == nil {
		return 0
	}
	size := len(request.SourceCode)
	for _, testCase := range request.Cases {
		if testCase == nil {
			continue
		}
		size += len(testCase.CaseId)
		size += len(testCase.Stdin)
		size += len(testCase.ExpectedOutput)
		size += len(testCase.TokenExpectedSha256)
	}
	return size
}

func payloadLimitError(field string, maximum int) error {
	return status.Errorf(codes.ResourceExhausted, "%s exceeds %d bytes", field, maximum)
}
