# Sandbox API Boundaries Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bound request decoding and execution lifecycles so oversized or overloaded RPCs cannot allocate or run beyond the documented limits.

**Architecture:** A protobuf `CodecV2` wrapper enforces message-type decode limits, while focused validation helpers enforce decoded field and aggregate limits. Batch admission moves into the stream interceptor, execution APIs become context-aware, timeout calculation is clamped, and shutdown is coordinated by a context-bounded helper.

**Tech Stack:** Go 1.24, gRPC-Go 1.72, protobuf, bufconn, Kubernetes Deployment.

---

### Task 1: Decode and payload boundaries

**Files:**
- Create: `cmd/api-server/request_limits.go`
- Create: `cmd/api-server/request_limits_test.go`
- Modify: `cmd/api-server/main.go`

- [ ] **Step 1: Write failing codec and validation tests**

Add tests that send a protobuf-encoded unary request larger than 4 MiB through
bufconn and assert `ResourceExhausted` with zero executor calls. Add table tests
at and one byte above source, stdin, expected-output, case-ID, case-count, and
aggregate batch limits.

- [ ] **Step 2: Run tests and record RED**

Run:

```bash
go test ./cmd/api-server -run 'Test(ExecuteRequestCodec|ExecutePayload|BatchPayload)' -count=1
```

Expected: compile failures for the missing codec/validation helpers or
behavioral failures because existing handlers have no byte limits.

- [ ] **Step 3: Implement minimal codec and validators**

Wrap `encoding.GetCodecV2("proto")`, check `mem.BufferSlice.Len()` before
delegating unmarshal for `*pb.ExecuteRequest`, and install it with
`grpc.ForceServerCodecV2`. Implement byte-count validators and return
`ResourceExhausted` for limits and `InvalidArgument` for malformed semantics.

- [ ] **Step 4: Run tests and record GREEN**

Run the command from Step 2 and expect PASS.

### Task 2: Pre-receive batch admission

**Files:**
- Modify: `cmd/api-server/admission.go`
- Modify: `cmd/api-server/admission_test.go`
- Modify: `cmd/api-server/main.go`
- Modify: `cmd/api-server/batch_test.go`

- [ ] **Step 1: Write failing interceptor tests**

Occupy the limiter, invoke the batch stream interceptor with a counting stream
and handler, and assert `ResourceExhausted`, zero handler calls, and zero
`RecvMsg` calls. Add full-capacity health/reflection method tests that still
invoke their handler.

- [ ] **Step 2: Run tests and record RED**

```bash
go test ./cmd/api-server -run 'TestBatchAdmission|TestNonExecutionStream' -count=1
```

Expected: compile failure because the batch admission interceptor is absent.

- [ ] **Step 3: Implement minimal interceptor ownership**

Add an exact-method stream interceptor, chain it after stream recovery, and
remove acquisition from `ExecuteBatchV1` so only the interceptor owns the
batch slot.

- [ ] **Step 4: Run tests and record GREEN**

Run the command from Step 2 and expect PASS.

### Task 3: RPC context and batch wall clock

**Files:**
- Modify: `internal/sandbox/api.go`
- Modify: `internal/sandbox/batch_api.go`
- Modify: `internal/sandbox/batch_test.go`
- Modify: `cmd/api-server/main.go`
- Modify: `cmd/api-server/admission_test.go`
- Modify: `cmd/api-server/logging_test.go`

- [ ] **Step 1: Write failing context and timeout tests**

Add a context-aware blocking API stub, cancel an admitted unary RPC, and assert
the stub observes cancellation. Add pure calculation tests that prove small
batches retain their calculated timeout and large batches clamp to five
minutes.

- [ ] **Step 2: Run tests and record RED**

```bash
go test ./internal/sandbox ./cmd/api-server -run 'Test(ExecuteContext|GRPCExecutePropagatesCancellation|BatchWallClock)' -count=1
```

Expected: compile failures for `ExecuteContext` and the timeout helper.

- [ ] **Step 3: Implement context propagation and timeout clamp**

Move current unary execution into `ExecuteContext`, derive its timeout child
from the supplied context, retain `Execute` as a background compatibility
wrapper, require `ExecuteContext` in the gRPC interface, and use
`min(calculated, 5*time.Minute)` for batch context creation.

- [ ] **Step 4: Run tests and record GREEN**

Run the command from Step 2 and expect PASS.

### Task 4: Bounded shutdown and documentation

**Files:**
- Create: `cmd/api-server/shutdown.go`
- Create: `cmd/api-server/shutdown_test.go`
- Modify: `cmd/api-server/main.go`
- Modify: `README.md`
- Modify: `CHANGELOG.md`
- Modify: `deploy/deployment.yaml`

- [ ] **Step 1: Write failing deterministic shutdown tests**

Use controllable fake `GracefulStop`/`Stop` methods and cancellable contexts to
assert health changes first, graceful completion avoids `Stop`, and timeout
forces `Stop` without sleeps.

- [ ] **Step 2: Run tests and record RED**

```bash
go test ./cmd/api-server -run 'TestBoundedShutdown' -count=1
```

Expected: compile failure because the helper does not exist.

- [ ] **Step 3: Implement helper and update operator contract**

Call the helper from the signal path with a 25-second context. Document all
request limits, five-minute batch wall clock, pre-receive overload rejection,
and 25-second graceful/forced shutdown within the 30-second Pod grace period.

- [ ] **Step 4: Run tests and record GREEN**

Run the command from Step 2 and expect PASS.

### Task 5: Repository verification and publication

**Files:**
- Modify: any touched Go file through `gofmt`

- [ ] **Step 1: Format and run focused tests**

```bash
gofmt -w cmd/api-server/*.go internal/sandbox/*.go
go test ./cmd/api-server ./internal/sandbox
```

- [ ] **Step 2: Run required verification**

```bash
go test -race -timeout=10m ./...
go vet ./...
go build ./cmd/api-server
go build ./cmd/simple-client
go build .
git diff --check
```

- [ ] **Step 3: Review scope and commit**

Confirm generated protobuf files are unchanged, inspect the full diff, commit
all tests/code/docs, and push `codex/sandbox-api-boundaries` without opening a
pull request.
