package security

import (
	"fmt"

	seccomp "github.com/seccomp/libseccomp-golang"
	"golang.org/x/sys/unix"
)

// blockedPrivilegeSyscalls are never required by a contestant process. The
// sandbox runs language tools without capabilities, and seccomp is a second
// boundary that prevents regaining host control through privileged kernel
// APIs even when the worker supervisor itself is privileged.
var blockedPrivilegeSyscalls = []string{
	"acct",
	"add_key",
	"bpf",
	"chroot",
	"delete_module",
	"fanotify_init",
	"finit_module",
	"init_module",
	"ioperm",
	"iopl",
	"kcmp",
	"kexec_file_load",
	"kexec_load",
	"keyctl",
	"lookup_dcookie",
	"mount",
	"move_mount",
	"name_to_handle_at",
	"open_by_handle_at",
	"open_tree",
	"perf_event_open",
	"pivot_root",
	"process_vm_readv",
	"process_vm_writev",
	"ptrace",
	"quotactl",
	"reboot",
	"request_key",
	"setdomainname",
	"sethostname",
	"setns",
	"swapoff",
	"swapon",
	"umount",
	"umount2",
	"unshare",
	"userfaultfd",
}

// ApplySeccompFilters applies a fail-closed deny policy to the calling
// process. It must only be called by the short-lived sandbox-exec child after
// the supervisor has placed that child in its request cgroup. The filter is
// inherited across execve by the contestant program.
func ApplySeccompFilters(profile *SecurityProfile) error {
	if profile == nil {
		return fmt.Errorf("security profile is required")
	}
	if profile.SeccompMode == "disabled" {
		return fmt.Errorf("seccomp cannot be disabled for contestant execution")
	}

	filter, err := seccomp.NewFilter(seccomp.ActAllow)
	if err != nil {
		return fmt.Errorf("create seccomp filter: %w", err)
	}
	if err := filter.SetNoNewPrivsBit(true); err != nil {
		return fmt.Errorf("set seccomp no-new-privileges bit: %w", err)
	}
	deniedAction := seccomp.ActErrno.SetReturnCode(int16(unix.EPERM))

	for _, syscallName := range blockedPrivilegeSyscalls {
		syscallID, lookupErr := seccomp.GetSyscallFromName(syscallName)
		if lookupErr != nil {
			// Syscall tables differ between supported CPU architectures.
			continue
		}
		if err := filter.AddRule(syscallID, deniedAction); err != nil {
			return fmt.Errorf("block syscall %s: %w", syscallName, err)
		}
	}

	if profile.DisableNetwork {
		socketCall, err := seccomp.GetSyscallFromName("socket")
		if err != nil {
			return fmt.Errorf("resolve socket syscall: %w", err)
		}
		if err := filter.AddRule(socketCall, deniedAction); err != nil {
			return fmt.Errorf("block sockets: %w", err)
		}
	}

	if err := filter.Load(); err != nil {
		return fmt.Errorf("load seccomp filter: %w", err)
	}
	return nil
}

// GetDefaultAllowedSyscalls remains for source compatibility with callers
// that construct SecurityProfile values. Enforcement is deny-based because
// the five supported language runtimes have materially different evolving
// syscall surfaces; privileged and network syscalls are still explicitly
// blocked above.
func GetDefaultAllowedSyscalls() []string {
	return nil
}
