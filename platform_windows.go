//go:build windows
// +build windows

package main

import (
	"os/exec"
	"syscall"
)

// setupProcessGroupUnix 在 Windows 上是空操作（为了编译通过）
func (b *PlatformCommandBuilder) setupProcessGroupUnix(cmd *exec.Cmd) {
	// Windows 不使用此方法
}

// killUnixProcess 在 Windows 上是空操作（为了编译通过）
func killUnixProcess(cmd *exec.Cmd) error {
	// Windows 不使用此方法
	return nil
}

// setupProcessGroupWindows 设置 Windows 进程组
func (b *PlatformCommandBuilder) setupProcessGroupWindows(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

// killWindowsProcess 杀死 Windows 进程
func killWindowsProcess(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
