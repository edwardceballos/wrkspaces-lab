// Package cmdpicker is a modal to choose which command to run for a component.
// Commands come from config.RunnableCommands (run/test/build + extras), so new
// commands added in schema.yml appear here automatically. enter runs streamed;
// f runs full-TTY (the root translates the choice into the actual launch).
package cmdpicker

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/edwardceballos/control/internal/config"
	"github.com/edwardceballos/control/internal/theme"
)

// ChosenMsg is emitted on selection. TTY=true means run cediendo la terminal.
type ChosenMsg struct {
	Component config.Component
	Cmd       config.RunCmd
	TTY       bool
}

// CancelMsg dismisses the picker.
type CancelMsg struct{}

type cmdItem struct{ rc config.RunCmd }

func (i cmdItem) Title() string { return i.rc.Name }
func (i cmdItem) Description() string {
	var parts []string
	if i.rc.Description != "" {
		parts = append(parts, i.rc.Description)
	}
	if len(i.rc.Tags) > 0 {
		parts = append(parts, "["+strings.Join(i.rc.Tags, ", ")+"]")
	}
	if len(i.rc.Hints) > 0 {
		parts = append(parts, formatHints(i.rc.Hints))
	} else {
		parts = append(parts, i.rc.Cmd)
	}
	return strings.Join(parts, " · ")
}

func (i cmdItem) FilterValue() string {
	return i.rc.Name + " " + i.rc.Description + " " + i.rc.Cmd + " " + formatHints(i.rc.Hints) + " " + strings.Join(i.rc.Tags, " ")
}

type Model struct {
	th    theme.Theme
	comp  config.Component
	cmds  []config.RunCmd
	tags  []string
	tag   string
	list  list.Model
	w, h  int
	box   lipgloss.Style
	title lipgloss.Style
	dim   lipgloss.Style
	warn  lipgloss.Style
}

func New(th theme.Theme, comp config.Component) Model {
	cmds := comp.RunnableCommands()
	items := commandItems(cmds, "")

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
	l.SetFilteringEnabled(true)

	return Model{
		th:   th,
		comp: comp,
		cmds: cmds,
		tags: commandTags(cmds),
		list: l,
		box: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(th.Orange)).Padding(1, 2),
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
			return m, cancel()
		case "ctrl+c":
			return m, tea.Quit
		case "enter":
			return m.choose(false)
		case "f":
			return m.choose(true)
		case "tab":
			return m.nextTag(1), nil
		case "shift+tab":
			return m.nextTag(-1), nil
		case "a":
			if m.list.FilterState() == list.Unfiltered {
				m.tag = ""
				m.applyTag()
				return m, nil
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) choose(tty bool) (Model, tea.Cmd) {
	if len(m.list.Items()) == 0 {
		return m, cancel()
	}
	it, ok := m.list.SelectedItem().(cmdItem)
	if !ok {
		return m, nil
	}
	comp, rc := m.comp, it.rc
	return m, func() tea.Msg { return ChosenMsg{Component: comp, Cmd: rc, TTY: tty} }
}

func cancel() tea.Cmd { return func() tea.Msg { return CancelMsg{} } }

func (m Model) nextTag(delta int) Model {
	if len(m.tags) == 0 {
		return m
	}
	idx := -1
	for i, t := range m.tags {
		if t == m.tag {
			idx = i
			break
		}
	}
	if delta > 0 {
		idx++
		if idx >= len(m.tags) {
			idx = -1
		}
	} else {
		idx--
		if idx < -1 {
			idx = len(m.tags) - 1
		}
	}
	if idx == -1 {
		m.tag = ""
	} else {
		m.tag = m.tags[idx]
	}
	m.applyTag()
	return m
}

func (m *Model) applyTag() {
	m.list.SetItems(commandItems(m.cmds, m.tag))
	m.list.Select(0)
}

func commandItems(cmds []config.RunCmd, tag string) []list.Item {
	items := make([]list.Item, 0, len(cmds))
	for _, rc := range cmds {
		if tag != "" && !hasTag(rc, tag) {
			continue
		}
		items = append(items, cmdItem{rc: rc})
	}
	return items
}

func commandTags(cmds []config.RunCmd) []string {
	seen := map[string]bool{}
	for _, rc := range cmds {
		for _, tag := range rc.Tags {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				seen[tag] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for tag := range seen {
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}

func hasTag(rc config.RunCmd, tag string) bool {
	for _, t := range rc.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

func formatHints(hints []config.CmdHint) string {
	if len(hints) == 0 {
		return ""
	}
	parts := make([]string, 0, len(hints))
	for _, hint := range hints {
		if hint.Label == "" || hint.Cmd == "" {
			continue
		}
		parts = append(parts, hint.Label+": "+hint.Cmd)
	}
	return strings.Join(parts, " · ")
}

func (m Model) boxInner() (w, h int) {
	w = m.w/2 - 6
	if w < 34 {
		w = 34
	}
	if w > 70 {
		w = 70
	}
	h = len(m.list.Items()) * 2
	if h < 4 {
		h = 4
	}
	if max := m.h - 8; h > max {
		h = max
	}
	return w, h
}

func (m Model) View() string {
	if m.w == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(m.title.Render("Ejecutar en ◆ "+m.comp.Name) + "\n")
	b.WriteString(m.dim.Render("wd: "+m.comp.WorkingDir(config.RunCmd{})) + "\n\n")
	if len(m.tags) > 0 {
		active := "all"
		if m.tag != "" {
			active = m.tag
		}
		b.WriteString(m.dim.Render(fmt.Sprintf("categoria: %s  (%d/%d comandos)", active, len(m.list.Items()), len(m.cmds))) + "\n\n")
	}

	if len(m.list.Items()) == 0 {
		b.WriteString(m.warn.Render("Este componente no tiene comandos.") + "\n\n")
		b.WriteString(m.dim.Render("esc cerrar"))
	} else {
		b.WriteString(m.list.View() + "\n")
		b.WriteString(m.dim.Render("↑/↓ mover · / buscar · tab categoria · a todos · enter streaming · f terminal · esc cancelar"))
	}
	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, m.box.Render(b.String()))
}
