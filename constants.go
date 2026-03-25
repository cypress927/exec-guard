package main

import "errors"

// 默认配置常量
const (
	// DefaultTimeoutSeconds 默认超时时间（秒）
	DefaultTimeoutSeconds = 30
	// MaxOutputBytes 最大输出字节数（8KB）
	MaxOutputBytes = 8192
	// TruncateHeadBytes 截断时保留的头部字节数
	TruncateHeadBytes = 4096
	// TruncateTailBytes 截断时保留的尾部字节数
	TruncateTailBytes = 4096
	// DefaultHTTPPort 默认 HTTP 服务端口
	DefaultHTTPPort = 8080
)

// 截断提示模板
const TruncatePlaceholder = "\n... [TRUNCATED: %d bytes omitted] ...\n"

// 执行状态常量
const (
	StatusSuccess = "success"
	StatusFailed  = "failed"
	StatusTimeout = "timeout"
	StatusKilled  = "killed"
	StatusRunning = "running"
)

// 后台进程状态常量
const (
	ProcessStatusRunning   = "running"
	ProcessStatusCompleted = "completed"
	ProcessStatusFailed    = "failed"
)

// 预定义错误
var (
	// ErrEmptyCommand 空命令错误
	ErrEmptyCommand = errors.New("command cannot be empty")
	// ErrProcessNotFound 进程未找到错误
	ErrProcessNotFound = errors.New("process not found")
	// ErrTimeoutExceeded 超时错误
	ErrTimeoutExceeded = errors.New("execution timeout exceeded")
	// ErrProcessAlreadyExists 进程已存在错误
	ErrProcessAlreadyExists = errors.New("process already exists")
	// ErrInvalidWorkingDir 无效工作目录错误
	ErrInvalidWorkingDir = errors.New("invalid working directory")
)
