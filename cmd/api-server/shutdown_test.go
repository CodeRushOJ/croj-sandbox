package main

import (
	"context"
	"sync"
	"testing"

	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func TestBoundedShutdownCompletesGracefullyAfterMarkingNotServing(t *testing.T) {
	health := &recordingHealthStatus{}
	server := newControllableStopper(health, false)

	if graceful := boundedShutdown(context.Background(), health, server); !graceful {
		t.Fatal("boundedShutdown reported forced stop, want graceful completion")
	}
	if got := health.current(); got != healthpb.HealthCheckResponse_NOT_SERVING {
		t.Fatalf("health status = %s, want %s", got, healthpb.HealthCheckResponse_NOT_SERVING)
	}
	if got := server.stopCalls; got != 0 {
		t.Fatalf("Stop calls = %d, want 0", got)
	}
}

func TestBoundedShutdownForcesStopWhenContextExpires(t *testing.T) {
	health := &recordingHealthStatus{}
	server := newControllableStopper(health, true)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan bool, 1)
	go func() {
		result <- boundedShutdown(ctx, health, server)
	}()
	<-server.gracefulStarted
	cancel()

	if graceful := <-result; graceful {
		t.Fatal("boundedShutdown reported graceful completion after deadline")
	}
	if got := health.current(); got != healthpb.HealthCheckResponse_NOT_SERVING {
		t.Fatalf("health status = %s, want %s", got, healthpb.HealthCheckResponse_NOT_SERVING)
	}
	if got := server.stopCalls; got != 1 {
		t.Fatalf("Stop calls = %d, want 1", got)
	}
}

type recordingHealthStatus struct {
	mu     sync.Mutex
	status healthpb.HealthCheckResponse_ServingStatus
}

func (health *recordingHealthStatus) SetServingStatus(_ string, status healthpb.HealthCheckResponse_ServingStatus) {
	health.mu.Lock()
	defer health.mu.Unlock()
	health.status = status
}

func (health *recordingHealthStatus) current() healthpb.HealthCheckResponse_ServingStatus {
	health.mu.Lock()
	defer health.mu.Unlock()
	return health.status
}

type controllableStopper struct {
	health          *recordingHealthStatus
	blockGraceful   bool
	gracefulStarted chan struct{}
	stop            chan struct{}
	stopOnce        sync.Once
	stopCalls       int
}

func newControllableStopper(health *recordingHealthStatus, blockGraceful bool) *controllableStopper {
	return &controllableStopper{
		health:          health,
		blockGraceful:   blockGraceful,
		gracefulStarted: make(chan struct{}),
		stop:            make(chan struct{}),
	}
}

func (server *controllableStopper) GracefulStop() {
	if got := server.health.current(); got != healthpb.HealthCheckResponse_NOT_SERVING {
		panic("GracefulStop started before NOT_SERVING")
	}
	close(server.gracefulStarted)
	if server.blockGraceful {
		<-server.stop
	}
}

func (server *controllableStopper) Stop() {
	server.stopCalls++
	server.stopOnce.Do(func() {
		close(server.stop)
	})
}
