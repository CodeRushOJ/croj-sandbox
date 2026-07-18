package main

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/test/bufconn"
)

func TestNewGRPCServerStartsNotServing(t *testing.T) {
	server, _ := newGRPCServer(nil, []string{"go"})
	client := newHealthTestClient(t, server)

	assertHealthStatus(t, client, healthpb.HealthCheckResponse_NOT_SERVING)
}

func TestMarkServingAfterCheckPublishesServing(t *testing.T) {
	server, healthServer := newGRPCServer(nil, []string{"go"})
	client := newHealthTestClient(t, server)

	if err := markServingAfterCheck(healthServer, func() error { return nil }); err != nil {
		t.Fatalf("mark serving: %v", err)
	}
	assertHealthStatus(t, client, healthpb.HealthCheckResponse_SERVING)
}

func TestMarkServingAfterCheckKeepsNotServingOnFailure(t *testing.T) {
	server, healthServer := newGRPCServer(nil, []string{"go"})
	client := newHealthTestClient(t, server)

	wantErr := errors.New("cgroup unavailable")
	if err := markServingAfterCheck(healthServer, func() error { return wantErr }); !errors.Is(err, wantErr) {
		t.Fatalf("mark serving error = %v, want %v", err, wantErr)
	}
	assertHealthStatus(t, client, healthpb.HealthCheckResponse_NOT_SERVING)
}

func TestCgroupStartupCheckIntegration(t *testing.T) {
	if os.Getenv("CROJ_RUN_CGROUP_TEST") != "1" {
		t.Skip("set CROJ_RUN_CGROUP_TEST=1 in a privileged Linux test container")
	}
	if err := checkCgroupWriteAccess(); err != nil {
		t.Fatalf("cgroup startup check: %v", err)
	}
}

func newHealthTestClient(t *testing.T, server *grpc.Server) healthpb.HealthClient {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(server.Stop)

	connection, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial in-memory gRPC server: %v", err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	return healthpb.NewHealthClient(connection)
}

func assertHealthStatus(t *testing.T, client healthpb.HealthClient, want healthpb.HealthCheckResponse_ServingStatus) {
	t.Helper()
	response, err := client.Check(context.Background(), &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("health Check returned error: %v", err)
	}
	if response.Status != want {
		t.Fatalf("health status = %s, want %s", response.Status, want)
	}
}
