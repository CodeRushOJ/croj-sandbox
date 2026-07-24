package security

// SecurityProfile 定义进程安全配置
type SecurityProfile struct {
	// Seccomp相关设置
	SeccompMode     string   // seccomp模式: strict, filtered, disabled
	AllowedSyscalls []string // 允许的系统调用白名单
	BlockedSyscalls []string // 拒绝的系统调用黑名单

	// Cgroups相关设置
	EnableCgroups    bool  // 是否启用cgroups
	MemoryLimitBytes int64 // 内存限制 (字节)
	CPULimit         int   // CPU限制 (%)
	PidsLimit        int   // 最大进程/线程数

	// 网络和文件系统限制
	DisableNetwork bool     // 禁用所有网络访问
	ReadOnlyPaths  []string // 只读目录列表
	WritablePaths  []string // 可写目录列表
	HiddenPaths    []string // 对进程隐藏的路径

	// 其他安全选项
	NoNewPrivileges bool // 防止获取新权限
	DisableExec     bool // 禁止执行其他程序
}

// CgroupManager 管理cgroup资源
type CgroupManager struct {
	BasePath    string // cgroup文件系统基础路径
	GroupID     string // 当前cgroup组ID
	Version     int    // cgroup version (1 or 2)
	Initialized bool   // 是否已初始化
}

// NewDefaultSecurityProfile 返回默认安全配置
func NewDefaultSecurityProfile() *SecurityProfile {
	return &SecurityProfile{
		SeccompMode:     "filtered",
		EnableCgroups:   true,
		PidsLimit:       64,   // 最多64个进程/线程
		DisableNetwork:  true, // 禁止网络访问
		NoNewPrivileges: true, // 禁止获取新权限
		DisableExec:     true, // 禁止运行其他程序
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
