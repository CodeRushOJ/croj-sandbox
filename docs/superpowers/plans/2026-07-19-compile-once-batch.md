# Compile-once Batch Sandbox Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Compile each submission once and stream ordered per-case results from one bounded sandbox request.

**Architecture:** Extend the protobuf with a versioned server-streaming RPC while keeping `Execute` unchanged. Split runner preparation from per-case execution so one temporary artifact lifecycle owns one compile and several independently limited child processes.

**Tech Stack:** Go 1.24, protobuf/gRPC, Linux process/cgroup/seccomp execution, Docker CI.

---

### Task 1: Versioned batch contract

**Files:** modify `proto/sandbox.proto`; regenerate `proto/sandbox.pb.go`, `proto/sandbox_grpc.pb.go`; test `cmd/api-server/batch_test.go`.

- [ ] Add a failing server test that calls `ExecuteBatchV1`, expects ordered case events plus completion, and verifies the legacy `Execute` still works.
- [ ] Run `go test ./cmd/api-server` and confirm compilation fails because the batch symbols are absent.
- [ ] Add `ExecuteBatchV1Request`, `ExecuteBatchV1Case`, `ExecuteBatchV1Event` and `ExecuteBatchV1` server-streaming RPC; regenerate both Go files.
- [ ] Run the focused test and confirm it now reaches the unimplemented runner path.
- [ ] Commit the contract and RED/GREEN server surface.

### Task 2: Compile-once runner lifecycle

**Files:** modify `internal/sandbox/runner.go`, `internal/sandbox/api.go`; create `internal/sandbox/batch.go`, `internal/sandbox/batch_test.go`.

- [ ] Add failing tests with a compile-command counter proving two cases compile once, run in order, aggregate no state outside the request, stop early and clean the run directory.
- [ ] Run `go test ./internal/sandbox -run Batch -count=1` and confirm the batch API is missing.
- [ ] Implement `RunBatchWithConfig`: validate the language, create one run directory, stage/compile once, prepare one run command, execute cases sequentially with the existing executor and output checker, and defer cleanup once.
- [ ] Propagate context to compile and every case, stop before the next case on cancellation or configured non-Accepted verdict, and return structured compile/case results.
- [ ] Run focused tests, then `go test -race ./internal/sandbox`.
- [ ] Commit the runner lifecycle.

### Task 3: gRPC admission, validation and streaming

**Files:** modify `cmd/api-server/main.go`; test `cmd/api-server/batch_test.go`, `cmd/api-server/admission_test.go`, `cmd/api-server/logging_test.go`.

- [ ] Add failing tests for empty/duplicate/over-256 cases, one slot per whole batch, `ResourceExhausted`, cancellation, ordered stream events and payload-free logs.
- [ ] Run `go test ./cmd/api-server -run 'Batch|Admission|Log' -count=1` and confirm expected failures.
- [ ] Validate batch metadata before admission, acquire/release exactly one limiter slot, invoke batch API once, stream bounded result metadata and map cancellation to canonical gRPC status.
- [ ] Run focused tests and race tests; commit.

### Task 4: Documentation and repository gates

**Files:** modify `README.md`, `CHANGELOG.md`.

- [ ] Document the batch v1 wire contract, limits, compatibility, cleanup and rollout order.
- [ ] Run `go test -race -timeout=10m ./...`, `go vet ./...`, `go build ./cmd/api-server`, `git diff --check`, and the builder image checks.
- [ ] Commit, push `codex/compile-once-batch`, create a Draft PR based on `codex/redact-hidden-judge-logs`, and update judging-server Issue #11 with the PR dependency.
