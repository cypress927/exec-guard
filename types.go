package main

// ExecRequest 命令执行请求结构
type ExecRequest struct {
	// Command 要执行的系统命令（必填）
	Command string `json:"command"`
	// WorkingDir 工作目录，默认为当前目录（可选）
	WorkingDir string `json:"working_dir,omitempty"`
	// TimeoutSeconds 超时时间（秒），默认 30 秒（可选）
	TimeoutSeconds int `json:"timeout_seconds,omitempty"`
	// RunInBackground 是否后台运行，默认 false（可选）
	RunInBackground bool `json:"run_in_background,omitempty"`
	// WatchDurationSeconds 监控窗口时长（秒），仅后台模式有效
	// 在 fork 子进程后等待 N 秒观察进程状态
	// - N 秒内退出：返回 failed + exit_code
	// - N 秒后仍在运行：返回 running + 初始化日志，进程继续后台运行
	WatchDurationSeconds int `json:"watch_duration_seconds,omitempty"`
	// Env 自定义环境变量（可选）
	// - 默认继承宿主机所有环境变量
	// - 传入的变量会覆盖宿主机同名变量（如 PATH、NODE_ENV 等）
	Env map[string]string `json:"env,omitempty"`
}

// ExecResponse 命令执行响应结构
type ExecResponse struct {
	// Status 执行状态：success/failed/timeout/killed/running
	Status string `json:"status"`
	// ExitCode 进程退出码，-1 表示异常或运行中
	ExitCode int `json:"exit_code"`
	// Stdout 标准输出（已截断处理，使用 Head-Tail 双端缓冲）
	Stdout string `json:"stdout"`
	// Stderr 标准错误（已截断处理，使用 Head-Tail 双端缓冲）
	Stderr string `json:"stderr"`
	// SystemMessage 系统消息/错误详情
	SystemMessage string `json:"system_message"`
}

// ProcessStatus 后台进程状态
type ProcessStatus struct {
	// PID 进程 ID
	PID int `json:"pid"`
	// Status 进程状态：running/completed/failed
	Status string `json:"status"`
	// ExitCode 退出码
	ExitCode int `json:"exit_code"`
	// Command 执行的命令
	Command string `json:"command"`
	// StartTime 启动时间
	StartTime string `json:"start_time"`
	// EndTime 结束时间（完成后填充）
	EndTime string `json:"end_time,omitempty"`
	// WatchDurationSeconds 监控窗口时长（如果设置了）
	WatchDurationSeconds int `json:"watch_duration_seconds,omitempty"`
	// WatchCompleted 监控窗口是否已完成
	WatchCompleted bool `json:"watch_completed,omitempty"`
}

// ProcessInfo 后台进程完整信息（含输出）
type ProcessInfo struct {
	ProcessStatus
	// Stdout 标准输出（Head-Tail 双端缓冲拼接结果）
	Stdout string `json:"stdout"`
	// Stderr 标准错误（Head-Tail 双端缓冲拼接结果）
	Stderr string `json:"stderr"`
}

// Validate 验证请求的合法性
func (r *ExecRequest) Validate() error {
	if r.Command == "" {
		return ErrEmptyCommand
	}
	return nil
}

// GetTimeout 获取超时时间（带默认值）
func (r *ExecRequest) GetTimeout() int {
	if r.TimeoutSeconds <= 0 {
		return DefaultTimeoutSeconds
	}
	return r.TimeoutSeconds
}

// GetWatchDuration 获取监控窗口时长（0 表示不启用监控窗口）
func (r *ExecRequest) GetWatchDuration() int {
	if r.WatchDurationSeconds < 0 {
		return 0
	}
	return r.WatchDurationSeconds
}

// HasWatchWindow 是否启用了监控窗口
func (r *ExecRequest) HasWatchWindow() bool {
	return r.RunInBackground && r.WatchDurationSeconds > 0
}
