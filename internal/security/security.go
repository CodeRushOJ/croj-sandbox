package security

import (
	"fmt"
	"os"
	"path/filepath"
	
	"github.com/CodeRushOJ/croj-sandbox/internal/util"
)

// SecurityProfile 定义进程安全配置
type SecurityProfile struct {
	// Seccomp相关设置
	SeccompMode       string   // seccomp模式: strict, filtered, disabled
	AllowedSyscalls   []string // 允许的系统调用白名单
	BlockedSyscalls   []string // 拒绝的系统调用黑名单
	
	// Cgroups相关设置
	EnableCgroups     bool     // 是否启用cgroups
	MemoryLimitBytes  int64    // 内存限制 (字节)
	CPULimit          int      // CPU限制 (%)
	PidsLimit         int      // 最大进程/线程数
	
	// 网络和文件系统限制
	DisableNetwork    bool     // 禁用所有网络访问
	ReadOnlyPaths     []string // 只读目录列表
	WritablePaths     []string // 可写目录列表
	HiddenPaths       []string // 对进程隐藏的路径
	
	// 其他安全选项
	NoNewPrivileges   bool     // 防止获取新权限
	DisableExec       bool     // 禁止执行其他程序
}

// CgroupManager 管理cgroup资源
type CgroupManager struct {
	BasePath    string // cgroup文件系统基础路径
	GroupID     string // 当前cgroup组ID
	Initialized bool   // 是否已初始化
}

// NewDefaultSecurityProfile 返回默认安全配置
func NewDefaultSecurityProfile() *SecurityProfile {
	return &SecurityProfile{
		SeccompMode:     "filtered",
		EnableCgroups:   true,
		PidsLimit:       64,        // 最多64个进程/线程
		DisableNetwork:  true,      // 禁止网络访问
		NoNewPrivileges: true,      // 禁止获取新权限
		DisableExec:     true,      // 禁止运行其他程序
		ReadOnlyPaths: []string{
			"/usr", "/lib", "/lib64", "/bin", "/sbin", 
			"/etc/ssl", "/etc/passwd", "/etc/group", 
			"/etc/resolv.conf",
		},
		WritablePaths: []string{
			"/tmp",
		},
		HiddenPaths: []string{
			"/etc/shadow", "/root", "/home",
			"/proc/kcore", "/proc/keys",
		},
	}
}

// ProfileForLanguage 根据编程语言返回合适的安全配置
func ProfileForLanguage(language string) *SecurityProfile {
	profile := NewDefaultSecurityProfile()
	
	// 配置系统调用白名单
	profile.AllowedSyscalls = GetDefaultAllowedSyscalls()
	
	// 根据语言特点调整配置
	switch language {
	case "python":
		// Python需要创建更多子进程和动态加载库
		profile.PidsLimit = 128
		profile.ReadOnlyPaths = append(profile.ReadOnlyPaths, 
			"/usr/lib/python*", "/usr/local/lib/python*")
	
	case "java":
		// Java需要更多资源和创建子进程的能力
		profile.PidsLimit = 256
		profile.ReadOnlyPaths = append(profile.ReadOnlyPaths, 
			"/usr/lib/jvm", "/etc/java*")

	case "go":
		// Go程序通常更加独立，可以应用更严格的限制
		profile.SeccompMode = "strict"
	}
	
	return profile
}

// SetupSecurity 设置所有安全机制
func SetupSecurity(profile *SecurityProfile, pid int, runDir string) error {
	// 创建唯一的cgroup ID
	cgroupID := fmt.Sprintf("croj_sandbox_%d", pid)
	
	// 设置cgroup资源限制
	if profile.EnableCgroups {
		manager, err := SetupCgroups(cgroupID, pid, profile)
		if err != nil {
			util.ErrorLog("设置cgroup失败: %v", err)
			return err
		}
		
		// 保存cgroup管理器，以便后续清理
		cgroupManager := manager
		util.DebugLog("已设置cgroup限制: %s", cgroupID)
		
		// 注册清理函数
		RegisterCleanupHandler(func() {
			if err := CleanupCgroups(cgroupManager); err != nil {
				util.ErrorLog("清理cgroup失败: %v", err)
			}
		})
	}
	
	// 应用seccomp系统调用过滤
	if profile.SeccompMode != "disabled" {
		if err := ApplySeccompFilters(profile); err != nil {
			util.ErrorLog("设置seccomp过滤器失败: %v", err)
			return err
		}
		util.DebugLog("已应用seccomp过滤器, 模式: %s", profile.SeccompMode)
	}
	
	return nil
}

// CreateNamespace 创建隔离的命名空间
func CreateNamespace(cmd *os.Process, profile *SecurityProfile) error {
	// 在Linux上，我们可以为进程创建隔离的命名空间
	// 这需要在进程开始前设置
	return nil
}

// RegisterCleanupHandler 注册资源清理处理程序
func RegisterCleanupHandler(handler func()) {
	// 保存清理函数，以便在执行结束后调用
	cleanupHandlers = append(cleanupHandlers, handler)
}

// 保存所有需要执行的清理函数
var cleanupHandlers []func()

// Cleanup 执行所有注册的清理函数
func Cleanup() {
	for _, handler := range cleanupHandlers {
		handler()
	}
	cleanupHandlers = nil
}
