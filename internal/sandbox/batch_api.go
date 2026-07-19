package sandbox

import (
	"context"
	"time"
)

type BatchCaseRequest struct {
	ID                  string
	Stdin               string
	ExpectedOutput      *string
	ExpectedTokenSHA256 *string
}

type BatchRequest struct {
	SourceCode    string
	Language      string
	Timeout       *int
	MemoryLimit   *int
	StopOnFailure bool
	Cases         []BatchCaseRequest
}

type BatchCaseResponse struct {
	ID string
	Response
}

// ExecuteBatch applies immutable request limits once and owns one compile/run lifecycle.
func (api *SandboxAPI) ExecuteBatch(
	ctx context.Context,
	request BatchRequest,
	emit func(BatchCaseResponse) error,
) Response {
	language := request.Language
	if language == "" {
		language = "go"
	}
	execTimeout := api.cfg.DefaultExecuteTimeLimit
	if langConfig, ok := api.cfg.Languages[language]; ok {
		execTimeout = langConfig.GetExecuteTimeout(execTimeout)
	}
	userSpecifiedTimeout := false
	if request.Timeout != nil && *request.Timeout > 0 {
		execTimeout = min(time.Duration(*request.Timeout)*time.Second, 30*time.Second)
		userSpecifiedTimeout = true
	}
	memoryLimit := api.cfg.DefaultExecuteMemoryLimit
	if request.MemoryLimit != nil && *request.MemoryLimit > 0 {
		memoryLimit = min(int64(*request.MemoryLimit)*1024*1024, int64(4*1024*1024*1024))
	}
	customConfig := api.cfg
	customConfig.DefaultExecuteTimeLimit = execTimeout
	customConfig.DefaultExecuteMemoryLimit = memoryLimit
	customConfig.ExecTimeout = execTimeout
	customConfig.UserSpecifiedTimeout = userSpecifiedTimeout

	compileTimeout := customConfig.DefaultCompileTimeLimit
	if customConfig.CompileTimeout > 0 {
		compileTimeout = customConfig.CompileTimeout
	}
	batchTimeout := compileTimeout + time.Duration(len(request.Cases))*execTimeout + 5*time.Second
	batchContext, cancel := context.WithTimeout(ctx, batchTimeout)
	defer cancel()

	cases := make([]BatchCase, 0, len(request.Cases))
	for _, testCase := range request.Cases {
		input := testCase.Stdin
		cases = append(cases, BatchCase{
			ID:                  testCase.ID,
			Stdin:               &input,
			ExpectedOutput:      testCase.ExpectedOutput,
			ExpectedTokenSHA256: testCase.ExpectedTokenSHA256,
		})
	}
	result := api.runner.RunBatchWithConfig(
		batchContext,
		language,
		request.SourceCode,
		cases,
		request.StopOnFailure,
		customConfig,
		func(caseResult BatchCaseResult) error {
			return emit(BatchCaseResponse{ID: caseResult.ID, Response: responseFromResult(caseResult.Result)})
		},
	)
	return responseFromResult(result)
}

func responseFromResult(result Result) Response {
	return Response{
		Status:       string(result.Status),
		ExitCode:     result.ExitCode,
		Stdout:       result.Stdout,
		Stderr:       result.Stderr,
		Error:        result.Error,
		TimeUsed:     result.TimeUsedMillis,
		MemoryUsed:   result.MemoryUsedKB,
		CompileError: result.CompileOutput,
	}
}
