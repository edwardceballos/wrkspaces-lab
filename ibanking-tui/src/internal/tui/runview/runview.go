// Package runview shows a single job's live output (ADR-0004). It reads
// job.Output by pointer (the root appends to it), so leaving and re-attaching
// shows full history. esc pops the screen WITHOUT killing the job.
package runview

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/edwardceballos/control/internal/runner"
	"github.com/edwardceballos/control/internal/theme"
)

// PopMsg pops this screen (job keeps running).
type PopMsg struct{}

// KillMsg asks the root to kill the job.
type KillMsg struct{ ID int }

type Model struct {
	th    theme.Theme
	job   *runner.Job
	vp    viewport.Model
	w, h  int
	ready bool

	header lipgloss.Style
	dim    lipgloss.Style
	footer lipgloss.Style
	ok     lipgloss.Style
	bad    lipgloss.Style
	run    lipgloss.Style
}

func New(th theme.Theme, job *runner.Job) Model {
	return Model{
		th:     th,
		job:    job,
		header: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(th.Orange)),
		dim:    lipgloss.NewStyle().Foreground(lipgloss.Color(th.Gray)),
		footer: lipgloss.NewStyle().Foreground(lipgloss.Color(th.Gray)),
		ok:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(th.Green)),
		bad:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(th.Red)),
		run:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(th.Yellow)),
	}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		if !m.ready {
			m.vp = viewport.New(m.w, m.contentHeight())
			m.ready = true
		} else {
			m.vp.Width = m.w
			m.vp.Height = m.contentHeight()
		}
		m.refresh()
		return m, nil

	case runner.OutputMsg:
		if msg.ID == m.job.ID {
			m.refresh() // root already appended the line to job.Output
		}
		return m, nil

	case runner.DoneMsg:
		if msg.ID == m.job.ID {
			m.refresh()
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q", "backspace":
			return m, func() tea.Msg { return PopMsg{} }
		case "k":
			if m.job.Status == runner.Running {
				id := m.job.ID
				return m, func() tea.Msg { return KillMsg{ID: id} }
			}
		case "ctrl+c":
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

// refresh re-renders the viewport, keeping tail-follow when already at bottom.
func (m *Model) refresh() {
	if !m.ready {
		return
	}
	wasBottom := m.vp.AtBottom()
	m.vp.SetContent(strings.Join(m.job.Output, "\n"))
	if wasBottom {
		m.vp.GotoBottom()
	}
}

func (m Model) contentHeight() int {
	h := m.h - 2 // header + footer
	if h < 3 {
		h = 3
	}
	return h
}

func (m Model) statusBadge() string {
	switch m.job.Status {
	case runner.Running:
		return m.run.Render("● running")
	case runner.Exited:
		return m.ok.Render("✓ exited")
	case runner.Failed:
		return m.bad.Render("✗ failed")
	case runner.Killed:
		return m.bad.Render("■ killed")
	}
	return ""
}

func (m Model) View() string {
	if m.w == 0 {
		return ""
	}
	title := m.header.Render("▶ "+m.job.Label) + "  " + m.statusBadge()
	lines := []string{
		m.dim.Render("$ " + m.job.Line),
		m.dim.Render("wd: " + m.job.Dir),
	}
	for _, detail := range m.job.Details {
		if detail.Label == "" || detail.Cmd == "" {
			continue
		}
		lines = append(lines, m.dim.Render(detail.Label+": "+detail.Cmd))
	}
	sub := strings.Join(lines, "\n")

	body := ""
	if m.ready {
		body = m.vp.View()
	}

	keys := " ↑/↓ scroll · esc volver (sigue corriendo)"
	if m.job.Status == runner.Running {
		keys += " · k detener"
	}
	keys += " · q salir"
	footer := m.footer.Render(keys)

	return lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.JoinVertical(lipgloss.Left, title, sub), body, footer)
}
