package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/CodeRushOJ/croj-sandbox/internal/sandbox"
	"github.com/CodeRushOJ/croj-sandbox/internal/security"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

var languageCommands = map[string][]string{
	"go":         {"go"},
	"cpp":        {"g++"},
	"python":     {"python3"},
	"java":       {"javac", "java"},
	"javascript": {"node"},
}

func markServingAfterCheck(healthServer *health.Server, check func() error) error {
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	if err := check(); err != nil {
		return err
	}
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	return nil
}

func checkStartupDependencies(cfg sandbox.Config, supportedLanguages []string) error {
	if err := checkLanguageCommands(supportedLanguages); err != nil {
		return err
	}
	if err := checkWritableTempDir(cfg.HostTempDir); err != nil {
		return err
	}
	if err := checkCgroupWriteAccess(); err != nil {
		return err
	}
	return nil
}

func checkLanguageCommands(supportedLanguages []string) error {
	if len(supportedLanguages) == 0 {
		return fmt.Errorf("no sandbox languages configured")
	}
	for _, language := range supportedLanguages {
		language = strings.TrimSpace(language)
		commands, ok := languageCommands[language]
		if !ok {
			return fmt.Errorf("unsupported configured language %q", language)
		}
		for _, command := range commands {
			if _, err := exec.LookPath(command); err != nil {
				return fmt.Errorf("language %s requires %s: %w", language, command, err)
			}
		}
	}
	return nil
}

func checkWritableTempDir(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create sandbox temp directory %s: %w", path, err)
	}
	probe, err := os.CreateTemp(path, ".startup-check-")
	if err != nil {
		return fmt.Errorf("sandbox temp directory %s is not writable: %w", path, err)
	}
	probePath := probe.Name()
	if err := probe.Close(); err != nil {
		return fmt.Errorf("close sandbox temp probe: %w", err)
	}
	if err := os.Remove(probePath); err != nil {
		return fmt.Errorf("remove sandbox temp probe: %w", err)
	}
	return nil
}

func checkCgroupWriteAccess() error {
	probe := exec.Command("sleep", "30")
	if err := probe.Start(); err != nil {
		return fmt.Errorf("start cgroup probe process: %w", err)
	}

	profile := security.NewDefaultSecurityProfile()
	profile.MemoryLimitBytes = 64 * 1024 * 1024
	profile.CPULimit = 50
	profile.PidsLimit = 16
	manager, setupErr := security.SetupCgroups(
		security.CgroupIDForProcess("startup", probe.Process.Pid),
		probe.Process.Pid,
		profile,
	)
	_ = probe.Process.Kill()
	_ = probe.Wait()
	if setupErr != nil {
		return fmt.Errorf("exercise cgroup execution path: %w", setupErr)
	}
	if err := security.CleanupCgroups(manager); err != nil {
		return fmt.Errorf("clean up cgroup startup probe: %w", err)
	}
	return nil
}
