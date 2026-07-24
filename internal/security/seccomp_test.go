package security

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func TestSeccompBlocksAllSocketCreation(t *testing.T) {
	if os.Getenv("CROJ_SECCOMP_SOCKET_PROBE") == "1" {
		profile := NewDefaultSecurityProfile()
		profile.SeccompMode = "filtered"
		if err := ApplySeccompFilters(profile); err != nil {
			os.Exit(90)
		}
		if descriptor, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0); err == nil {
			_ = unix.Close(descriptor)
			os.Exit(91)
		} else if !errors.Is(err, syscall.EPERM) {
			os.Exit(92)
		}
		if descriptor, err := unix.Socket(unix.AF_UNIX, unix.SOCK_STREAM, 0); err == nil {
			_ = unix.Close(descriptor)
			os.Exit(93)
		} else if !errors.Is(err, syscall.EPERM) {
			os.Exit(94)
		}
		os.Exit(0)
	}

	command := exec.Command(os.Args[0], "-test.run=^TestSeccompBlocksAllSocketCreation$")
	command.Env = append(os.Environ(), "CROJ_SECCOMP_SOCKET_PROBE=1")
	if err := command.Run(); err != nil {
		t.Fatalf("seccomp child probe failed: %v", err)
	}
}
