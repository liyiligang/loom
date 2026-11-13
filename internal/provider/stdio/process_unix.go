//go:build !windows

package stdio

import (
	"context"
	"os/exec"
	"syscall"
)

func command(ctx context.Context, executable string, args []string) *exec.Cmd {
	return exec.CommandContext(ctx, executable, args...)
}

func configure(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}
