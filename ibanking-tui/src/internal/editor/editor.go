// Package editor knows how to detect installed editors and launch them.
// GUI editors (JetBrains suite, VS Code) are started fire-and-forget so they
// don't block or steal the TTY. Terminal editors (nvim) use tea.ExecProcess,
// which hands over the terminal and restores it on exit (ADR-0004).
package editor

import (
	"os/exec"
	"runtime"

	tea "github.com/charmbracelet/bubbletea"
)

type Kind int

const (
	GUI Kind = iota
	Terminal
)

func (k Kind) String() string {
	if k == Terminal {
		return "terminal"
	}
	return "gui"
}

type Editor struct {
	ID    string // command name on PATH (also matches schema env.ide values)
	Label string
	Kind  Kind
}

// registry lists known editors. Detection filters this to what's on PATH, so
// listing extras here is harmless. JetBrains CLI launchers come from Toolbox
// "shell scripts" (rider, idea, goland, …).
var registry = []Editor{
	{"rider", "Rider", GUI},
	{"idea", "IntelliJ IDEA", GUI},
	{"pycharm", "PyCharm", GUI},
	{"webstorm", "WebStorm", GUI},
	{"goland", "GoLand", GUI},
	{"rubymine", "RubyMine", GUI},
	{"phpstorm", "PhpStorm", GUI},
	{"clion", "CLion", GUI},
	{"datagrip", "DataGrip", GUI},
	{"rustrover", "RustRover", GUI},
	{"dataspell", "DataSpell", GUI},
	{"aqua", "Aqua", GUI},
	{"fleet", "Fleet", GUI},
	{"code", "VS Code", GUI},
	{"code-insiders", "VS Code Insiders", GUI},
	{"nvim", "Neovim", Terminal},
}

// LaunchedMsg reports the result of a launch (or URL open).
type LaunchedMsg struct {
	Editor string
	Err    error
}

// Available returns the registry entries whose command resolves on PATH.
func Available() []Editor {
	var out []Editor
	for _, e := range registry {
		if _, err := exec.LookPath(e.ID); err == nil {
			out = append(out, e)
		}
	}
	return out
}

// Launch opens path in this editor as a tea.Cmd.
func (e Editor) Launch(path string) tea.Cmd {
	if e.Kind == Terminal {
		// Hands over the TTY; control resumes after the editor exits.
		return tea.ExecProcess(exec.Command(e.ID, path), func(err error) tea.Msg {
			return LaunchedMsg{Editor: e.ID, Err: err}
		})
	}
	// GUI: Start() (NOT Run()) — fire-and-forget, returns immediately.
	return func() tea.Msg {
		c := exec.Command(e.ID, path)
		err := c.Start()
		return LaunchedMsg{Editor: e.ID, Err: err}
	}
}

// OpenURL opens a URL in the default browser, cross-platform, no extra deps.
func OpenURL(url string) tea.Cmd {
	return func() tea.Msg {
		var c *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			c = exec.Command("open", url)
		case "windows":
			c = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
		default: // linux, *bsd
			c = exec.Command("xdg-open", url)
		}
		return LaunchedMsg{Editor: "browser", Err: c.Start()}
	}
}
