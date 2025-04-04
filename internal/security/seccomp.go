package security

import (
	"fmt"
	"log"

	"github.com/seccomp/libseccomp-golang"
	"github.com/CodeRushOJ/croj-sandbox/internal/util"
)

// ApplySeccompFilters 应用seccomp系统调用过滤器
func ApplySeccompFilters(profile *SecurityProfile) error {
	// 默认设置为拒绝所有系统调用
	defaultAction := seccomp.ActErrno
	if profile.SeccompMode == "strict" {
		defaultAction = seccomp.ActKill // 更严格的模式：直接终止进程
	}
	
	// 创建seccomp过滤器
	filter, err := seccomp.NewFilter(defaultAction)
	if err != nil {
		return fmt.Errorf("创建seccomp过滤器失败: %w", err)
	}
	
	// 允许的系统调用列表
	allowedSyscalls := profile.AllowedSyscalls
	if len(allowedSyscalls) == 0 {
		// 如果未指定，使用默认安全列表
		allowedSyscalls = GetDefaultAllowedSyscalls()
	}
	
	// 添加白名单系统调用
	for _, syscallName := range allowedSyscalls {
		syscallID, err := seccomp.GetSyscallFromName(syscallName)
		if err != nil {
			util.WarnLog("系统调用不存在: %s (忽略)", syscallName)
			continue
		}
		
		if err := filter.AddRule(syscallID, seccomp.ActAllow); err != nil {
			util.WarnLog("添加系统调用规则失败 %s: %v", syscallName, err)
		}
	}
	
	// 特殊处理：添加对socket系统调用的精细控制
	if profile.DisableNetwork {
		// 允许socket但有条件限制
		socketCall, err := seccomp.GetSyscallFromName("socket")
		if err == nil {
			// 仅允许本地通信的socket类型
			filter.AddRuleConditional(
				socketCall,
				seccomp.ActAllow,
				[]seccomp.ScmpCondition{
					{
						Argument: 0,
						Op:       seccomp.CompareMaskedEqual,
						Operand1: 1, // AF_UNIX
						Operand2: 0xFFFFFFFF,
					},
				},
			)
			
			filter.AddRuleConditional(
				socketCall,
				seccomp.ActErrno,
				[]seccomp.ScmpCondition{
					{
						Argument: 0,
						Op:       seccomp.CompareNotEqual,
						Operand1: 1, // 只允许AF_UNIX
						Operand2: 0xFFFFFFFF,
					},
				},
			)
		}
	}
	
	// 如果禁止执行其他程序
	if profile.DisableExec {
		execveSyscall, err := seccomp.GetSyscallFromName("execve")
		if err == nil {
			filter.AddRule(execveSyscall, seccomp.ActErrno)
		}
		
		execveatSyscall, err := seccomp.GetSyscallFromName("execveat")
		if err == nil {
			filter.AddRule(execveatSyscall, seccomp.ActErrno)
		}
	}
	
	// 加载seccomp过滤器
	if err := filter.Load(); err != nil {
		return fmt.Errorf("加载seccomp过滤器失败: %w", err)
	}
	
	return nil
}

// GetDefaultAllowedSyscalls 返回默认允许的系统调用列表
func GetDefaultAllowedSyscalls() []string {
	// 这是一个比较安全的系统调用白名单
	return []string{
		// 常规I/O操作
		"read", "write", "close", "fstat", "lseek", "mmap", "mprotect", "munmap", "brk",
		"readv", "writev", "pread64", "pwrite64", "lstat", "readlink",
		
		// 文件操作
		"access", "open", "openat", "stat", "getcwd", "fcntl",
		"fstatfs", "getdents", "getdents64", "readdir", "rename", "unlink", "rmdir", 
		"mkdir", "link", "chmod", "truncate", "fallocate", "utime", "chdir", "dup", "dup2", "pipe",
		
		// 进程管理
		"clone", "fork", "vfork", "wait4", "kill", "exit", "exit_group", 
		"rt_sigreturn", "rt_sigaction", "rt_sigprocmask", "rt_sigqueueinfo",
		"setitimer", "getitimer", "nanosleep", "clock_gettime", "sched_yield",
		
		// 内存管理
		"mremap", "msync", "mincore", "madvise", "shmget", "shmat", "shmdt", "shmctl",
		
		// 资源信息
		"getrusage", "getrlimit", "getpriority", "getuid", "geteuid", "getgid", "getegid",
		"gettid", "getpid", "getppid", "gettimeofday", "uname", "getrandom",
		
		// 套接字（受条件控制）
		"socket", "socketpair", "bind", "listen", "accept", "accept4", "connect",
		
		// 其他必要调用
		"futex", "epoll_create", "epoll_create1", "epoll_ctl", "epoll_wait", "epoll_pwait",
		"select", "poll", "timerfd_create", "timerfd_settime", "timerfd_gettime",
	}
}
