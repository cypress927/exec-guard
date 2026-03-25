package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// CommandExecutor 命令执行器
// 提供同步和异步两种执行模式，支持超时控制和输出截断
type CommandExecutor struct {
	// platformBuilder 平台命令构建器
	platformBuilder *PlatformCommandBuilder
	// processManager 后台进程管理器
	processManager *ProcessManager
	// streamReader 流读取器
	streamReader *StreamReader
}

// NewCommandExecutor 创建命令执行器实例
func NewCommandExecutor(processManager *ProcessManager) *CommandExecutor {
	return &CommandExecutor{
		platformBuilder: NewPlatformCommandBuilder(),
		processManager:  processManager,
		streamReader:    NewStreamReader(),
	}
}

// Execute 执行命令（主入口）
// 根据请求的 RunInBackground 字段决定同步或异步执行
func (e *CommandExecutor) Execute(req *ExecRequest) (*ExecResponse, error) {
	// 验证请求
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// 验证工作目录
	if req.WorkingDir != "" {
		if err := e.validateWorkingDir(req.WorkingDir); err != nil {
			return nil, err
		}
	}

	if req.RunInBackground {
		return e.executeBackground(req)
	}
	return e.executeSync(req)
}

// executeSync 同步执行命令
func (e *CommandExecutor) executeSync(req *ExecRequest) (*ExecResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(req.GetTimeout())*time.Second)
	defer cancel()

	cmd := e.platformBuilder.BuildCommand(req.Command, req.WorkingDir)

	// 设置环境变量（继承 + 覆盖）
	cmd.Env = buildEnv(req.Env)

	// 创建管道捕获输出
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// 启动进程
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start command: %w", err)
	}

	// 创建 RingBuffer 读取器
	stdoutReader := NewStreamReader()
	stderrReader := NewStreamReader()

	// 等待通道
	done := make(chan error, 1)
	var waitErr error

	// 启动协程读取输出（使用 RingBuffer）
	go func() {
		_, err := stdoutReader.ReadAll(stdoutPipe)
		if err != nil && err != io.EOF {
			// 记录错误但不中断
		}
	}()

	go func() {
		_, err := stderrReader.ReadAll(stderrPipe)
		if err != nil && err != io.EOF {
			// 记录错误但不中断
		}
	}()

	// 等待完成或超时
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case waitErr = <-done:
		// 正常完成
	case <-ctx.Done():
		// 超时，杀死进程
		if err := KillProcess(cmd); err != nil {
			return nil, fmt.Errorf("failed to kill process on timeout: %w", err)
		}
		// 等待读取协程完成
		<-done
		return &ExecResponse{
			Status:        StatusTimeout,
			ExitCode:      -1,
			Stdout:        stdoutReader.ringBuffer.String(),
			Stderr:        stderrReader.ringBuffer.String(),
			SystemMessage: ErrTimeoutExceeded.Error(),
		}, nil
	}

	// 构建响应
	response := &ExecResponse{
		Stdout: stdoutReader.ringBuffer.String(),
		Stderr: stderrReader.ringBuffer.String(),
	}

	// 处理错误
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			response.ExitCode = exitErr.ExitCode()
			response.Status = StatusFailed
			response.SystemMessage = fmt.Sprintf("command exited with code %d", exitErr.ExitCode())
		} else {
			response.ExitCode = -1
			response.Status = StatusFailed
			response.SystemMessage = fmt.Sprintf("execution error: %v", waitErr)
		}
	} else {
		response.ExitCode = 0
		response.Status = StatusSuccess
		response.SystemMessage = "command executed successfully"
	}

	return response, nil
}

// executeBackground 后台执行命令
// 支持监控窗口模式：如果设置了 watch_duration_seconds，等待 N 秒观察进程状态
func (e *CommandExecutor) executeBackground(req *ExecRequest) (*ExecResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(req.GetTimeout())*time.Second)

	cmd := e.platformBuilder.BuildCommand(req.Command, req.WorkingDir)

	// 设置环境变量（继承 + 覆盖）
	cmd.Env = buildEnv(req.Env)

	// 创建管道捕获输出
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// 启动进程
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to start command: %w", err)
	}

	// 创建 RingBuffer 读取器（双端缓冲）
	stdoutReader := NewStreamReader()
	stderrReader := NewStreamReader()

	// 创建后台进程对象
	proc := e.processManager.NewBackgroundProcess(cmd.Process.Pid, req.Command, cmd, cancel)
	proc.StdoutReader = stdoutReader
	proc.StderrReader = stderrReader

	// 注册进程
	if err := e.processManager.Register(cmd.Process.Pid, proc); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to register process: %w", err)
	}

	// 启动输出捕获协程（持续写入 RingBuffer）
	go func() {
		_, err := stdoutReader.ReadAll(stdoutPipe)
		if err != nil && err != io.EOF {
			proc.Error = fmt.Errorf("stdout read error: %w", err)
		}
	}()

	go func() {
		_, err := stderrReader.ReadAll(stderrPipe)
		if err != nil && err != io.EOF {
			proc.Error = fmt.Errorf("stderr read error: %w", err)
		}
	}()

	// 等待进程退出的通道
	processDone := make(chan error, 1)
	go func() {
		processDone <- cmd.Wait()
	}()

	// 检查是否启用监控窗口
	watchDuration := req.GetWatchDuration()
	if watchDuration > 0 {
		// 启动监控窗口等待逻辑
		return e.executeWithWatchWindow(proc, processDone, watchDuration)
	}

	// 无监控窗口：立即返回
	// 启动后台监控协程处理超时
	go func() {
		select {
		case err := <-processDone:
			// 进程正常完成
			if err != nil {
				proc.Error = err
				if exitErr, ok := err.(*exec.ExitError); ok {
					proc.ExitCode = exitErr.ExitCode()
					proc.Status = ProcessStatusFailed
				}
			} else {
				proc.ExitCode = 0
				proc.Status = ProcessStatusCompleted
			}
			proc.EndTime = time.Now()
		case <-ctx.Done():
			// 超时，杀死进程
			if err := KillProcess(cmd); err != nil {
				proc.Error = fmt.Errorf("timeout kill error: %w", err)
			}
			proc.Status = ProcessStatusFailed
			proc.EndTime = time.Now()
			proc.ExitCode = -1
		}
	}()

	// 立即返回响应
	return &ExecResponse{
		Status:        StatusRunning,
		ExitCode:      -1,
		Stdout:        "",
		Stderr:        "",
		SystemMessage: fmt.Sprintf("process started with PID %d", cmd.Process.Pid),
	}, nil
}

// executeWithWatchWindow 带监控窗口的后台执行
// 等待 watchDuration 秒观察进程状态
func (e *CommandExecutor) executeWithWatchWindow(proc *BackgroundProcess, processDone <-chan error, watchDuration int) (*ExecResponse, error) {
	watchTimer := time.NewTimer(time.Duration(watchDuration) * time.Second)
	defer watchTimer.Stop()

	select {
	case err := <-processDone:
		// 进程在监控窗口内退出
		proc.EndTime = time.Now()
		proc.WatchCompleted = true

		if err != nil {
			proc.Error = err
			if exitErr, ok := err.(*exec.ExitError); ok {
				proc.ExitCode = exitErr.ExitCode()
			} else {
				proc.ExitCode = -1
			}
			proc.Status = ProcessStatusFailed

			// 返回失败状态（视作同步执行失败）
			return &ExecResponse{
				Status:        StatusFailed,
				ExitCode:      proc.ExitCode,
				Stdout:        proc.StdoutReader.ringBuffer.String(),
				Stderr:        proc.StderrReader.ringBuffer.String(),
				SystemMessage: fmt.Sprintf("process exited during watch window with code %d", proc.ExitCode),
			}, nil
		} else {
			proc.ExitCode = 0
			proc.Status = ProcessStatusCompleted

			// 返回成功状态
			return &ExecResponse{
				Status:        StatusSuccess,
				ExitCode:      0,
				Stdout:        proc.StdoutReader.ringBuffer.String(),
				Stderr:        proc.StderrReader.ringBuffer.String(),
				SystemMessage: "process completed during watch window",
			}, nil
		}

	case <-watchTimer.C:
		// 监控窗口超时，进程仍在运行
		proc.WatchCompleted = true
		proc.WatchDuration = watchDuration

		// 获取监控窗口内收集的输出
		stdout := proc.StdoutReader.ringBuffer.String()
		stderr := proc.StderrReader.ringBuffer.String()

		// 启动后台监控协程继续追踪进程
		go func() {
			// 等待进程最终完成或主超时
			ctx := proc.CancelFunc // 获取取消上下文
			if ctx == nil {
				return
			}

			select {
			case err := <-processDone:
				if err != nil {
					proc.Error = err
					if exitErr, ok := err.(*exec.ExitError); ok {
						proc.ExitCode = exitErr.ExitCode()
						proc.Status = ProcessStatusFailed
					}
				} else {
					proc.ExitCode = 0
					proc.Status = ProcessStatusCompleted
				}
				proc.EndTime = time.Now()
			case <-time.After(time.Duration(proc.ExitCode) * time.Second):
				// 这里需要获取真正的超时上下文
			}
		}()

		// 返回 running 状态，附带初始化日志
		return &ExecResponse{
			Status:        StatusRunning,
			ExitCode:      -1,
			Stdout:        stdout,
			Stderr:        stderr,
			SystemMessage: fmt.Sprintf("process running with PID %d, watch window (%ds) elapsed", proc.PID, watchDuration),
		}, nil
	}
}

// validateWorkingDir 验证工作目录
func (e *CommandExecutor) validateWorkingDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", ErrInvalidWorkingDir, dir)
		}
		return fmt.Errorf("failed to stat working directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: %s is not a directory", ErrInvalidWorkingDir, dir)
	}
	return nil
}

// GetProcessStatus 获取后台进程状态
func (e *CommandExecutor) GetProcessStatus(pid int) (*ProcessStatus, error) {
	return e.processManager.GetStatus(pid)
}

// GetProcessInfo 获取后台进程完整信息
func (e *CommandExecutor) GetProcessInfo(pid int) (*ProcessInfo, error) {
	return e.processManager.GetProcessInfo(pid)
}

// TerminateProcess 终止后台进程
func (e *CommandExecutor) TerminateProcess(pid int) error {
	return e.processManager.Terminate(pid)
}

// ListProcesses 列出所有后台进程
func (e *CommandExecutor) ListProcesses() []*ProcessStatus {
	return e.processManager.List()
}

// CleanupCompletedProcesses 清理已完成的进程记录
func (e *CommandExecutor) CleanupCompletedProcesses() int {
	return e.processManager.CleanupCompleted()
}

// buildEnv 构建环境变量列表
// 1. 默认继承宿主机所有环境变量 (os.Environ())
// 2. 传入的自定义环境变量会覆盖宿主机同名变量
func buildEnv(customEnv map[string]string) []string {
	// 1. 继承系统环境变量到 map 中（便于去重和覆盖）
	envMap := make(map[string]string)
	for _, e := range os.Environ() {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	// 2. 合并/覆盖 AI 传入的自定义环境变量
	if customEnv != nil {
		for k, v := range customEnv {
			envMap[k] = v
		}
	}

	// 3. 转换回 KEY=VALUE 切片
	finalEnv := make([]string, 0, len(envMap))
	for k, v := range envMap {
		finalEnv = append(finalEnv, fmt.Sprintf("%s=%s", k, v))
	}

	return finalEnv
}
