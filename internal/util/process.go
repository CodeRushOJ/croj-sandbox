package util

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ProcessStats 存储进程的资源使用情况
type ProcessStats struct {
	PID        int   // 进程ID
	MemoryKB   int64 // 内存使用（KB）
	CPUTimeMS  int64 // CPU使用时间（毫秒）
	IsExceeded bool  // 是否超过内存限制
	IsTimeout  bool  // 是否超时
	Duration   time.Duration // 执行时长
}

// MonitorProcess 监控指定进程的资源使用情况（内存和时间）
func MonitorProcess(pid int, memoryLimitKB int64, timeoutDuration time.Duration, interval time.Duration, done <-chan struct{}) *ProcessStats {
	stats := &ProcessStats{
		PID:      pid,
		MemoryKB: -1,
	}

	// 确保进程ID有效
	if pid <= 0 {
		ErrorLog("无效的进程ID: %d", pid)
		return stats
	}

	// 确保超时值有效
	if timeoutDuration <= 0 {
		WarnLog("无效的超时设置: %v, 使用默认值", timeoutDuration)
		timeoutDuration = 10 * time.Second // 使用安全的默认值
	}

	DebugLog("开始监控进程 %d，超时设置: %.2f秒", pid, timeoutDuration.Seconds())
	
	// 创建同步组，确保资源监控正确完成
	var wg sync.WaitGroup
	var mutex sync.Mutex
	startTime := time.Now()
	
	// 创建独立的计时器用于精确超时控制
	wg.Add(1)
	go func() {
		defer wg.Done()
		
		 // 记录实际启动时间
		timer := time.NewTimer(timeoutDuration)
		defer timer.Stop()
		
		select {
		case <-timer.C:
			mutex.Lock()
			elapsed := time.Since(startTime)
			InfoLog("进程 %d 超时: %.2f秒 (限制: %.2f秒)", 
				pid, elapsed.Seconds(), timeoutDuration.Seconds())
			stats.IsTimeout = true
			stats.Duration = elapsed
			
			// 强制终止进程树
			killErr := terminateProcessTree(pid)
			if killErr != nil {
				ErrorLog("终止进程 %d 时发生错误: %v", pid, killErr)
			} else {
				DebugLog("成功终止进程 %d 和其子进程", pid)
			}
			mutex.Unlock()
			
		case <-done:
			// 如果通道关闭，退出监控
			return
		}
	}()
	
	// 使用独立的goroutine监控资源使用
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				mutex.Lock()
				
				// 检查是否已超时
				if stats.IsTimeout {
					mutex.Unlock()
					return
				}
				
				// 检查进程是否仍在运行
				if !isProcessRunning(pid) {
					stats.Duration = time.Since(startTime)
					mutex.Unlock()
					return
				}
				
				// 更新运行时间
				elapsed := time.Since(startTime)
				stats.Duration = elapsed
				
				 // 定期记录进程状态
				if DebugMode && int(elapsed.Seconds()) > 0 && 
				   int(elapsed.Seconds()) % 1 == 0 && 
				   int(elapsed.Seconds()) != int((elapsed - interval).Seconds()) {
					DebugLog("进程 %d 已运行: %.1f秒 (限制: %.1f秒)", 
						pid, elapsed.Seconds(), timeoutDuration.Seconds())
				}
				
				 // 监控内存使用
				memKB, err := getProcessAndChildrenMemoryKB(pid)
				if err == nil && memKB > stats.MemoryKB {
					stats.MemoryKB = memKB
					
					// 仅在调试模式下记录内存使用情况
					if DebugMode && int(elapsed.Seconds()) > 0 && int(elapsed.Seconds()) % 2 == 0 &&
					   int(elapsed.Seconds()) != int((elapsed - interval).Seconds()) {
						DebugLog("进程 %d 内存使用: %d KB (%.2f MB)", 
							pid, memKB, float64(memKB)/1024)
					}
				}
				
				// 检查内存限制
				if memoryLimitKB > 0 && stats.MemoryKB > memoryLimitKB {
					InfoLog("进程 %d 内存超限: %d KB > %d KB", 
						pid, stats.MemoryKB, memoryLimitKB)
					stats.IsExceeded = true
					_ = terminateProcessTree(pid)
					mutex.Unlock()
					return
				}
				
				mutex.Unlock()
				
			case <-done:
				mutex.Lock()
				stats.Duration = time.Since(startTime)
				mutex.Unlock()
				return
			}
		}
	}()
	
	// 等待所有监控goroutine完成
	go func() {
		wg.Wait()
		DebugLog("进程 %d 的监控任务已完成", pid)
	}()

	return stats
}

// isProcessRunning 检查进程是否在运行
func isProcessRunning(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	
	// 尝试发送空信号检查进程是否存在
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

// terminateProcessTree 终止进程及其子进程，改进版本
func terminateProcessTree(pid int) error {
	// 首先尝试获取所有子进程
	children, err := getChildProcesses(pid)
	if err == nil && len(children) > 0 {
		log.Printf("找到 %d 个子进程需要终止", len(children))
		
		// 终止所有子进程
		for _, childPid := range children {
			proc, err := os.FindProcess(childPid)
			if err == nil {
				log.Printf("终止子进程: %d", childPid)
				if err := proc.Kill(); err != nil {
					log.Printf("无法终止子进程 %d: %v", childPid, err)
				}
				
				// 在Unix系统上使用SIGKILL确保终止
				if runtime.GOOS != "windows" {
					_ = proc.Signal(syscall.SIGKILL)
				}
			}
		}
	}
	
	// 最后终止主进程
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("找不到进程 %d: %w", pid, err)
	}
	
	log.Printf("终止主进程: %d", pid)
	if err := proc.Kill(); err != nil {
		return fmt.Errorf("终止进程 %d 失败: %w", pid, err)
	}
	
	// 在Unix系统上使用SIGKILL确保终止
	if runtime.GOOS != "windows" {
		_ = proc.Signal(syscall.SIGKILL)
	}
	
	return nil
}

// MonitorMemory 监控指定进程的内存使用(为兼容性保留)
func MonitorMemory(pid int, memoryLimitKB int64, interval time.Duration, done <-chan struct{}) *ProcessStats {
	return MonitorProcess(pid, memoryLimitKB, 0, interval, done)
}

// getProcessMemoryKB 获取指定进程的内存使用量（KB）
func getProcessMemoryKB(pid int) (int64, error) {
	switch runtime.GOOS {
	case "linux":
		return getLinuxProcessMemoryKB(pid)
	case "darwin":
		return getDarwinProcessMemoryKB(pid)
	case "windows":
		return getWindowsProcessMemoryKB(pid)
	default:
		return -1, fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

// getLinuxProcessMemoryKB 获取Linux上进程的内存使用量（KB）
func getLinuxProcessMemoryKB(pid int) (int64, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return -1, err
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				memKB, err := strconv.ParseInt(fields[1], 10, 64)
				if err != nil {
					return -1, err
				}
				return memKB, nil
			}
		}
	}

	return -1, fmt.Errorf("无法获取进程内存使用")
}

// getDarwinProcessMemoryKB 获取macOS上进程的内存使用量（KB）
func getDarwinProcessMemoryKB(pid int) (int64, error) {
	// 在macOS上使用ps命令获取内存使用
	// ps -o rss= -p <pid>
	cmd := execCommand("ps", "-o", "rss=", "-p", strconv.Itoa(pid))
	output, err := cmd.Output()
	if err != nil {
		return -1, err
	}

	// 解析输出（KB）
	memStr := strings.TrimSpace(string(output))
	memKB, err := strconv.ParseInt(memStr, 10, 64)
	if err != nil {
		return -1, err
	}

	return memKB, nil
}

// getWindowsProcessMemoryKB 获取Windows上进程的内存使用量（KB）
func getWindowsProcessMemoryKB(pid int) (int64, error) {
	// 在Windows上使用wmic命令获取内存使用
	// wmic process where ProcessId=<pid> get WorkingSetSize
	cmd := execCommand("wmic", "process", "where", fmt.Sprintf("ProcessId=%d", pid), "get", "WorkingSetSize")
	output, err := cmd.Output()
	if err != nil {
		return -1, err
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return -1, fmt.Errorf("无法解析wmic输出")
	}

	memStr := strings.TrimSpace(lines[1])
	memBytes, err := strconv.ParseInt(memStr, 10, 64)
	if err != nil {
		return -1, err
	}

	// 转换为KB
	return memBytes / 1024, nil
}

// getProcessAndChildrenMemoryKB 获取进程及其所有子进程的总内存使用量
func getProcessAndChildrenMemoryKB(pid int) (int64, error) {
	// 先获取进程自身的内存使用
	memKB, err := getProcessMemoryKB(pid)
	if err != nil {
		return -1, err
	}
	
	// 如果是Java或Python等解释型语言，尝试查找子进程
	children, err := getChildProcesses(pid)
	if err == nil && len(children) > 0 {
		for _, childPid := range children {
			childMem, err := getProcessMemoryKB(childPid)
			if err == nil && childMem > 0 {
				memKB += childMem // 累加子进程内存
				DebugLog("子进程 %d 内存使用: %d KB", childPid, childMem)
			}
		}
	}
	
	return memKB, nil
}

// getChildProcesses 获取指定进程的所有子进程ID
func getChildProcesses(pid int) ([]int, error) {
	var children []int
	
	switch runtime.GOOS {
	case "linux":
		// 在Linux上使用/proc文件系统
		files, err := os.ReadDir(fmt.Sprintf("/proc"))
		if err != nil {
			return nil, err
		}
		
		for _, file := range files {
			// 如果是数字，可能是进程ID目录
			if file.IsDir() {
				childPid, err := strconv.Atoi(file.Name())
				if err == nil {
					// 读取/proc/{pid}/stat文件，查看父进程ID
					statFile := fmt.Sprintf("/proc/%d/stat", childPid)
					data, err := os.ReadFile(statFile)
					if err == nil {
						fields := strings.Fields(string(data))
						if len(fields) >= 4 {
							ppid, err := strconv.Atoi(fields[3])
							if err == nil && ppid == pid {
								children = append(children, childPid)
							}
						}
					}
				}
			}
		}
		
	case "darwin", "windows":
		// 在macOS和Windows上使用ps命令
		var cmd *exec.Cmd
		if runtime.GOOS == "darwin" {
			cmd = exec.Command("pgrep", "-P", strconv.Itoa(pid))
		} else {
			cmd = exec.Command("wmic", "process", "where", fmt.Sprintf("ParentProcessId=%d", pid), "get", "ProcessId")
		}
		
		output, err := cmd.Output()
		if err != nil {
			return nil, err
		}
		
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && line != "ProcessId" { // 过滤Windows输出的标题行
				childPid, err := strconv.Atoi(line)
				if err == nil {
					children = append(children, childPid)
				}
			}
		}
	}
	
	return children, nil
}

// 为了便于测试，将exec.Command包装起来
var execCommand = exec.Command
