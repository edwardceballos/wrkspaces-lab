// Package picker is a small modal screen to choose an editor for a component.
// It lists only editors detected on PATH (editor.Available). It emits ChosenMsg
// on selection or CancelMsg on esc; the root performs the actual launch + pop.
package picker

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/edwardceballos/control/internal/config"
	"github.com/edwardceballos/control/internal/editor"
	"github.com/edwardceballos/control/internal/theme"
)

// ChosenMsg is emitted when the user selects an editor.
type ChosenMsg struct {
	Editor editor.Editor
	Path   string
}

// CancelMsg is emitted when the user dismisses the picker.
type CancelMsg struct{}

type edItem struct{ ed editor.Editor }

func (i edItem) Title() string       { return i.ed.Label }
func (i edItem) Description() string { return i.ed.ID + " · " + i.ed.Kind.String() }
func (i edItem) FilterValue() string { return i.ed.Label + " " + i.ed.ID }

type Model struct {
	th    theme.Theme
	comp  config.Component
	path  string
	list  list.Model
	avail []editor.Editor
	w, h  int

	box   lipgloss.Style
	title lipgloss.Style
	dim   lipgloss.Style
	warn  lipgloss.Style
}

// New builds the picker for a component. The path to open is folder, falling
// back to srcfolder (IDEs open project roots).
func New(th theme.Theme, comp config.Component) Model {
	path := comp.Folder
	if path == "" {
		path = comp.SrcFolder
	}

	avail := editor.Available()
	items := make([]list.Item, len(avail))
	for i, e := range avail {
		items[i] = edItem{ed: e}
	}

	d := list.NewDefaultDelegate()
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.
		Foreground(lipgloss.Color(th.Orange)).BorderForeground(lipgloss.Color(th.Orange))
	d.Styles.SelectedDesc = d.Styles.SelectedDesc.
		Foreground(lipgloss.Color(th.Yellow)).BorderForeground(lipgloss.Color(th.Orange))
	d.Styles.NormalTitle = d.Styles.NormalTitle.Foreground(lipgloss.Color(th.Fg))
	d.Styles.NormalDesc = d.Styles.NormalDesc.Foreground(lipgloss.Color(th.FgDim))

	l := list.New(items, d, 0, 0)
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)

	return Model{
		th:    th,
		comp:  comp,
		path:  path,
		list:  l,
		avail: avail,
		box: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(th.Orange)).
			Padding(1, 2),
		title: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(th.Orange)),
		dim:   lipgloss.NewStyle().Foreground(lipgloss.Color(th.Gray)),
		warn:  lipgloss.NewStyle().Foreground(lipgloss.Color(th.Red)),
	}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		bw, bh := m.boxInner()
		m.list.SetSize(bw, bh)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			return m, func() tea.Msg { return CancelMsg{} }
		case "ctrl+c":
			return m, tea.Quit
		case "enter":
			if len(m.avail) == 0 {
				return m, func() tea.Msg { return CancelMsg{} }
			}
			if it, ok := m.list.SelectedItem().(edItem); ok {
				chosen := it.ed
				path := m.path
				return m, func() tea.Msg { return ChosenMsg{Editor: chosen, Path: path} }
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) boxInner() (w, h int) {
	w = m.w/2 - 6
	if w < 30 {
		w = 30
	}
	if w > 60 {
		w = 60
	}
	h = len(m.avail) * 2 // default delegate uses 2 lines per item
	if h < 4 {
		h = 4
	}
	if h > m.h-8 {
		h = m.h - 8
	}
	return w, h
}

func (m Model) View() string {
	if m.w == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(m.title.Render("Abrir ◆ "+m.comp.Name+" en…") + "\n")
	b.WriteString(m.dim.Render(m.path) + "\n\n")

	if len(m.avail) == 0 {
		b.WriteString(m.warn.Render("No se detectaron editores en PATH.") + "\n")
		b.WriteString(m.dim.Render("Instala los CLI launchers (JetBrains Toolbox → Shell scripts,\n'code' de VS Code, o nvim) y reintenta.") + "\n\n")
		b.WriteString(m.dim.Render("esc cerrar"))
	} else {
		b.WriteString(m.list.View() + "\n")
		b.WriteString(m.dim.Render("↑/↓ mover · enter abrir · esc cancelar"))
	}

	box := m.box.Render(b.String())
	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, box)
}
