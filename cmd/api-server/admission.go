package main

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"sync"
	"sync/atomic"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type admissionStats struct {
	capacity int
	inFlight int64
	rejected int64
}

type executionLimiter struct {
	capacity int
	slots    chan struct{}
	rejected atomic.Int64
}

func defaultMaxConcurrency() int {
	capacity := runtime.GOMAXPROCS(0)
	if capacity < 1 {
		return 1
	}
	return capacity
}

func newExecutionLimiter(capacity int) (*executionLimiter, error) {
	if capacity <= 0 {
		return nil, fmt.Errorf("max-concurrency must be greater than zero, got %d", capacity)
	}
	return &executionLimiter{
		capacity: capacity,
		slots:    make(chan struct{}, capacity),
	}, nil
}

func (l *executionLimiter) tryAcquire() (release func(), admitted bool) {
	select {
	case l.slots <- struct{}{}:
		var once sync.Once
		return func() {
			once.Do(func() {
				<-l.slots
			})
		}, true
	default:
		l.rejected.Add(1)
		return nil, false
	}
}

func (l *executionLimiter) snapshot() admissionStats {
	return admissionStats{
		capacity: l.capacity,
		inFlight: int64(len(l.slots)),
		rejected: l.rejected.Load(),
	}
}

func recoveryUnaryServerInterceptor(
	ctx context.Context,
	req any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (response any, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("sandbox handler panic recovered method=%s", info.FullMethod)
			response = nil
			err = status.Error(codes.Internal, "internal sandbox error")
		}
	}()
	return handler(ctx, req)
}
