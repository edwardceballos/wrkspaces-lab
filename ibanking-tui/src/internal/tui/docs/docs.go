// Package docs renders a component's Markdown documentation (TechDocs-lite, ADR-0006).
// Discovery: srcfolder first, then folder (fallback), recursively.
// File I/O and markdown rendering happen in a tea.Cmd, never in Update/View.
package docs

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/edwardceballos/control/internal/config"
	"github.com/edwardceballos/control/internal/theme"
)

// PopMsg tells the root to pop this screen off the stack.
type PopMsg struct{}

// docsMsg is the result of the async load.
type docsMsg struct {
	rendered string // glamour output (or raw fallback)
	paths    []string
	found    bool
	err      error
}

type Model struct {
	th      theme.Theme
	comp    config.Component
	vp      viewport.Model
	w, h    int
	ready   bool
	loading bool
	found   bool
	srcPath string
	loadErr error

	header lipgloss.Style
	dim    lipgloss.Style
	footer lipgloss.Style
	errSt  lipgloss.Style
}

func New(th theme.Theme, comp config.Component) Model {
	return Model{
		th:      th,
		comp:    comp,
		loading: true,
		header:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(th.Orange)),
		dim:     lipgloss.NewStyle().Foreground(lipgloss.Color(th.Gray)),
		footer:  lipgloss.NewStyle().Foreground(lipgloss.Color(th.Gray)),
		errSt:   lipgloss.NewStyle().Foreground(lipgloss.Color(th.Red)),
	}
}

// Init does not load yet: the load needs the real terminal width for correct
// word-wrap. The root's push() sends a WindowSizeMsg immediately, which is
// where the load is kicked off (see Update). This avoids a wrong-width render.
func (m Model) Init() tea.Cmd { return nil }

// loadDocsCmd finds and renders Markdown files off the UI goroutine.
func loadDocsCmd(c config.Component, width int) tea.Cmd {
	return func() tea.Msg {
		paths := findMarkdownFiles(c)
		if len(paths) == 0 {
			return docsMsg{found: false}
		}
		raw, err := buildMarkdownBundle(paths)
		if err != nil {
			return docsMsg{found: true, paths: paths, err: err}
		}
		r, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle("dark"), // gruvbox-custom glamour style = future (ADR-0007)
			glamour.WithWordWrap(wrapWidth(width)),
		)
		if err != nil {
			return docsMsg{found: true, paths: paths, rendered: raw} // raw fallback
		}
		out, err := r.Render(raw)
		if err != nil {
			return docsMsg{found: true, paths: paths, rendered: raw}
		}
		return docsMsg{found: true, paths: paths, rendered: out}
	}
}

// findMarkdownFiles walks srcfolder then folder, returning unique Markdown files.
func findMarkdownFiles(c config.Component) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, dir := range []string{c.SrcFolder, c.Folder} {
		if dir == "" {
			continue
		}
		files, err := markdownFilesInDir(dir)
		if err != nil {
			continue
		}
		for _, file := range files {
			key := filepath.Clean(file)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, file)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return markdownRank(out[i]) < markdownRank(out[j])
	})
	return out
}

func markdownFilesInDir(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if shouldSkipDir(d.Name()) && filepath.Clean(path) != filepath.Clean(root) {
				return filepath.SkipDir
			}
			return nil
		}
		if isMarkdownFile(d.Name()) {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

func shouldSkipDir(name string) bool {
	switch strings.ToLower(name) {
	case ".git", ".hg", ".svn", ".idea", ".vscode", ".nx", ".next", "node_modules",
		"bin", "obj", "dist", "build", "coverage", "target", "vendor":
		return true
	default:
		return false
	}
}

func isMarkdownFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown", ".mdown":
		return true
	default:
		return false
	}
}

func markdownRank(path string) int {
	name := strings.ToLower(filepath.Base(path))
	switch {
	case name == "readme.md" || name == "readme.markdown" || name == "readme.mdown":
		return 0
	case strings.Contains(strings.ToLower(path), string(filepath.Separator)+"docs"+string(filepath.Separator)):
		return 1
	default:
		return 2
	}
}

func buildMarkdownBundle(paths []string) (string, error) {
	var b strings.Builder
	b.WriteString("# Documentación\n\n")
	b.WriteString("## Índice\n\n")
	for i, path := range paths {
		b.WriteString(fmt.Sprintf("%d. `%s`\n", i+1, path))
	}

	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		b.WriteString("\n\n---\n\n")
		b.WriteString("## ")
		b.WriteString(path)
		b.WriteString("\n\n")
		b.Write(raw)
		if !bytes.HasSuffix(raw, []byte("\n")) {
			b.WriteString("\n")
		}
	}
	return b.String(), nil
}

func wrapWidth(w int) int {
	ww := w - 4
	if ww < 20 {
		ww = 20
	}
	if ww > 120 {
		ww = 120 // keep long-line readability
	}
	return ww
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		first := m.w == 0
		m.w, m.h = msg.Width, msg.Height
		m.vp.Width = m.w
		m.vp.Height = m.contentHeight()
		m.ready = true
		switch {
		case first:
			// First sizing: kick the load at the real width.
			return m, loadDocsCmd(m.comp, m.w)
		case m.found && !m.loading:
			// Resize after load: re-render markdown at the new wrap width.
			return m, loadDocsCmd(m.comp, m.w)
		}
		return m, nil

	case docsMsg:
		m.loading = false
		m.found = msg.found
		m.srcPath = pathsSummary(msg.paths)
		m.loadErr = msg.err
		if msg.found && msg.err == nil {
			if !m.ready {
				m.vp = viewport.New(m.w, m.contentHeight())
				m.ready = m.w > 0
			}
			m.vp.SetContent(msg.rendered)
			m.vp.GotoTop()
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q", "backspace":
			return m, func() tea.Msg { return PopMsg{} }
		case "ctrl+c":
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

func (m Model) contentHeight() int {
	h := m.h - 2 // header + footer
	if h < 3 {
		h = 3
	}
	return h
}

func (m Model) View() string {
	if m.w == 0 {
		return ""
	}
	title := m.header.Render("◆ "+m.comp.Name) + "  " + m.dim.Render("Markdown docs")

	var body string
	switch {
	case m.loading:
		body = m.dim.Render("Cargando documentación Markdown…")
	case m.loadErr != nil:
		body = m.errSt.Render("Error leyendo documentación: " + m.loadErr.Error())
	case !m.found:
		hint := "Sin archivos Markdown en:\n  " + dirOr(m.comp.SrcFolder, "(srcfolder vacío)") +
			"\n  " + dirOr(m.comp.Folder, "(folder vacío)")
		body = m.dim.Render(hint)
	default:
		body = m.vp.View()
	}

	footer := m.footer.Render(" ↑/↓ scroll · esc volver · q salir")
	if m.found && m.srcPath != "" {
		footer = m.dim.Render(" "+m.srcPath) + "   " + footer
	}
	return lipgloss.JoinVertical(lipgloss.Left, title, body, footer)
}

func pathsSummary(paths []string) string {
	switch len(paths) {
	case 0:
		return ""
	case 1:
		return paths[0]
	default:
		return fmt.Sprintf("%d archivos Markdown · %s", len(paths), paths[0])
	}
}

func dirOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
