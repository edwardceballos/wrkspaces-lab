package schemapicker

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/edwardceballos/control/internal/config"
	"github.com/edwardceballos/control/internal/theme"
)

// ChosenMsg is emitted when the user selects a schema to load.
type ChosenMsg struct{ Schema config.SchemaFile }

// CancelMsg is emitted when the user leaves the schema picker without loading
// another schema.
type CancelMsg struct{}

type schemaItem struct{ schema config.SchemaFile }

func (i schemaItem) Title() string       { return i.schema.Name }
func (i schemaItem) Description() string { return i.schema.Description + " · " + i.schema.Path }
func (i schemaItem) FilterValue() string {
	return i.schema.Name + " " + i.schema.Description + " " + i.schema.Path
}

type Model struct {
	th    theme.Theme
	list  list.Model
	w, h  int
	box   lipgloss.Style
	title lipgloss.Style
	dim   lipgloss.Style
}

func New(th theme.Theme, schemas []config.SchemaFile) Model {
	items := make([]list.Item, 0, len(schemas))
	for _, schema := range schemas {
		items = append(items, schemaItem{schema: schema})
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
	l.SetFilteringEnabled(true)

	return Model{
		th:   th,
		list: l,
		box: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(th.Orange)).Padding(1, 2),
		title: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(th.Orange)),
		dim:   lipgloss.NewStyle().Foreground(lipgloss.Color(th.Gray)),
	}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.list.SetSize(m.innerSize())
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q", "esc":
			return m, func() tea.Msg { return CancelMsg{} }
		case "enter":
			if it, ok := m.list.SelectedItem().(schemaItem); ok {
				schema := it.schema
				return m, func() tea.Msg { return ChosenMsg{Schema: schema} }
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
	var b strings.Builder
	b.WriteString(m.title.Render("Seleccionar catálogo") + "\n")
	b.WriteString(m.dim.Render("Se detectó más de un schema*.yml") + "\n\n")
	b.WriteString(m.list.View() + "\n")
	b.WriteString(m.dim.Render("↑/↓ mover · / buscar · enter cargar · q salir"))
	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, m.box.Render(b.String()))
}

func (m Model) innerSize() (int, int) {
	w := m.w/2 - 6
	if w < 44 {
		w = 44
	}
	if w > 88 {
		w = 88
	}
	h := m.h - 10
	if h < 6 {
		h = 6
	}
	return w, h
}
