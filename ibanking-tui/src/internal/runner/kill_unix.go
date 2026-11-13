//go:build unix

package runner

import (
	"os"
	"os/exec"
	"syscall"
)

// setpgid puts the child in its own process group so we can signal the whole
// tree (sh -c → npm → node, etc.) instead of just the shell.
func setpgid(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killGroup sends SIGTERM to the negative pid (the process group).
func killGroup(p *os.Process) {
	_ = syscall.Kill(-p.Pid, syscall.SIGTERM)
}
