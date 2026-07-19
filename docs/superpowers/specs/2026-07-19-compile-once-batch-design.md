# Compile-once Batch Sandbox Design

## Goal

Add a versioned sandbox RPC that compiles one submission once and runs an ordered, bounded test-case batch without changing the existing unary `Execute` contract. The API must preserve per-case time, memory and output limits, cancellation, admission backpressure, hidden-payload log redaction and deterministic cleanup.

## Contract

`SandboxService.ExecuteBatchV1` is a server-streaming RPC. Its request carries language, source, immutable limits, `stop_on_failure`, and ordered cases containing an opaque case ID, stdin, expected output and an explicit comparison flag. The server emits exactly one compile-error event, or one result event per completed case followed by a completion event. The method name is versioned so incompatible future protocol changes do not silently alter judging.

The legacy `Execute` RPC and messages remain unchanged. A batch is rejected with `InvalidArgument` when empty, over 256 cases, or contains duplicate/empty case IDs. The existing Pod admission semaphore accounts for a whole batch as one execution slot and returns `ResourceExhausted` before compiling when full.

## Execution lifecycle

The runner creates one private temporary directory, stages source once and invokes the configured compiler once. It then creates a fresh process for each case from the prepared source/artifact, applying the existing per-case executor timeout, cgroup/seccomp, stdout/stderr limits and output comparison. Cases run in request order. `stop_on_failure` ends after the first non-Accepted verdict. Context cancellation stops before the next case and reaches the active process through `exec.CommandContext`. The temporary directory and compiled artifact are removed once after stream completion, cancellation or error.

Compiled artifacts are request-local and never cached across submissions. This avoids cross-tenant artifact poisoning while removing the repeated compilation inside one submission.

## Judge retry and confidentiality

Judging-server sends a whole immutable bundle to one Ready EndpointSlice target. It may retry the full batch on another target only for `Unavailable` or `ResourceExhausted`; contestant verdicts are terminal. A stream that fails after partial events is still treated as an infrastructure failure because no callback is published until the full judge result exists.

The sandbox logs only language, case count, opaque case index, verdict and bounded metrics. Source, stdin, expected output, stdout/stderr and compiler diagnostics never enter logs. Judging callbacks continue to redact compiler diagnostics and all hidden payloads.

## Verification and rollout

Tests prove one compiler invocation for multiple cases, ordered streaming, early stop, per-case metrics, cleanup, cancellation, batch admission, backward-compatible unary execution and hidden-data redaction. Judging contract tests prove one batch RPC, exact/token behavior, bounded endpoint failover and callback redaction. Both repositories must pass race tests, vet and builds. Sandbox PR lands before the dependent judging-server PR; neither PR merges directly to `main`.
