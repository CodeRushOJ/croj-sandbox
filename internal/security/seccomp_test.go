package security

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func TestSeccompPolicyDeniesIOUringSyscalls(t *testing.T) {
	blocked := make(map[string]bool, len(blockedPrivilegeSyscalls))
	for _, name := range blockedPrivilegeSyscalls {
		blocked[name] = true
	}
	for _, name := range []string{"io_uring_setup", "io_uring_register", "io_uring_enter"} {
		if !blocked[name] {
			t.Errorf("%s is not denied", name)
		}
	}
}

func TestSeccompBlocksIOUringEntryPoints(t *testing.T) {
	if os.Getenv("CROJ_SECCOMP_IO_URING_PROBE") == "1" {
		profile := NewDefaultSecurityProfile()
		profile.SeccompMode = "filtered"
		if err := ApplySeccompFilters(profile); err != nil {
			os.Exit(80)
		}
		_, _, setupErrno := unix.Syscall(unix.SYS_IO_URING_SETUP, 0, 0, 0)
		if setupErrno != syscall.EPERM {
			os.Exit(81)
		}
		_, _, registerErrno := unix.Syscall6(unix.SYS_IO_URING_REGISTER, ^uintptr(0), 0, 0, 0, 0, 0)
		if registerErrno != syscall.EPERM {
			os.Exit(82)
		}
		_, _, enterErrno := unix.Syscall6(unix.SYS_IO_URING_ENTER, ^uintptr(0), 0, 0, 0, 0, 0)
		if enterErrno != syscall.EPERM {
			os.Exit(83)
		}
		os.Exit(0)
	}

	command := exec.Command(os.Args[0], "-test.run=^TestSeccompBlocksIOUringEntryPoints$")
	command.Env = append(os.Environ(), "CROJ_SECCOMP_IO_URING_PROBE=1")
	if err := command.Run(); err != nil {
		t.Fatalf("seccomp io_uring child probe failed: %v", err)
	}
}

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
