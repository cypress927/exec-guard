package main

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

// BackgroundProcess 后台进程信息
type BackgroundProcess struct {
	// PID 进程 ID
	PID int
	// Command 执行的命令
	Command string
	// Cmd 底层命令对象
	Cmd *exec.Cmd
	// CancelFunc 取消函数（用于超时控制）
	CancelFunc context.CancelFunc
	// StartTime 启动时间
	StartTime time.Time
	// EndTime 结束时间
	EndTime time.Time
	// Status 进程状态
	Status string
	// ExitCode 退出码
	ExitCode int
	// StdoutReader 标准输出读取器（RingBuffer）
	StdoutReader *StreamReader
	// StderrReader 标准错误读取器（RingBuffer）
	StderrReader *StreamReader
	// WatchDuration 监控窗口时长（秒）
	WatchDuration int
	// WatchCompleted 监控窗口是否已完成
	WatchCompleted bool
	// Error 执行错误
	Error error
}

// ProcessManager 后台进程管理器
// 提供线程安全的进程注册、查询、终止功能
type ProcessManager struct {
	// mu 互斥锁保护进程映射
	mu sync.RWMutex
	// processes 进程映射表 PID -> BackgroundProcess
	processes map[int]*BackgroundProcess
	// maxProcesses 最大进程数限制
	maxProcesses int
}

// NewProcessManager 创建进程管理器实例
func NewProcessManager(maxProcesses int) *ProcessManager {
	return &ProcessManager{
		processes:    make(map[int]*BackgroundProcess),
		maxProcesses: maxProcesses,
	}
}

// Register 注册新的后台进程
func (pm *ProcessManager) Register(pid int, proc *BackgroundProcess) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if len(pm.processes) >= pm.maxProcesses {
		return fmt.Errorf("max process limit reached: %d", pm.maxProcesses)
	}

	if _, exists := pm.processes[pid]; exists {
		return ErrProcessAlreadyExists
	}

	pm.processes[pid] = proc
	return nil
}

// Get 获取进程信息
func (pm *ProcessManager) Get(pid int) (*BackgroundProcess, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	proc, exists := pm.processes[pid]
	return proc, exists
}

// GetStatus 获取进程状态
func (pm *ProcessManager) GetStatus(pid int) (*ProcessStatus, error) {
	proc, exists := pm.Get(pid)
	if !exists {
		return nil, ErrProcessNotFound
	}

	return &ProcessStatus{
		PID:                  proc.PID,
		Status:               proc.Status,
		ExitCode:             proc.ExitCode,
		Command:              proc.Command,
		StartTime:            proc.StartTime.Format(time.RFC3339),
		EndTime:              proc.EndTime.Format(time.RFC3339),
		WatchDurationSeconds: proc.WatchDuration,
		WatchCompleted:       proc.WatchCompleted,
	}, nil
}

// GetProcessInfo 获取进程完整信息（含输出）
func (pm *ProcessManager) GetProcessInfo(pid int) (*ProcessInfo, error) {
	proc, exists := pm.Get(pid)
	if !exists {
		return nil, ErrProcessNotFound
	}

	stdout := ""
	stderr := ""
	if proc.StdoutReader != nil {
		stdout = proc.StdoutReader.ringBuffer.String()
	}
	if proc.StderrReader != nil {
		stderr = proc.StderrReader.ringBuffer.String()
	}

	return &ProcessInfo{
		ProcessStatus: ProcessStatus{
			PID:                  proc.PID,
			Status:               proc.Status,
			ExitCode:             proc.ExitCode,
			Command:              proc.Command,
			StartTime:            proc.StartTime.Format(time.RFC3339),
			EndTime:              proc.EndTime.Format(time.RFC3339),
			WatchDurationSeconds: proc.WatchDuration,
			WatchCompleted:       proc.WatchCompleted,
		},
		Stdout: stdout,
		Stderr: stderr,
	}, nil
}

// Remove 移除进程记录
func (pm *ProcessManager) Remove(pid int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	delete(pm.processes, pid)
}

// Terminate 终止进程
func (pm *ProcessManager) Terminate(pid int) error {
	proc, exists := pm.Get(pid)
	if !exists {
		return ErrProcessNotFound
	}

	// 调用取消函数（如果设置了超时）
	if proc.CancelFunc != nil {
		proc.CancelFunc()
	}

	// 杀死进程
	err := KillProcess(proc.Cmd)
	if err != nil {
		return fmt.Errorf("failed to kill process %d: %w", pid, err)
	}

	// 更新状态
	proc.Status = ProcessStatusFailed // 被终止的进程标记为 failed
	proc.EndTime = time.Now()
	proc.ExitCode = -1

	return nil
}

// CleanupCompleted 清理已完成的进程记录
func (pm *ProcessManager) CleanupCompleted() int {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	count := 0
	for pid, proc := range pm.processes {
		if proc.Status == ProcessStatusCompleted || proc.Status == ProcessStatusFailed {
			delete(pm.processes, pid)
			count++
		}
	}
	return count
}

// List 列出所有进程
func (pm *ProcessManager) List() []*ProcessStatus {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	statuses := make([]*ProcessStatus, 0, len(pm.processes))
	for _, proc := range pm.processes {
		statuses = append(statuses, &ProcessStatus{
			PID:                  proc.PID,
			Status:               proc.Status,
			ExitCode:             proc.ExitCode,
			Command:              proc.Command,
			StartTime:            proc.StartTime.Format(time.RFC3339),
			EndTime:              proc.EndTime.Format(time.RFC3339),
			WatchDurationSeconds: proc.WatchDuration,
			WatchCompleted:       proc.WatchCompleted,
		})
	}
	return statuses
}

// NewBackgroundProcess 创建后台进程对象
func (pm *ProcessManager) NewBackgroundProcess(pid int, command string, cmd *exec.Cmd, cancelFunc context.CancelFunc) *BackgroundProcess {
	return &BackgroundProcess{
		PID:            pid,
		Command:        command,
		Cmd:            cmd,
		CancelFunc:     cancelFunc,
		StartTime:      time.Now(),
		Status:         ProcessStatusRunning,
		ExitCode:       -1,
		StdoutReader:   NewStreamReader(),
		StderrReader:   NewStreamReader(),
		WatchDuration:  0,
		WatchCompleted: false,
	}
}

// GetStdout 获取标准输出（从 RingBuffer）
func (proc *BackgroundProcess) GetStdout() string {
	if proc.StdoutReader == nil {
		return ""
	}
	return proc.StdoutReader.ringBuffer.String()
}

// GetStderr 获取标准错误（从 RingBuffer）
func (proc *BackgroundProcess) GetStderr() string {
	if proc.StderrReader == nil {
		return ""
	}
	return proc.StderrReader.ringBuffer.String()
}

// StreamOutput 后台进程输出流结构
type StreamOutput struct {
	Stdout string
	Stderr string
	Error  error
}

// StartStreamCapture 开始捕获进程输出流（后台协程，持续写入 RingBuffer）
// 返回一个通道用于接收完成通知
func (pm *ProcessManager) StartStreamCapture(proc *BackgroundProcess, stdoutPipe, stderrPipe io.ReadCloser) <-chan *StreamOutput {
	resultChan := make(chan *StreamOutput, 1)

	go func() {
		var wg sync.WaitGroup
		var stdoutErr, stderrErr error

		wg.Add(2)

		// 捕获 stdout（直接写入 RingBuffer）
		go func() {
			defer wg.Done()
			if proc.StdoutReader != nil {
				_, err := proc.StdoutReader.ReadAll(stdoutPipe)
				if err != nil && err != io.EOF {
					stdoutErr = err
				}
			}
		}()

		// 捕获 stderr（直接写入 RingBuffer）
		go func() {
			defer wg.Done()
			if proc.StderrReader != nil {
				_, err := proc.StderrReader.ReadAll(stderrPipe)
				if err != nil && err != io.EOF {
					stderrErr = err
				}
			}
		}()

		wg.Wait()

		// 等待进程结束
		err := proc.Cmd.Wait()
		if err != nil {
			proc.Error = err
			if exitErr, ok := err.(*exec.ExitError); ok {
				proc.ExitCode = exitErr.ExitCode()
			} else {
				proc.ExitCode = -1
			}
		} else {
			proc.ExitCode = 0
		}

		// 确定最终状态
		if proc.Status == ProcessStatusRunning {
			if proc.Error != nil {
				proc.Status = ProcessStatusFailed
			} else {
				proc.Status = ProcessStatusCompleted
			}
		}
		proc.EndTime = time.Now()

		// 发送结果
		result := &StreamOutput{
			Stdout: proc.GetStdout(),
			Stderr: proc.GetStderr(),
		}

		if stdoutErr != nil {
			result.Error = fmt.Errorf("stdout error: %w", stdoutErr)
		} else if stderrErr != nil {
			result.Error = fmt.Errorf("stderr error: %w", stderrErr)
		} else {
			result.Error = proc.Error
		}

		resultChan <- result
	}()

	return resultChan
}
