package util

import (
	"os/exec"
	"testing"
	"time"
)

func TestMonitorProcessReturnsFinalStatsAfterProcessExit(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 0.05")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start short process: %v", err)
	}

	done := make(chan struct{})
	waited := make(chan error, 1)
	go func() {
		waited <- cmd.Wait()
		close(done)
	}()

	start := time.Now()
	stats := MonitorProcess(cmd.Process.Pid, 0, time.Second, time.Millisecond, done)
	elapsed := time.Since(start)

	readProcessStatsFor(stats, 20*time.Millisecond)

	if err := <-waited; err != nil {
		t.Fatalf("short process failed: %v", err)
	}
	if elapsed < 20*time.Millisecond {
		t.Fatalf("MonitorProcess returned before the process exited: %v", elapsed)
	}
	if stats.IsTimeout {
		t.Fatal("short process was incorrectly reported as timed out")
	}
	if stats.Duration <= 0 {
		t.Fatalf("expected a final duration, got %v", stats.Duration)
	}
}

func TestMonitorProcessReturnsFinalStatsAfterTimeout(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 5")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start timeout process: %v", err)
	}

	done := make(chan struct{})
	waited := make(chan error, 1)
	go func() {
		waited <- cmd.Wait()
		close(done)
	}()

	start := time.Now()
	stats := MonitorProcess(cmd.Process.Pid, 0, 30*time.Millisecond, time.Millisecond, done)
	elapsed := time.Since(start)

	readProcessStatsFor(stats, 20*time.Millisecond)

	select {
	case <-waited:
	case <-time.After(time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("timed-out process was not terminated")
	}
	if elapsed < 15*time.Millisecond {
		t.Fatalf("MonitorProcess returned before its timer fired: %v", elapsed)
	}
	if !stats.IsTimeout {
		t.Fatal("expected the final snapshot to report a timeout")
	}
	if stats.Duration < 15*time.Millisecond {
		t.Fatalf("expected timeout duration in final snapshot, got %v", stats.Duration)
	}
}

func TestMonitorProcessReturnsFinalStatsAfterDoneCloses(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 5")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start cancellable process: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	done := make(chan struct{})
	go func() {
		time.Sleep(30 * time.Millisecond)
		close(done)
	}()

	start := time.Now()
	stats := MonitorProcess(cmd.Process.Pid, 0, time.Second, time.Millisecond, done)
	elapsed := time.Since(start)

	readProcessStatsFor(stats, 20*time.Millisecond)

	if elapsed < 15*time.Millisecond {
		t.Fatalf("MonitorProcess returned before done closed: %v", elapsed)
	}
	if stats.IsTimeout {
		t.Fatal("done closure was incorrectly reported as a timeout")
	}
	if stats.Duration < 15*time.Millisecond {
		t.Fatalf("expected cancellation duration in final snapshot, got %v", stats.Duration)
	}
}

func readProcessStatsFor(stats *ProcessStats, duration time.Duration) {
	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		_ = *stats
	}
}
