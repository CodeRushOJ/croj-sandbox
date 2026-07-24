package security

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

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
	cgroupVersion := detectCgroupVersion()
	util.DebugLog("检测到cgroup版本: %d", cgroupVersion)
	if cgroupVersion != 2 {
		return nil, fmt.Errorf("cgroup v2 is required for request isolation")
	}
	return setupCgroupsV2(cgroupID, pid, profile)
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

	parentPath, err := requestCgroupRoot()
	if err != nil {
		return nil, err
	}

	cgroupPath := filepath.Join(parentPath, cgroupID)
	if err := os.Mkdir(cgroupPath, 0700); err != nil {
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

	manager.BasePath = cgroupPath
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
	cgroupPath := manager.BasePath
	if cgroupPath == "" {
		return fmt.Errorf("cgroup manager path is missing")
	}

	// 尝试删除目录
	if err := os.Remove(cgroupPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("清理cgroup v2目录失败: %w", err)
	}

	return nil
}

var requestCgroupState struct {
	sync.Mutex
	root string
}

// requestCgroupRoot creates request cgroups below the worker Pod's own cgroup.
// Keeping both the supervisor and every contestant below this boundary
// preserves kubelet lifecycle and Pod-level resource enforcement.
func requestCgroupRoot() (string, error) {
	requestCgroupState.Lock()
	defer requestCgroupState.Unlock()
	if requestCgroupState.root != "" {
		return requestCgroupState.root, nil
	}

	membership, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return "", fmt.Errorf("read supervisor cgroup membership: %w", err)
	}
	podCgroup, err := resolveUnifiedCgroupPath("/sys/fs/cgroup", membership)
	if err != nil {
		return "", err
	}
	if filepath.Base(podCgroup) == ".croj-supervisor" {
		podCgroup = filepath.Dir(podCgroup)
	}

	supervisorPath := filepath.Join(podCgroup, ".croj-supervisor")
	jobsPath := filepath.Join(podCgroup, ".croj-jobs")
	if err := os.Mkdir(supervisorPath, 0700); err != nil && !os.IsExist(err) {
		return "", fmt.Errorf("create supervisor cgroup: %w", err)
	}
	if err := moveProcessesToSupervisor(podCgroup, supervisorPath); err != nil {
		return "", err
	}
	if err := enableControllers(podCgroup); err != nil {
		return "", err
	}
	if err := os.Mkdir(jobsPath, 0700); err != nil && !os.IsExist(err) {
		return "", fmt.Errorf("create request cgroup root: %w", err)
	}
	if err := enableControllers(jobsPath); err != nil {
		return "", err
	}
	requestCgroupState.root = jobsPath
	return jobsPath, nil
}

func moveProcessesToSupervisor(podCgroup string, supervisorPath string) error {
	source := filepath.Join(podCgroup, "cgroup.procs")
	target := filepath.Join(supervisorPath, "cgroup.procs")
	for attempt := 0; attempt < 10; attempt++ {
		processes, err := os.ReadFile(source)
		if err != nil {
			return fmt.Errorf("read Pod cgroup processes: %w", err)
		}
		pids := strings.Fields(string(processes))
		if len(pids) == 0 {
			return nil
		}
		for _, pid := range pids {
			if _, err := strconv.Atoi(pid); err != nil {
				return fmt.Errorf("invalid PID in Pod cgroup")
			}
			if err := os.WriteFile(target, []byte(pid), 0600); err != nil {
				return fmt.Errorf("move PID %s into supervisor cgroup: %w", pid, err)
			}
		}
	}
	return fmt.Errorf("Pod cgroup remained populated while enabling request controllers")
}

func enableControllers(path string) error {
	controllers, err := os.ReadFile(filepath.Join(path, "cgroup.controllers"))
	if err != nil {
		return fmt.Errorf("read available controllers for %s: %w", path, err)
	}
	available := make(map[string]bool)
	for _, controller := range strings.Fields(string(controllers)) {
		available[controller] = true
	}
	for _, required := range []string{"memory", "cpu", "pids"} {
		if !available[required] {
			return fmt.Errorf(
				"required cgroup v2 controller %s is unavailable below the Pod cgroup",
				required,
			)
		}
	}
	if err := os.WriteFile(
		filepath.Join(path, "cgroup.subtree_control"),
		[]byte("+memory +cpu +pids"),
		0600,
	); err != nil {
		return fmt.Errorf("delegate Pod cgroup v2 controllers: %w", err)
	}
	return nil
}

func resolveUnifiedCgroupPath(root string, membership []byte) (string, error) {
	for _, line := range strings.Split(strings.TrimSpace(string(membership)), "\n") {
		if !strings.HasPrefix(line, "0::/") {
			continue
		}
		relative := strings.TrimPrefix(line, "0::/")
		clean := filepath.Clean(relative)
		if clean == "." || clean == ".." || filepath.IsAbs(clean) ||
			strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("invalid unified cgroup path")
		}
		resolved := filepath.Join(root, clean)
		rootWithSeparator := filepath.Clean(root) + string(filepath.Separator)
		if !strings.HasPrefix(resolved+string(filepath.Separator), rootWithSeparator) {
			return "", fmt.Errorf("unified cgroup path escapes mount root")
		}
		return resolved, nil
	}
	return "", fmt.Errorf("unified cgroup v2 membership is missing")
}
