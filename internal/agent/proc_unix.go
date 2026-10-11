//go:build unix

package agent

import (
	"os/exec"
	"syscall"
)

// note: opencode menjalankan proses anak (shell, test, dev server); saat timeout seluruh grup dibunuh biar nggak ada yang yatim
func isolate(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
