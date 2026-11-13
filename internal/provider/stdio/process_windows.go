package stdio

import (
	"context"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func command(ctx context.Context, executable string, args []string) *exec.Cmd {
	extension := strings.ToLower(filepath.Ext(executable))
	if extension == ".cmd" || extension == ".bat" {
		// cmd.exe needs a surrounding quote pair in addition to path/argument quotes.
		// These shims receive only Loom's fixed version/app-server arguments.
		parts := []string{executable}
		parts = append(parts, args...)
		for i, part := range parts {
			parts[i] = `"` + strings.ReplaceAll(part, `"`, `""`) + `"`
		}
		cmd := exec.CommandContext(ctx, "cmd.exe")
		cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd.exe /d /v:off /s /c "` + strings.Join(parts, " ") + `"`}
		return cmd
	}
	return exec.CommandContext(ctx, executable, args...)
}

func configure(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		return exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
	}
}
