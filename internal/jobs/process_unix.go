//go:build !windows

package jobs

import (
	"os/exec"
	"syscall"
)

// pauseSupported is true where POSIX job control signals are available.
const pauseSupported = true

func configureProcess(cmd *exec.Cmd) {
	// Own process group so signals reach ffmpeg's children too (e.g. segment
	// muxers). Never combined with a shell.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func pauseProcess(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return exec.ErrNotFound
	}
	return syscall.Kill(cmd.Process.Pid, syscall.SIGSTOP)
}

func resumeProcess(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return exec.ErrNotFound
	}
	return syscall.Kill(cmd.Process.Pid, syscall.SIGCONT)
}
