//go:build !unix

package agent

import "os/exec"

func isolate(cmd *exec.Cmd) {}
