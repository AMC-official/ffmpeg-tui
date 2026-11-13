//go:build windows

package jobs

import (
	"errors"
	"os/exec"
	"syscall"
)

// pauseSupported is false on Windows: there is no SIGSTOP equivalent that a
// console child process honours reliably. The UI reports this honestly instead
// of pretending a job is paused.
const pauseSupported = false

var errUnsupported = errors.New("pausing is not supported on Windows")

func configureProcess(cmd *exec.Cmd) {
	// CREATE_NEW_PROCESS_GROUP lets a future implementation send CTRL_BREAK,
	// and keeps the child's console signals separate from ours.
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

func pauseProcess(cmd *exec.Cmd) error { return errUnsupported }

func resumeProcess(cmd *exec.Cmd) error { return errUnsupported }
