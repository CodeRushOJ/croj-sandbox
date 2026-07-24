// internal/sandbox/executor.go
package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/CodeRushOJ/croj-sandbox/internal/security"
	"github.com/CodeRushOJ/croj-sandbox/internal/util"
)

// Executor handles executing commands with appropriate resource limits.
type Executor struct {
	cfg Config
}

// NewExecutor creates a new executor instance.
func NewExecutor(cfg Config) *Executor {
	return &Executor{cfg: cfg}
}

// Execute runs the provided command with resource constraints.
// runCmd: Command and arguments to execute (already processed for placeholders)
// env: Optional environment variables
// stdinData: Optional standard input data
func (e *Executor) Execute(ctx context.Context, runCmd []string, env map[string]string, stdinData *string) (result Result) {
	if len(runCmd) == 0 {
		return NewResult(StatusSandboxError, fmt.Errorf("empty command provided to executor"))
	}
	if e.cfg.WorkingDir == "" {
		return NewResult(StatusSandboxError, fmt.Errorf("request working directory is required"))
	}
	launcherPath := e.cfg.SandboxExecPath
	if launcherPath == "" {
		launcherPath = DefaultSandboxExecPath
	}
	resolvedLauncher, err := exec.LookPath(launcherPath)
	if err != nil {
		return NewResult(StatusSandboxError, fmt.Errorf("security launcher is unavailable: %w", err))
	}

	util.DebugLog("sandbox event=command_prepared argv_count=%d", len(runCmd))
	launcherArgs := []string{
		"--language", e.cfg.Language,
		"--work-dir", e.cfg.WorkingDir,
	}
	environmentNames := make([]string, 0, len(env))
	for name := range env {
		environmentNames = append(environmentNames, name)
	}
	sort.Strings(environmentNames)
	for _, name := range environmentNames {
		launcherArgs = append(launcherArgs, "--target-env", name+"="+env[name])
	}
	launcherArgs = append(launcherArgs, "--")
	launcherArgs = append(launcherArgs, runCmd...)

	gateReader, gateWriter, err := os.Pipe()
	if err != nil {
		return NewResult(StatusSandboxError, fmt.Errorf("create isolation gate: %w", err))
	}
	defer gateReader.Close()
	defer gateWriter.Close()
	statusReader, statusWriter, err := os.Pipe()
	if err != nil {
		return NewResult(StatusSandboxError, fmt.Errorf("create isolation status pipe: %w", err))
	}
	defer statusReader.Close()
	defer statusWriter.Close()

	execCmd := exec.CommandContext(ctx, resolvedLauncher, launcherArgs...)
	execCmd.Env = []string{
		"PATH=/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
	}
	execCmd.ExtraFiles = []*os.File{gateReader, statusWriter}
	execCmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGKILL,
	}

	// Setup stdin if provided
	var stdinPipe io.WriteCloser
	if stdinData != nil {
		var err error
		stdinPipe, err = execCmd.StdinPipe()
		if err != nil {
			return NewResult(StatusSandboxError, fmt.Errorf("failed to get stdin pipe: %w", err))
		}
	}

	// Setup stdout/stderr with limits
	var stdoutBuf, stderrBuf bytes.Buffer
	stdoutWriter := NewLimitedWriter(&stdoutBuf, e.cfg.MaxStdoutSize)
	stderrWriter := NewLimitedWriter(&stderrBuf, e.cfg.MaxStderrSize)
	execCmd.Stdout = stdoutWriter
	execCmd.Stderr = stderrWriter

	// 直接使用配置中的超时设置，不再处理
	execTimeout := e.cfg.DefaultExecuteTimeLimit

	// 确保有合理的默认值
	if execTimeout <= 0 {
		execTimeout = 3 * time.Second
		util.WarnLog("超时设置为0或负值，使用默认值 %.2f秒", execTimeout.Seconds())
	}

	// 仅在调试模式下打印超时设置
	util.DebugLog("Executor: 使用超时设置: %.2f秒", execTimeout.Seconds())

	// Execute the command
	startTime := time.Now()

	// 启动命令但不等待它完成
	if err := execCmd.Start(); err != nil {
		return NewResult(StatusSandboxError, fmt.Errorf("failed to start command: %w", err))
	}
	_ = gateReader.Close()
	_ = statusWriter.Close()

	// 获取进程ID并开始监控资源使用
	pid := execCmd.Process.Pid
	memLimitKB := e.cfg.DefaultExecuteMemoryLimit / 1024 // 从bytes转换为KB

	// 内存和时间限制信息只需简要展示
	util.InfoLog("监控进程 %d: 内存限制 %.2f MB, 时间限制 %.2f 秒",
		pid, float64(memLimitKB)/1024, execTimeout.Seconds())

	// 创建安全配置文件
	secProfile := security.ProfileForLanguage(e.cfg.Language)

	// 设置内存限制
	secProfile.MemoryLimitBytes = e.cfg.DefaultExecuteMemoryLimit

	if err := prepareRequestWorkingDirectory(e.cfg.WorkingDir, pid); err != nil {
		terminateUnreadyProcess(execCmd)
		return NewResult(StatusSandboxError, fmt.Errorf("prepare isolated working directory: %w", err))
	}

	var cgroupManager *security.CgroupManager
	if secProfile.EnableCgroups {
		cgroupManager, err = security.SetupCgroups(
			security.CgroupIDForProcess("execute", pid),
			pid,
			secProfile,
		)
		if err != nil {
			terminateUnreadyProcess(execCmd)
			return NewResult(StatusSandboxError, fmt.Errorf("configure request cgroup: %w", err))
		}
		defer func() {
			cleanupErr := security.CleanupCgroups(cgroupManager)
			if cleanupErr != nil {
				util.WarnLog("sandbox event=cgroup_cleanup_failed category=resource_isolation")
			}
			result = resultAfterCgroupCleanup(result, cleanupErr)
		}()
	} else {
		terminateUnreadyProcess(execCmd)
		return NewResult(StatusSandboxError, fmt.Errorf("request cgroup isolation is required"))
	}

	if _, err := gateWriter.Write([]byte{1}); err != nil {
		terminateUnreadyProcess(execCmd)
		return NewResult(StatusSandboxError, fmt.Errorf("release security launcher: %w", err))
	}
	_ = gateWriter.Close()

	if err := statusReader.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		terminateUnreadyProcess(execCmd)
		return NewResult(StatusSandboxError, fmt.Errorf("bound launcher handshake: %w", err))
	}
	var ready [1]byte
	if count, err := statusReader.Read(ready[:]); err != nil || count != 1 || ready[0] != 1 {
		terminateUnreadyProcess(execCmd)
		return NewResult(StatusSandboxError, fmt.Errorf("security launcher failed before contestant exec"))
	}
	_ = statusReader.Close()
	util.DebugLog("sandbox event=isolation_ready pid=%d", pid)

	if stdinPipe != nil {
		go func() {
			defer stdinPipe.Close()
			if _, err := io.WriteString(stdinPipe, *stdinData); err != nil {
				log.Printf("sandbox event=stdin_write_failed category=pipe_write")
			}
		}()
	}

	// 创建监控通道
	monitorDone := make(chan struct{})
	defer close(monitorDone)

	// 创建结果通道，用于从监控goroutine接收资源使用情况
	resultChan := make(chan *util.ProcessStats, 1)

	// 启动监控goroutine
	go func() {
		// 每10ms检查一次资源使用，提高精度
		procStats := util.MonitorProcess(pid, memLimitKB, execTimeout, 10*time.Millisecond, monitorDone)
		resultChan <- procStats
	}()

	// 等待命令完成或超时
	runErr := execCmd.Wait()
	duration := time.Since(startTime)

	// 收集监控结果
	var procStats *util.ProcessStats
	select {
	case stats := <-resultChan:
		procStats = stats
	case <-time.After(100 * time.Millisecond):
		// 如果监控没有及时返回结果，创建默认结果
		procStats = &util.ProcessStats{
			PID:       pid,
			MemoryKB:  -1,
			Duration:  duration,
			IsTimeout: false,
		}
	}

	// 构建结果
	result = Result{
		ExitCode:       0, // Will be set below if available
		TimeUsedMillis: duration.Milliseconds(),
		MemoryUsedKB:   procStats.MemoryKB,
		Stdout:         stdoutBuf.String(),
		Stderr:         stderrBuf.String(),
	}

	// 确定状态
	// 1. 首先检查是否超时
	if procStats.IsTimeout {
		result.Status = StatusTimeLimitExceeded
		result.Error = fmt.Sprintf("时间超限: %.2f秒 (限制: %.2f秒)",
			procStats.Duration.Seconds(), execTimeout.Seconds())
		result.ExitCode = -1
		return result
	}

	// 2. 再检查内存限制
	if procStats.IsExceeded {
		result.Status = StatusMemoryLimitExceeded
		result.Error = fmt.Sprintf("Memory limit exceeded: %d KB (limit: %d KB)", procStats.MemoryKB, memLimitKB)
		result.ExitCode = -1
		return result
	}

	// 3. 检查Context是否超时(备用检测)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		result.Status = StatusTimeLimitExceeded
		result.Error = fmt.Sprintf("Context deadline exceeded: %v", ErrExecuteTimeout)
		result.ExitCode = -1
		return result
	}

	// 4. Check for output limit exceeded
	var outputLimitErr error
	if stdoutWriter.(*LimitedWriter).Exceeded {
		outputLimitErr = fmt.Errorf("%w (stdout, limit: %d bytes)", ErrOutputLimitExceeded, e.cfg.MaxStdoutSize)
	}
	if stderrWriter.(*LimitedWriter).Exceeded {
		errAppend := fmt.Errorf("%w (stderr, limit: %d bytes)", ErrOutputLimitExceeded, e.cfg.MaxStderrSize)
		if outputLimitErr != nil {
			outputLimitErr = fmt.Errorf("%v; %v", outputLimitErr, errAppend)
		} else {
			outputLimitErr = errAppend
		}
	}

	if outputLimitErr != nil {
		result.Status = StatusOutputLimitExceeded
		result.Error = outputLimitErr.Error()
	}

	// 5. Check run errors and exit code
	if execCmd.ProcessState != nil {
		result.ExitCode = execCmd.ProcessState.ExitCode()
	}

	if runErr != nil && result.Status == "" {
		result.Status = StatusRuntimeError
		result.Error = fmt.Sprintf("Runtime error: %v (exit code: %d)", runErr, result.ExitCode)
	}

	// 6. Set as Accepted if no other status was determined
	if result.Status == "" {
		if result.ExitCode == 0 {
			result.Status = StatusAccepted
		} else {
			result.Status = StatusRuntimeError
			result.Error = fmt.Sprintf("Process exited with code %d", result.ExitCode)
		}
	}

	return result
}

func resultAfterCgroupCleanup(result Result, cleanupErr error) Result {
	if cleanupErr == nil {
		return result
	}
	return NewResult(
		StatusSandboxError,
		fmt.Errorf("cleanup request cgroup process tree: %w", cleanupErr),
	)
}

func prepareRequestWorkingDirectory(path string, pid int) error {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(absolutePath)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("working path is not a directory")
	}
	uid := security.IsolatedUIDForProcess(pid)
	if err := filepath.WalkDir(absolutePath, func(entryPath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		return os.Lchown(entryPath, uid, uid)
	}); err != nil {
		return err
	}
	return os.Chmod(absolutePath, 0o700)
}

func terminateUnreadyProcess(command *exec.Cmd) {
	if command == nil || command.Process == nil {
		return
	}
	_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	_ = command.Process.Kill()
	_ = command.Wait()
}

// --- LimitedWriter ---

// LimitedWriter wraps an io.Writer but stops writing after a certain limit.
type LimitedWriter struct {
	w        io.Writer
	limit    int64
	written  int64
	mu       sync.Mutex
	Exceeded bool // Flag to indicate if the limit was reached
}

// NewLimitedWriter creates a new LimitedWriter.
func NewLimitedWriter(w io.Writer, limit int64) io.Writer {
	return &LimitedWriter{w: w, limit: limit}
}

func (lw *LimitedWriter) Write(p []byte) (n int, err error) {
	lw.mu.Lock()
	defer lw.mu.Unlock()

	remaining := lw.limit - lw.written
	if remaining <= 0 {
		if !lw.Exceeded {
			lw.Exceeded = true
		}
		return len(p), nil // Pretend we wrote everything
	}

	writeLen := int64(len(p))
	if writeLen > remaining {
		writeLen = remaining
		lw.Exceeded = true
	}

	n, err = lw.w.Write(p[:writeLen])
	lw.written += int64(n)

	if err == nil && lw.Exceeded {
		return len(p), nil
	}

	return n, err
}
