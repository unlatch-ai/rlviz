//go:build windows

package daemon

import (
	"os/exec"
	"syscall"
)

const createNewProcessGroup = 0x00000200

func configureDetachedProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNewProcessGroup,
		HideWindow:    true,
	}
}
