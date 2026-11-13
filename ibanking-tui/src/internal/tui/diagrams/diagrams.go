// Package diagrams renders ASCII diagram files with a dissolve transition.
// Files are read from {project.Folder}/docs/diagrams/*.txt (alphabetical order).
// The dissolve replaces ~5% of the non-space positions per tick (50 ms) so that
// each transition takes roughly one second in each direction.
package diagrams

import (
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/edwardceballos/control/internal/config"
	"github.com/edwardceballos/control/internal/theme"
)

const (
	frameDur     = 50 * time.Millisecond
	idleDur      = 4 * time.Second
	dissolvePerc = 5 // % of total non-space positions to transition per tick
)

type phase int

const (
	phaseIdle phase = iota
	phaseFadeOut
	phaseFadeIn
)

// --- messages ---------------------------------------------------------------

type tickMsg time.Time
type idleExpiredMsg struct{}

type loadedMsg struct {
	diagrams   [][]rune
	maxW, maxH int
	err        error
}

// PopMsg is emitted on esc so the root can pop this screen.
type PopMsg struct{}

// --- model ------------------------------------------------------------------

type Model struct {
	project config.Project
	th      theme.Theme

	diagrams     [][]rune // flat rune canvas per file, size maxW*maxH
	current      int
	canvas       []rune // mutable display state
	pending      []int  // remaining positions to transition
	pendingTotal int    // initial len of pending (keeps dissolve rate constant)
	phase        phase
	maxW, maxH   int
	w, h         int
	paused       bool
	loaded       bool
	loadErr      error

	rowStyle    lipgloss.Style
	footerStyle lipgloss.Style
	errStyle    lipgloss.Style
}

func New(project config.Project, th theme.Theme) Model {
	return Model{
		project:     project,
		th:          th,
		w:           80,
		h:           24,
		rowStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color(th.Fg)),
		footerStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(th.Gray)),
		errStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color(th.Red)),
	}
}

func (m Model) Init() tea.Cmd {
	return loadFiles(m.project.DiagramsDir())
}

func loadFiles(dir string) tea.Cmd {
	return func() tea.Msg {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return loadedMsg{err: fmt.Errorf("no se puede leer %s: %w", dir, err)}
		}

		var files []string
		for _, e := range entries {
			if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".txt") {
				files = append(files, filepath.Join(dir, e.Name()))
			}
		}
		sort.Strings(files)

		if len(files) == 0 {
			return loadedMsg{err: fmt.Errorf("no hay archivos .txt en %s", dir)}
		}

		var allLines [][]string
		maxW, maxH := 0, 0
		for _, f := range files {
			data, readErr := os.ReadFile(f)
			if readErr != nil {
				continue
			}
			text := strings.ReplaceAll(string(data), "\r\n", "\n")
			text = strings.TrimRight(text, "\n")
			lines := strings.Split(text, "\n")
			allLines = append(allLines, lines)
			if len(lines) > maxH {
				maxH = len(lines)
			}
			for _, l := range lines {
				if w := len([]rune(l)); w > maxW {
					maxW = w
				}
			}
		}

		if len(allLines) == 0 || maxW == 0 || maxH == 0 {
			return loadedMsg{err: fmt.Errorf("diagramas vacíos en %s", dir)}
		}

		diagrams := make([][]rune, len(allLines))
		for i, lines := range allLines {
			canvas := make([]rune, maxW*maxH)
			for j := range canvas {
				canvas[j] = ' '
			}
			for row, line := range lines {
				for col, r := range []rune(line) {
					canvas[row*maxW+col] = r
				}
			}
			diagrams[i] = canvas
		}

		return loadedMsg{diagrams: diagrams, maxW: maxW, maxH: maxH}
	}
}

// --- ticks ------------------------------------------------------------------

func tick() tea.Cmd {
	return tea.Tick(frameDur, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func idleTimer() tea.Cmd {
	return tea.Tick(idleDur, func(t time.Time) tea.Msg { return idleExpiredMsg{} })
}

// --- update -----------------------------------------------------------------

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil

	case loadedMsg:
		if msg.err != nil {
			m.loadErr = msg.err
			m.loaded = true
			return m, nil
		}
		m.diagrams = msg.diagrams
		m.maxW = msg.maxW
		m.maxH = msg.maxH
		m.current = 0
		m.canvas = make([]rune, len(m.diagrams[0]))
		copy(m.canvas, m.diagrams[0])
		m.loaded = true
		m.phase = phaseIdle
		if len(m.diagrams) > 1 && !m.paused {
			return m, idleTimer()
		}
		return m, nil

	case idleExpiredMsg:
		if m.paused || len(m.diagrams) <= 1 || m.phase != phaseIdle {
			return m, nil
		}
		m.phase = phaseFadeOut
		m.pending = nonSpacePositions(m.canvas)
		m.pendingTotal = len(m.pending)
		rand.Shuffle(len(m.pending), func(i, j int) { m.pending[i], m.pending[j] = m.pending[j], m.pending[i] })
		return m, tick()

	case tickMsg:
		return m.animateTick()

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return PopMsg{} }
		case "left", "h":
			return m.jumpTo(m.prevIndex())
		case "right", "l":
			return m.jumpTo(m.nextIndex())
		case " ":
			m.paused = !m.paused
			if !m.paused && m.phase == phaseIdle && len(m.diagrams) > 1 {
				return m, idleTimer()
			}
			return m, nil
		}
	}
	return m, nil
}

func (m Model) animateTick() (Model, tea.Cmd) {
	// Copy canvas so we don't alias the previous model state's underlying array.
	canvas := make([]rune, len(m.canvas))
	copy(canvas, m.canvas)
	m.canvas = canvas

	switch m.phase {
	case phaseFadeOut:
		n := min(max(1, m.pendingTotal*dissolvePerc/100), len(m.pending))
		for range n {
			m.canvas[m.pending[0]] = ' '
			m.pending = m.pending[1:]
		}

		if len(m.pending) == 0 {
			m.current = m.nextIndex()
			m.phase = phaseFadeIn
			target := m.diagrams[m.current]
			m.pending = nonSpacePositions(target)
			m.pendingTotal = len(m.pending)
			rand.Shuffle(len(m.pending), func(i, j int) { m.pending[i], m.pending[j] = m.pending[j], m.pending[i] })
		}
		return m, tick()

	case phaseFadeIn:
		target := m.diagrams[m.current]
		n := min(max(1, m.pendingTotal*dissolvePerc/100), len(m.pending))
		for range n {
			pos := m.pending[0]
			m.pending = m.pending[1:]
			m.canvas[pos] = target[pos]
		}

		if len(m.pending) == 0 {
			copy(m.canvas, target) // ensure no missed positions
			m.phase = phaseIdle
			if !m.paused {
				return m, idleTimer()
			}
		}
		return m, tick()
	}
	return m, nil
}

func (m Model) jumpTo(idx int) (Model, tea.Cmd) {
	if !m.loaded || m.loadErr != nil || len(m.diagrams) == 0 {
		return m, nil
	}
	m.current = idx
	m.canvas = make([]rune, len(m.diagrams[idx]))
	copy(m.canvas, m.diagrams[idx])
	m.phase = phaseIdle
	m.pending = nil
	m.pendingTotal = 0
	if !m.paused && len(m.diagrams) > 1 {
		return m, idleTimer()
	}
	return m, nil
}

func (m Model) nextIndex() int {
	if len(m.diagrams) == 0 {
		return 0
	}
	return (m.current + 1) % len(m.diagrams)
}

func (m Model) prevIndex() int {
	if len(m.diagrams) == 0 {
		return 0
	}
	return (m.current - 1 + len(m.diagrams)) % len(m.diagrams)
}

// --- view -------------------------------------------------------------------

func (m Model) View() string {
	if m.w == 0 {
		return ""
	}

	var bodyStr string
	switch {
	case !m.loaded:
		bodyStr = lipgloss.NewStyle().Foreground(lipgloss.Color(m.th.Gray)).Render("Cargando diagramas…")
	case m.loadErr != nil:
		bodyStr = m.errStyle.Render("✗ " + m.loadErr.Error())
	default:
		bodyStr = m.renderCanvas()
	}

	body := lipgloss.Place(m.w, m.h-1, lipgloss.Center, lipgloss.Center, bodyStr)
	footer := m.footerStyle.Render(m.footerText())
	return body + "\n" + footer
}

func (m Model) renderCanvas() string {
	if len(m.canvas) == 0 || m.maxW == 0 {
		return ""
	}
	var b strings.Builder
	for row := 0; row < m.maxH; row++ {
		line := string(m.canvas[row*m.maxW : (row+1)*m.maxW])
		line = strings.TrimRight(line, " ")
		b.WriteString(m.rowStyle.Render(line))
		if row < m.maxH-1 {
			b.WriteRune('\n')
		}
	}
	return b.String()
}

func (m Model) footerText() string {
	parts := []string{""}
	if len(m.diagrams) > 0 {
		parts = append(parts, fmt.Sprintf("%d/%d", m.current+1, len(m.diagrams)))
	}
	parts = append(parts, "←/→ navegar")
	if m.paused {
		parts = append(parts, "space reanudar [pausado]")
	} else {
		parts = append(parts, "space pausar")
	}
	parts = append(parts, "esc volver")
	return strings.Join(parts, " · ")
}

func nonSpacePositions(canvas []rune) []int {
	out := make([]int, 0, len(canvas)/2)
	for i, r := range canvas {
		if r != ' ' && r != 0 {
			out = append(out, i)
		}
	}
	return out
}
