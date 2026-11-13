// Package catalog is the main navigation screen: a master-detail browser over
// the loaded Config. Customer -> Project -> {Components, Environments}, with a
// detail pane that reacts to the highlighted item (ADR-0002).
package catalog

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/edwardceballos/control/internal/config"
	"github.com/edwardceballos/control/internal/runner"
	"github.com/edwardceballos/control/internal/theme"
)

const (
	maxURLsShown   = 8
	maxStacksShown = 12
)

type level int

const (
	levelCustomers level = iota
	levelProjects
	levelProject
)

type itemKind int

const (
	kindCustomer itemKind = iota
	kindProject
	kindComponent
	kindEnvironment
	kindCICD
	kindLayerHeader
)

// catItem implements list.Item and carries a pointer to the underlying data so
// the detail pane can render it without re-looking-up indices.
type catItem struct {
	kind        itemKind
	title       string
	desc        string
	customer    *config.Customer
	project     *config.Project
	component   *config.Component
	environment *config.Environment
}

func (i catItem) Title() string       { return i.title }
func (i catItem) Description() string { return i.desc }
func (i catItem) FilterValue() string { return i.title }

type Model struct {
	cfg *config.Config
	th  theme.Theme
	mgr *runner.Manager

	list   list.Model
	w, h   int
	level  level
	status string

	curCustomer *config.Customer
	curProject  *config.Project

	bread  lipgloss.Style
	sep    lipgloss.Style
	footer lipgloss.Style
	hint   lipgloss.Style
	detail lipgloss.Style
	dTitle lipgloss.Style
	dLabel lipgloss.Style
	dValue lipgloss.Style
	dTag   lipgloss.Style
}

func New(th theme.Theme, cfg *config.Config, mgr *runner.Manager) Model {
	d := list.NewDefaultDelegate()
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.
		Foreground(lipgloss.Color(th.Orange)).BorderForeground(lipgloss.Color(th.Orange))
	d.Styles.SelectedDesc = d.Styles.SelectedDesc.
		Foreground(lipgloss.Color(th.Yellow)).BorderForeground(lipgloss.Color(th.Orange))
	d.Styles.NormalTitle = d.Styles.NormalTitle.Foreground(lipgloss.Color(th.Fg))
	d.Styles.NormalDesc = d.Styles.NormalDesc.Foreground(lipgloss.Color(th.FgDim))

	l := list.New(customerItems(cfg), d, 0, 0)
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	l.SetStatusBarItemName("item", "items")
	l.Styles.StatusBar = l.Styles.StatusBar.Foreground(lipgloss.Color(th.Gray))

	return Model{
		cfg:    cfg,
		th:     th,
		mgr:    mgr,
		list:   l,
		level:  levelCustomers,
		bread:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(th.Fg)),
		sep:    lipgloss.NewStyle().Foreground(lipgloss.Color(th.Gray)),
		footer: lipgloss.NewStyle().Foreground(lipgloss.Color(th.Gray)),
		hint:   lipgloss.NewStyle().Foreground(lipgloss.Color(th.Yellow)),
		detail: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(lipgloss.Color(th.Gray)).
			Padding(0, 2),
		dTitle: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(th.Orange)),
		dLabel: lipgloss.NewStyle().Foreground(lipgloss.Color(th.FgDim)),
		dValue: lipgloss.NewStyle().Foreground(lipgloss.Color(th.Fg)),
		dTag:   lipgloss.NewStyle().Foreground(lipgloss.Color(th.Aqua)),
	}
}

func (m Model) Init() tea.Cmd { return nil }

// actionable reports whether an item supports run/editor/repo actions.
func actionable(k itemKind) bool { return k == kindComponent || k == kindCICD || k == kindEnvironment }

// --- item builders ----------------------------------------------------------

func customerItems(cfg *config.Config) []list.Item {
	out := make([]list.Item, 0, len(cfg.Customers))
	for i := range cfg.Customers {
		cu := &cfg.Customers[i]
		out = append(out, catItem{
			kind:     kindCustomer,
			title:    cu.Name,
			desc:     strings.TrimSpace(fmt.Sprintf("%s · %d proyectos", cu.Website, len(cu.Projects))),
			customer: cu,
		})
	}
	return out
}

func projectItems(cu *config.Customer) []list.Item {
	out := make([]list.Item, 0, len(cu.Projects))
	for i := range cu.Projects {
		p := &cu.Projects[i]
		out = append(out, catItem{
			kind:    kindProject,
			title:   p.Name,
			desc:    fmt.Sprintf("%d componentes · %d entornos", len(p.Components), len(p.Environments)),
			project: p,
		})
	}
	return out
}

func projectChildItems(p *config.Project) []list.Item {
	out := make([]list.Item, 0, len(p.Components)+len(p.Environments)+1)

	if p.CICD != nil {
		comp := p.CICD.Component(p.Name)
		config.EnrichStackAdminHints(&comp, p.Environments)
		desc := fmt.Sprintf("%d pipelines", len(p.CICD.Pipelines))
		if p.CICD.Actions != nil {
			desc += " · actions: " + p.CICD.Actions.Repo
		}
		out = append(out, catItem{
			kind:      kindCICD,
			title:     "⚙ ci/cd",
			desc:      desc,
			component: &comp, // synthetic component (ADR-0008)
		})
	}

	layerOrder := []string{"", "frontend", "backend", "infra"}
	grouped := make(map[string][]*config.Component, len(layerOrder))
	for i := range p.Components {
		c := &p.Components[i]
		grouped[c.Layer] = append(grouped[c.Layer], c)
	}
	for _, layer := range layerOrder {
		comps := grouped[layer]
		if len(comps) == 0 {
			continue
		}
		if layer != "" {
			out = append(out, catItem{kind: kindLayerHeader, title: "── " + layer + " ──", desc: " "})
		}
		for _, c := range comps {
			desc := c.Type
			if len(c.Tags) > 0 {
				desc += " · " + strings.Join(c.Tags, ", ")
			}
			out = append(out, catItem{
				kind:      kindComponent,
				title:     "◆ " + c.Name,
				desc:      desc,
				component: c,
			})
		}
	}
	for i := range p.Environments {
		e := &p.Environments[i]
		var extra []string
		if e.IDE != "" {
			extra = append(extra, "ide: "+e.IDE)
		}
		if len(e.URLs) > 0 {
			extra = append(extra, fmt.Sprintf("%d urls", len(e.URLs)))
		}
		if e.Compose != nil {
			extra = append(extra, fmt.Sprintf("%d stacks", len(e.Compose.Stacks)))
		}
		out = append(out, catItem{
			kind:        kindEnvironment,
			title:       "▣ " + e.Name,
			desc:        strings.Join(extra, " · "),
			environment: e,
		})
	}
	return out
}

// --- update -----------------------------------------------------------------

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.resize()
		return m, nil

	case StatusMsg:
		m.status = msg.Text
		return m, nil

	case tea.KeyMsg:
		filtering := m.list.FilterState() == list.Filtering
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			if !filtering {
				return m, tea.Quit
			}
		case "enter":
			if !filtering {
				return m.drillDown()
			}
		case "o":
			if !filtering {
				if it, ok := m.list.SelectedItem().(catItem); ok && actionable(it.kind) {
					if it.kind == kindEnvironment {
						m.status = "Los entornos compose no tienen editor asociado"
						return m, nil
					}
					c := *it.component
					m.status = ""
					return m, func() tea.Msg { return OpenEditorMsg{Component: c} }
				}
			}
		case "b":
			if !filtering {
				if it, ok := m.list.SelectedItem().(catItem); ok && actionable(it.kind) {
					if it.kind == kindEnvironment {
						m.status = "Los entornos compose no tienen repo asociado"
						return m, nil
					}
					if it.component.Repo == "" {
						m.status = "No tiene repo configurado"
						return m, nil
					}
					url := it.component.Repo
					m.status = "Abriendo repo en el navegador…"
					return m, func() tea.Msg { return OpenRepoMsg{URL: url} }
				}
			}
		case "r":
			if !filtering {
				if it, ok := m.list.SelectedItem().(catItem); ok && actionable(it.kind) {
					if it.kind == kindEnvironment {
						return m.openEnvironmentCommands(it)
					}
					c := *it.component
					m.enrichCommandHints(&c)
					m.status = ""
					return m, func() tea.Msg { return OpenCommandsMsg{Component: c} }
				}
			}
		case "d":
			if !filtering {
				if it, ok := m.list.SelectedItem().(catItem); ok && it.kind == kindProject && it.project != nil {
					dir := it.project.DiagramsDir()
					if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
						p := *it.project
						return m, func() tea.Msg { return OpenDiagramsMsg{Project: p} }
					}
				}
			}
		case "j":
			if !filtering {
				m.status = ""
				return m, func() tea.Msg { return OpenJobsMsg{} }
			}
		case "s":
			if !filtering {
				m.status = ""
				return m, func() tea.Msg { return OpenSchemaPickerMsg{} }
			}
		case "esc":
			if m.list.FilterState() == list.Unfiltered {
				return m.goUp()
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// OpenComponentMsg is emitted when the user presses enter on a component, so
// the root can push the docs screen (ADR-0002/0006).
type OpenComponentMsg struct{ Component config.Component }

// OpenEditorMsg asks the root to push the editor picker for a component.
type OpenEditorMsg struct{ Component config.Component }

// OpenRepoMsg asks the root to open a repo URL in the browser.
type OpenRepoMsg struct{ URL string }

// OpenCommandsMsg asks the root to push the command picker for a component.
type OpenCommandsMsg struct{ Component config.Component }

// OpenJobsMsg asks the root to push the jobs screen.
type OpenJobsMsg struct{}

// OpenDiagramsMsg asks the root to push the diagrams viewer for a project.
type OpenDiagramsMsg struct{ Project config.Project }

// OpenSchemaPickerMsg asks the root to open the local schema selector.
type OpenSchemaPickerMsg struct{}

// StatusMsg lets the root push a transient status line into the catalog
// (e.g. the result of launching an editor).
type StatusMsg struct{ Text string }

func (m Model) drillDown() (Model, tea.Cmd) {
	it, ok := m.list.SelectedItem().(catItem)
	if !ok {
		return m, nil
	}
	m.status = ""
	switch it.kind {
	case kindCustomer:
		m.curCustomer = it.customer
		m.level = levelProjects
		m.setItems(projectItems(it.customer))
	case kindProject:
		m.curProject = it.project
		m.level = levelProject
		m.setItems(projectChildItems(it.project))
	case kindComponent:
		c := *it.component
		return m, func() tea.Msg { return OpenComponentMsg{Component: c} }
	case kindCICD:
		c := *it.component
		return m, func() tea.Msg { return OpenCommandsMsg{Component: c} }
	case kindEnvironment:
		return m.openEnvironmentCommands(it)
	case kindLayerHeader:
		// no-op
	}
	return m, nil
}

func (m Model) openEnvironmentCommands(it catItem) (Model, tea.Cmd) {
	if it.environment == nil || m.curProject == nil {
		return m, nil
	}
	c, ok := it.environment.Component(m.curProject.Name)
	if !ok || len(c.RunnableCommands()) == 0 {
		m.status = "El entorno no tiene stacks compose configurados"
		return m, nil
	}
	m.status = ""
	return m, func() tea.Msg { return OpenCommandsMsg{Component: c} }
}

func (m Model) enrichCommandHints(c *config.Component) {
	if c == nil || m.curProject == nil {
		return
	}
	config.EnrichStackAdminHints(c, m.curProject.Environments)
}

func (m Model) goUp() (Model, tea.Cmd) {
	m.status = ""
	switch m.level {
	case levelProject:
		m.level = levelProjects
		m.setItems(projectItems(m.curCustomer))
	case levelProjects:
		m.level = levelCustomers
		m.curCustomer = nil
		m.setItems(customerItems(m.cfg))
	}
	return m, nil
}

func (m *Model) setItems(items []list.Item) {
	m.list.SetItems(items)
	m.list.Select(0)
}

func (m *Model) resize() {
	if m.w == 0 {
		return
	}
	m.list.SetSize(m.listWidth(), m.contentHeight())
}

func (m Model) listWidth() int {
	w := m.w * 38 / 100
	if w < 28 {
		w = 28
	}
	return w
}

func (m Model) contentHeight() int {
	h := m.h - 2 // breadcrumb + footer
	if h < 3 {
		h = 3
	}
	return h
}

// --- view -------------------------------------------------------------------

func (m Model) View() string {
	if m.w == 0 {
		return "" // not sized yet
	}
	contentH := m.contentHeight()
	detailW := m.w - m.listWidth() - 1
	if detailW < 12 {
		detailW = 12
	}
	innerW := detailW - 5 // border(1) + padding(4)
	if innerW < 8 {
		innerW = 8
	}

	left := m.list.View()
	right := m.detail.
		Width(innerW).Height(contentH).
		MaxWidth(detailW).MaxHeight(contentH).
		Render(m.renderDetail())
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	return lipgloss.JoinVertical(lipgloss.Left, m.breadcrumb(), body, m.footerLine())
}

func (m Model) breadcrumb() string {
	parts := []string{"Catálogo"}
	if m.curCustomer != nil {
		parts = append(parts, m.curCustomer.Name)
	}
	if m.level == levelProject && m.curProject != nil {
		parts = append(parts, m.curProject.Name)
	}
	styled := make([]string, len(parts))
	for i, p := range parts {
		styled[i] = m.bread.Render(p)
	}
	return " " + strings.Join(styled, m.sep.Render(" / "))
}

func (m Model) footerLine() string {
	action := ""
	if it, ok := m.list.SelectedItem().(catItem); ok {
		switch it.kind {
		case kindComponent:
			action = " · enter docs · r comandos · o editor · b repo"
		case kindCICD:
			action = " · enter pipelines · o editor · b repo"
		case kindEnvironment:
			if it.environment != nil && it.environment.Compose != nil {
				action = " · enter/r stacks compose"
			}
		case kindProject:
			if it.project != nil {
				dir := it.project.DiagramsDir()
				if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
					action = " · d diagramas"
				}
			}
		}
	}
	jobs := ""
	if n := m.mgr.RunningCount(); n > 0 {
		jobs = "   " + m.hint.Render(fmt.Sprintf("▶ %d corriendo (j)", n))
	}
	keys := m.footer.Render(" ↑/↓ mover · enter entrar" + action + " · j trabajos · s schema · q salir")
	if m.status != "" {
		return keys + "   " + m.hint.Render(m.status) + jobs
	}
	return keys + jobs
}

func (m Model) renderDetail() string {
	it, ok := m.list.SelectedItem().(catItem)
	if !ok {
		return m.dLabel.Render("(vacío)")
	}
	switch it.kind {
	case kindCustomer:
		return m.detailCustomer(it.customer)
	case kindProject:
		return m.detailProject(it.project)
	case kindComponent:
		return m.detailComponent(it.component)
	case kindCICD:
		return m.detailCICD(it.component)
	case kindEnvironment:
		return m.detailEnvironment(it.environment)
	}
	return ""
}

func (m Model) detailCICD(c *config.Component) string {
	var b strings.Builder
	b.WriteString(m.dTitle.Render("⚙ "+c.Name) + "\n")
	if c.Description != "" {
		b.WriteString(m.dLabel.Render(c.Description) + "\n")
	}
	b.WriteString("\n")
	writeRow(&b, m.row("folder", c.Folder))
	writeRow(&b, m.row("repo", c.Repo))
	b.WriteString("\n" + m.dLabel.Render("pipelines:") + "\n")
	for _, rc := range c.RunnableCommands() {
		b.WriteString("  " + m.dValue.Render(rc.Name) + "  " + m.dLabel.Render(rc.Cmd) + "\n")
		for _, hint := range rc.Hints {
			if hint.Label == "" || hint.Cmd == "" {
				continue
			}
			b.WriteString("    " + m.dLabel.Render(hint.Label) + "  " + m.dValue.Render(hint.Cmd) + "\n")
		}
	}
	return b.String()
}

func (m Model) row(label, value string) string {
	if value == "" {
		return ""
	}
	return m.dLabel.Render(fmt.Sprintf("%-11s", label)) + " " + m.dValue.Render(value)
}

func writeRow(b *strings.Builder, s string) {
	if strings.TrimSpace(s) == "" {
		return
	}
	b.WriteString(s + "\n")
}

func (m Model) detailCustomer(c *config.Customer) string {
	var b strings.Builder
	b.WriteString(m.dTitle.Render(c.Name) + "\n\n")
	writeRow(&b, m.row("website", c.Website))
	writeRow(&b, m.row("proyectos", fmt.Sprintf("%d", len(c.Projects))))
	b.WriteString("\n" + m.dLabel.Render("proyectos:") + "\n")
	for i := range c.Projects {
		b.WriteString("  " + m.dValue.Render(c.Projects[i].Name) + "\n")
	}
	return b.String()
}

func (m Model) detailProject(p *config.Project) string {
	var b strings.Builder
	b.WriteString(m.dTitle.Render(p.Name) + "\n\n")
	writeRow(&b, m.row("folder", p.Folder))
	writeRow(&b, m.row("componentes", fmt.Sprintf("%d", len(p.Components))))
	writeRow(&b, m.row("entornos", fmt.Sprintf("%d", len(p.Environments))))
	return b.String()
}

func (m Model) detailComponent(c *config.Component) string {
	var b strings.Builder
	b.WriteString(m.dTitle.Render("◆ "+c.Name) + "\n")
	if c.Description != "" {
		b.WriteString(m.dLabel.Render(c.Description) + "\n")
	}
	b.WriteString("\n")
	writeRow(&b, m.row("type", c.Type))
	if len(c.Tags) > 0 {
		writeRow(&b, m.dLabel.Render(fmt.Sprintf("%-11s", "tags"))+" "+m.dTag.Render(strings.Join(c.Tags, " ")))
	}
	writeRow(&b, m.row("repo", c.Repo))
	writeRow(&b, m.row("repo-path", c.RepoPath))
	writeRow(&b, m.row("folder", c.Folder))
	writeRow(&b, m.row("src", c.SrcFolder))
	writeRow(&b, m.row("tests", c.TestFolder))
	writeRow(&b, m.row("scripts", c.ScriptsFolder))
	b.WriteString("\n" + m.dLabel.Render("commands:") + "\n")
	writeRow(&b, m.row("  run", c.Commands.Run))
	writeRow(&b, m.row("  test", c.Commands.Test))
	writeRow(&b, m.row("  build", c.Commands.Build))
	return b.String()
}

func (m Model) detailEnvironment(e *config.Environment) string {
	var b strings.Builder
	b.WriteString(m.dTitle.Render("▣ "+e.Name) + "\n\n")
	writeRow(&b, m.row("ide", e.IDE))

	if len(e.URLs) > 0 {
		b.WriteString(m.dLabel.Render("urls:") + "\n")
		for i, u := range e.URLs {
			if i >= maxURLsShown {
				b.WriteString(m.dLabel.Render(fmt.Sprintf("  … +%d más", len(e.URLs)-maxURLsShown)) + "\n")
				break
			}
			b.WriteString("  " + m.dValue.Render(u.Name) + "  " + m.dLabel.Render(u.URL) + "\n")
		}
	}

	if e.Compose != nil {
		b.WriteString("\n" + m.dLabel.Render("compose:") + "\n")
		writeRow(&b, m.row("  dir", e.Compose.Dir))
		writeRow(&b, m.row("  env-file", e.Compose.EnvFile))
		b.WriteString(m.dLabel.Render(fmt.Sprintf("  stacks (%d):", len(e.Compose.Stacks))) + "\n")
		for i, s := range e.Compose.Stacks {
			if i >= maxStacksShown {
				b.WriteString(m.dLabel.Render(fmt.Sprintf("    … +%d más", len(e.Compose.Stacks)-maxStacksShown)) + "\n")
				break
			}
			line := "    " + m.dValue.Render(s.Name)
			if len(s.Tags) > 0 {
				line += " " + m.dTag.Render("["+strings.Join(s.Tags, ",")+"]")
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n" + m.dLabel.Render("acciones: enter/r abre up, ps, logs, config-check, restart y down por stack") + "\n")
	}
	return b.String()
}
