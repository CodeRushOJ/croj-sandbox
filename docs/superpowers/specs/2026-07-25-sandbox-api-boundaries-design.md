# Sandbox API Boundaries Design

## Scope

This change hardens the existing unary `Execute` and server-streaming
`ExecuteBatchV1` RPCs without changing the protobuf contract or generated code.
It adds decode-time message limits, explicit field and aggregate limits,
pre-decode batch admission, RPC context propagation, a five-minute batch wall
clock ceiling, and bounded gRPC shutdown.

## Request limits

The server keeps the transport receive ceiling at 64 MiB for Batch V1, but
installs a `CodecV2` wrapper around gRPC's registered protobuf codec. Before
calling protobuf unmarshal, the wrapper checks the destination message type.
An `ExecuteRequest` larger than 4 MiB is converted to a private rejection
marker; the unary handler recognizes the marker and returns
`ResourceExhausted`, so protobuf never allocates the request fields and the
executor is never called. `ExecuteBatchV1Request` retains the 64 MiB transport
ceiling.

After decode, both RPCs apply byte-oriented limits:

- source code: 4 MiB;
- stdin: 4 MiB per unary request or batch case;
- expected output: 4 MiB per unary request or batch case;
- batch `case_id`: 256 bytes;
- batch cases: 256;
- aggregate batch payload (source, IDs, stdin, expected output, token digests):
  64 MiB.

Resource limits return `ResourceExhausted`. Malformed values such as empty or
duplicate case IDs and invalid token digests return `InvalidArgument`.

## Admission and execution lifecycle

The stream interceptor acquires one execution slot only for
`/sandbox.SandboxService/ExecuteBatchV1`. gRPC invokes stream interceptors
before the generated handler, so overload is rejected before its first
`RecvMsg`. The interceptor owns release for the complete stream lifecycle and
the service handler does not acquire again. Health and reflection methods do
not match the execution method and therefore never consume a slot. Unary
admission remains in the unary handler because gRPC decodes unary requests
before invoking unary interceptors.

`SandboxAPI.ExecuteContext(ctx, req)` derives its execution timeout from the
caller context. The compatibility `Execute(req)` method delegates with
`context.Background()`, while the gRPC service requires and calls the
context-aware method. Cancellation and deadlines therefore reach compilation
and execution.

Batch timeout calculation remains compile timeout plus per-case execution
timeouts and the existing five-second allowance, but is clamped to an absolute
five-minute wall-clock limit.

## Shutdown

The signal path calls a helper with a 25-second context. The helper first marks
the health service `NOT_SERVING`, starts `GracefulStop`, and returns if the
drain completes. If the context expires, it calls `Stop` and waits for
`GracefulStop` to unblock. This leaves five seconds inside the Deployment's
30-second `terminationGracePeriodSeconds` for process and container cleanup.

## Verification

Tests cover codec rejection before executor invocation, unary and batch field
boundaries, aggregate/count limits, stream admission before handler/`RecvMsg`,
health bypass, RPC cancellation propagation, deterministic timeout clamping,
and deterministic graceful/forced shutdown without timing sleeps. Final
verification runs `gofmt`, the race suite, vet, all three binaries, and
`git diff --check`.
