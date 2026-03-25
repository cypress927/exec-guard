package main

import (
	"os"
	"os/exec"
	"runtime"
)

// PlatformCommandBuilder 跨平台命令构建器
// 根据不同操作系统包装命令，确保跨平台兼容性
type PlatformCommandBuilder struct{}

// NewPlatformCommandBuilder 创建命令构建器实例
func NewPlatformCommandBuilder() *PlatformCommandBuilder {
	return &PlatformCommandBuilder{}
}

// BuildCommand 根据当前操作系统构建命令
// Windows: 使用 cmd.exe /c <command>
// Linux/macOS: 使用 bash -c "<command>"
func (b *PlatformCommandBuilder) BuildCommand(command string, workingDir string) *exec.Cmd {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		// Windows 平台使用 cmd.exe
		cmd = exec.Command("cmd.exe", "/c", command)
	default:
		// Linux/macOS/Unix 使用 bash
		cmd = exec.Command("bash", "-c", command)
	}

	// 设置工作目录
	if workingDir != "" {
		cmd.Dir = workingDir
	} else {
		// 使用当前工作目录
		if wd, err := os.Getwd(); err == nil {
			cmd.Dir = wd
		}
	}

	// 设置进程组，便于后续统一杀死进程
	b.setupProcessGroup(cmd)

	return cmd
}

// setupProcessGroup 设置进程组（跨平台）
func (b *PlatformCommandBuilder) setupProcessGroup(cmd *exec.Cmd) {
	switch runtime.GOOS {
	case "windows":
		b.setupProcessGroupWindows(cmd)
	default:
		b.setupProcessGroupUnix(cmd)
	}
}

// KillProcess 彻底杀死进程及其所有子进程
// Linux/Unix: 杀死整个进程组
// Windows: 使用 CREATE_NEW_PROCESS_GROUP 标志
func KillProcess(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	switch runtime.GOOS {
	case "windows":
		return killWindowsProcess(cmd)
	default:
		return killUnixProcess(cmd)
	}
}

// GetShellCommand 获取当前平台的 shell 命令格式
func GetShellCommand() string {
	switch runtime.GOOS {
	case "windows":
		return "cmd.exe /c"
	default:
		return "bash -c"
	}
}
