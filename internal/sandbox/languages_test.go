package sandbox

import (
	"testing"
	"time"
)

func TestDefaultGoCompileBudgetCoversConcurrentColdBuilds(t *testing.T) {
	cfg := DefaultConfig()
	goConfig := cfg.Languages["go"]

	if got, want := goConfig.GetCompileTimeout(cfg.DefaultCompileTimeLimit), 90*time.Second; got != want {
		t.Fatalf("Go compile timeout = %v, want %v", got, want)
	}
}
