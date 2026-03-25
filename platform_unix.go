//go:build !windows
// +build !windows

package main

import (
	"os/exec"
	"syscall"
)

// setupProcessGroupUnix 设置 Unix 进程组
func (b *PlatformCommandBuilder) setupProcessGroupUnix(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

// killUnixProcess 杀死 Unix 进程组
func killUnixProcess(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}

	// 使用负数 PID 杀死整个进程组
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		// 如果获取进程组失败，尝试直接杀死进程
		return cmd.Process.Kill()
	}

	// 杀死整个进程组（负数表示进程组）
	err = syscall.Kill(-pgid, syscall.SIGKILL)
	if err != nil {
		// 回退到直接杀死进程
		return cmd.Process.Kill()
	}

	return nil
}
