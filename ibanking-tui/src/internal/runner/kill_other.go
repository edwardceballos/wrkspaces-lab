//go:build !unix

package runner

import (
	"os"
	"os/exec"
)

func setpgid(c *exec.Cmd) {} // no-op off Unix

func killGroup(p *os.Process) { _ = p.Kill() }
