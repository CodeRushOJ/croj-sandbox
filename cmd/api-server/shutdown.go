package main

import (
	"context"

	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

type healthStatusSetter interface {
	SetServingStatus(string, healthpb.HealthCheckResponse_ServingStatus)
}

type grpcStopper interface {
	GracefulStop()
	Stop()
}

func boundedShutdown(ctx context.Context, health healthStatusSetter, server grpcStopper) bool {
	health.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	gracefulDone := make(chan struct{})
	go func() {
		server.GracefulStop()
		close(gracefulDone)
	}()

	select {
	case <-gracefulDone:
		return true
	case <-ctx.Done():
		server.Stop()
		<-gracefulDone
		return false
	}
}
