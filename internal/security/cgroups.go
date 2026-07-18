package security

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/CodeRushOJ/croj-sandbox/internal/util"
)

// CgroupIDForProcess returns a host-unique, path-safe cgroup name. Kubernetes
// injects the Pod UID so equal namespace-local PIDs in different Pods cannot
// share a host cgroup.
func CgroupIDForProcess(prefix string, pid int) string {
	instanceID := os.Getenv("CROJ_SANDBOX_INSTANCE_ID")
	if instanceID == "" {
		instanceID, _ = os.Hostname()
	}
	raw := fmt.Sprintf("%s_%s_%d", prefix, instanceID, pid)
	return strings.Map(func(character rune) rune {
		if character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '-' || character == '_' {
			return character
		}
		return '_'
	}, raw)
}

// SetupCgroups 设置cgroup资源限制
func SetupCgroups(cgroupID string, pid int, profile *SecurityProfile) (*CgroupManager, error) {
	// 判断使用v1还是v2版本的cgroup
	cgroupVersion := detectCgroupVersion()
	util.DebugLog("检测到cgroup版本: %d", cgroupVersion)

	var manager *CgroupManager
	var err error

	if cgroupVersion == 2 {
		manager, err = setupCgroupsV2(cgroupID, pid, profile)
	} else {
		manager, err = setupCgroupsV1(cgroupID, pid, profile)
	}

	if err != nil {
		return nil, err
	}

	return manager, nil
}

// CleanupCgroups 清理cgroup资源
func CleanupCgroups(manager *CgroupManager) error {
	if manager == nil || !manager.Initialized {
		return nil
	}

	// 删除cgroup目录
	util.DebugLog("清理cgroup: %s", manager.GroupID)

	if manager.Version == 2 {
		return cleanupCgroupV2(manager)
	}
	return cleanupCgroupV1(manager)
}

// detectCgroupVersion 检测系统使用的cgroup版本
func detectCgroupVersion() int {
	// 检查cgroup v2挂载点
	if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err == nil {
		return 2
	}

	// 检查cgroup v1挂载点
	if _, err := os.Stat("/sys/fs/cgroup/memory"); err == nil {
		return 1
	}

	// 默认假设为v1
	return 1
}

// setupCgroupsV1 配置cgroup v1资源限制
func setupCgroupsV1(cgroupID string, pid int, profile *SecurityProfile) (*CgroupManager, error) {
	manager := &CgroupManager{
		GroupID: cgroupID,
		Version: 1,
	}

	// 创建内存控制器
	memCgroupPath := filepath.Join("/sys/fs/cgroup/memory", "croj", cgroupID)
	if err := os.MkdirAll(memCgroupPath, 0755); err != nil {
		return nil, fmt.Errorf("创建内存cgroup失败: %w", err)
	}

	// 创建CPU控制器
	cpuCgroupPath := filepath.Join("/sys/fs/cgroup/cpu", "croj", cgroupID)
	if err := os.MkdirAll(cpuCgroupPath, 0755); err != nil {
		return nil, fmt.Errorf("创建CPU cgroup失败: %w", err)
	}

	// 创建pids控制器
	pidsCgroupPath := filepath.Join("/sys/fs/cgroup/pids", "croj", cgroupID)
	if err := os.MkdirAll(pidsCgroupPath, 0755); err != nil {
		return nil, fmt.Errorf("创建pids cgroup失败: %w", err)
	}

	// 设置内存限制
	if profile.MemoryLimitBytes > 0 {
		memLimitPath := filepath.Join(memCgroupPath, "memory.limit_in_bytes")
		if err := os.WriteFile(memLimitPath, []byte(fmt.Sprintf("%d", profile.MemoryLimitBytes)), 0644); err != nil {
			return nil, fmt.Errorf("设置内存限制失败: %w", err)
		}

		// 禁用内存交换，确保更准确的内存限制
		swapLimitPath := filepath.Join(memCgroupPath, "memory.swappiness")
		if err := os.WriteFile(swapLimitPath, []byte("0"), 0644); err != nil {
			util.WarnLog("设置内存交换限制失败: %v", err)
		}
	}

	// 设置CPU限制
	if profile.CPULimit > 0 && profile.CPULimit <= 100 {
		// CPU配额（微秒）：100000表示一个核心的100%
		cpuQuota := profile.CPULimit * 1000
		cpuQuotaPath := filepath.Join(cpuCgroupPath, "cpu.cfs_quota_us")
		if err := os.WriteFile(cpuQuotaPath, []byte(fmt.Sprintf("%d", cpuQuota)), 0644); err != nil {
			return nil, fmt.Errorf("设置CPU配额失败: %w", err)
		}

		// CPU周期（微秒）：默认100000
		cpuPeriodPath := filepath.Join(cpuCgroupPath, "cpu.cfs_period_us")
		if err := os.WriteFile(cpuPeriodPath, []byte("100000"), 0644); err != nil {
			return nil, fmt.Errorf("设置CPU周期失败: %w", err)
		}
	}

	// 设置进程数限制
	if profile.PidsLimit > 0 {
		pidsMaxPath := filepath.Join(pidsCgroupPath, "pids.max")
		if err := os.WriteFile(pidsMaxPath, []byte(fmt.Sprintf("%d", profile.PidsLimit)), 0644); err != nil {
			return nil, fmt.Errorf("设置进程数限制失败: %w", err)
		}
	}

	// 将进程加入到cgroup
	pidStr := strconv.Itoa(pid)

	// 添加到内存控制器
	memTasksPath := filepath.Join(memCgroupPath, "tasks")
	if err := os.WriteFile(memTasksPath, []byte(pidStr), 0644); err != nil {
		return nil, fmt.Errorf("将进程添加到内存cgroup失败: %w", err)
	}

	// 添加到CPU控制器
	cpuTasksPath := filepath.Join(cpuCgroupPath, "tasks")
	if err := os.WriteFile(cpuTasksPath, []byte(pidStr), 0644); err != nil {
		return nil, fmt.Errorf("将进程添加到CPU cgroup失败: %w", err)
	}

	// 添加到pids控制器
	pidsTasksPath := filepath.Join(pidsCgroupPath, "tasks")
	if err := os.WriteFile(pidsTasksPath, []byte(pidStr), 0644); err != nil {
		return nil, fmt.Errorf("将进程添加到pids cgroup失败: %w", err)
	}

	manager.BasePath = "/sys/fs/cgroup"
	manager.Initialized = true

	return manager, nil
}

// setupCgroupsV2 配置cgroup v2资源限制
func setupCgroupsV2(cgroupID string, pid int, profile *SecurityProfile) (*CgroupManager, error) {
	manager := &CgroupManager{
		GroupID: cgroupID,
		Version: 2,
	}

	parentPath := filepath.Join("/sys/fs/cgroup", "croj")
	if err := os.MkdirAll(parentPath, 0755); err != nil {
		return nil, fmt.Errorf("创建cgroup v2父目录失败: %w", err)
	}
	controllers, err := os.ReadFile(filepath.Join(parentPath, "cgroup.controllers"))
	if err != nil {
		return nil, fmt.Errorf("读取cgroup v2控制器失败: %w", err)
	}
	available := make(map[string]bool)
	for _, controller := range strings.Fields(string(controllers)) {
		available[controller] = true
	}
	for _, required := range []string{"memory", "cpu", "pids"} {
		if !available[required] {
			return nil, fmt.Errorf("缺少cgroup v2控制器: %s", required)
		}
	}
	if err := os.WriteFile(
		filepath.Join(parentPath, "cgroup.subtree_control"),
		[]byte("+memory +cpu +pids"),
		0644,
	); err != nil {
		return nil, fmt.Errorf("委派cgroup v2控制器失败: %w", err)
	}

	cgroupPath := filepath.Join(parentPath, cgroupID)
	if err := os.MkdirAll(cgroupPath, 0755); err != nil {
		return nil, fmt.Errorf("创建cgroup v2目录失败: %w", err)
	}
	initialized := false
	defer func() {
		if !initialized {
			_ = os.Remove(cgroupPath)
		}
	}()

	// 设置内存限制
	if profile.MemoryLimitBytes > 0 {
		memLimitPath := filepath.Join(cgroupPath, "memory.max")
		if err := os.WriteFile(memLimitPath, []byte(fmt.Sprintf("%d", profile.MemoryLimitBytes)), 0644); err != nil {
			return nil, fmt.Errorf("设置内存限制失败: %w", err)
		}

		// 禁用内存交换
		swapLimitPath := filepath.Join(cgroupPath, "memory.swap.max")
		if err := os.WriteFile(swapLimitPath, []byte("0"), 0644); err != nil {
			util.WarnLog("设置内存交换限制失败: %v", err)
		}
	}

	// 设置CPU限制
	if profile.CPULimit > 0 && profile.CPULimit <= 100 {
		// CPU配额：100000表示一个核心的100%
		cpuQuota := profile.CPULimit * 1000
		cpuMaxPath := filepath.Join(cgroupPath, "cpu.max")
		if err := os.WriteFile(cpuMaxPath, []byte(fmt.Sprintf("%d 100000", cpuQuota)), 0644); err != nil {
			return nil, fmt.Errorf("设置CPU限制失败: %w", err)
		}
	}

	// 设置进程数限制
	if profile.PidsLimit > 0 {
		pidsMaxPath := filepath.Join(cgroupPath, "pids.max")
		if err := os.WriteFile(pidsMaxPath, []byte(fmt.Sprintf("%d", profile.PidsLimit)), 0644); err != nil {
			return nil, fmt.Errorf("设置进程数限制失败: %w", err)
		}
	}

	// 将进程加入到cgroup
	procsPath := filepath.Join(cgroupPath, "cgroup.procs")
	if err := os.WriteFile(procsPath, []byte(strconv.Itoa(pid)), 0644); err != nil {
		return nil, fmt.Errorf("将进程添加到cgroup失败: %w", err)
	}

	manager.BasePath = "/sys/fs/cgroup"
	manager.Initialized = true
	initialized = true

	return manager, nil
}

// cleanupCgroupV1 清理cgroup v1资源
func cleanupCgroupV1(manager *CgroupManager) error {
	// 在V1中，需要分别清理各个子系统
	controllers := []string{"memory", "cpu", "pids"}

	for _, controller := range controllers {
		cgroupPath := filepath.Join("/sys/fs/cgroup", controller, "croj", manager.GroupID)

		// 尝试删除目录
		if err := os.Remove(cgroupPath); err != nil && !os.IsNotExist(err) {
			util.WarnLog("清理cgroup控制器目录失败 %s: %v", controller, err)
		}
	}

	return nil
}

// cleanupCgroupV2 清理cgroup v2资源
func cleanupCgroupV2(manager *CgroupManager) error {
	// V2只需要删除一个目录
	cgroupPath := filepath.Join("/sys/fs/cgroup", "croj", manager.GroupID)

	// 尝试删除目录
	if err := os.Remove(cgroupPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("清理cgroup v2目录失败: %w", err)
	}

	return nil
}
