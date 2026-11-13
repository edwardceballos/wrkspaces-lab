// Package jobs lists all jobs from the manager so the user can re-attach to a
// backgrounded process or kill it. This is what makes "leave it running and go
// back to the catalog" usable.
package jobs

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/edwardceballos/control/internal/runner"
	"github.com/edwardceballos/control/internal/theme"
)

// AttachMsg asks the root to open the run view for a job.
type AttachMsg struct{ JobID int }

// KillMsg asks the root to kill a job.
type KillMsg struct{ JobID int }

// CloseMsg pops the jobs screen.
type CloseMsg struct{}

type jobItem struct {
	id     int
	title  string
	status runner.Status
}

func (i jobItem) Title() string { return i.title }
func (i jobItem) Description() string {
	switch i.status {
	case runner.Running:
		return "● running"
	case runner.Exited:
		return "✓ exited"
	case runner.Failed:
		return "✗ failed"
	case runner.Killed:
		return "■ killed"
	}
	return ""
}
func (i jobItem) FilterValue() string { return i.title }

type Model struct {
	th   theme.Theme
	mgr  *runner.Manager
	list list.Model
	w, h int

	title  lipgloss.Style
	footer lipgloss.Style
	dim    lipgloss.Style
}

func New(th theme.Theme, mgr *runner.Manager) Model {
	d := list.NewDefaultDelegate()
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.
		Foreground(lipgloss.Color(th.Orange)).BorderForeground(lipgloss.Color(th.Orange))
	d.Styles.SelectedDesc = d.Styles.SelectedDesc.
		Foreground(lipgloss.Color(th.Yellow)).BorderForeground(lipgloss.Color(th.Orange))
	d.Styles.NormalTitle = d.Styles.NormalTitle.Foreground(lipgloss.Color(th.Fg))
	d.Styles.NormalDesc = d.Styles.NormalDesc.Foreground(lipgloss.Color(th.FgDim))

	l := list.New(nil, d, 0, 0)
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)

	m := Model{
		th:     th,
		mgr:    mgr,
		list:   l,
		title:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(th.Fg)),
		footer: lipgloss.NewStyle().Foreground(lipgloss.Color(th.Gray)),
		dim:    lipgloss.NewStyle().Foreground(lipgloss.Color(th.Gray)),
	}
	m.rebuild()
	return m
}

func (m *Model) rebuild() {
	jobs := m.mgr.Jobs()
	items := make([]list.Item, 0, len(jobs))
	// newest first
	for i := len(jobs) - 1; i >= 0; i-- {
		j := jobs[i]
		items = append(items, jobItem{id: j.ID, title: fmt.Sprintf("[%d] %s", j.ID, j.Label), status: j.Status})
	}
	idx := m.list.Index()
	m.list.SetItems(items)
	if idx < len(items) {
		m.list.Select(idx)
	}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.list.SetSize(m.w-2, m.h-2)
		return m, nil

	case runner.OutputMsg, runner.DoneMsg:
		m.rebuild() // statuses may have changed
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			return m, func() tea.Msg { return CloseMsg{} }
		case "ctrl+c":
			return m, tea.Quit
		case "enter":
			if it, ok := m.list.SelectedItem().(jobItem); ok {
				id := it.id
				return m, func() tea.Msg { return AttachMsg{JobID: id} }
			}
		case "k":
			if it, ok := m.list.SelectedItem().(jobItem); ok && it.status == runner.Running {
				id := it.id
				return m, func() tea.Msg { return KillMsg{JobID: id} }
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	if m.w == 0 {
		return ""
	}
	if len(m.mgr.Jobs()) == 0 {
		empty := m.dim.Render("No hay trabajos todavía.\nEjecuta un comando con 'r' sobre un componente.")
		return lipgloss.JoinVertical(lipgloss.Left,
			m.title.Render(" Trabajos"), "\n"+empty,
			m.footer.Render(" esc volver"))
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		m.title.Render(" Trabajos"),
		m.list.View(),
		m.footer.Render(" ↑/↓ mover · enter ver salida · k detener · esc volver · q salir"))
}
