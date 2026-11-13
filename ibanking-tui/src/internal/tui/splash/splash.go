// Package splash is the animated startup screen. It runs a frame-based
// animation (tea.Tick) while a load Cmd does real startup work (parsing
// schema.yml) in parallel, then emits DoneMsg so the root model can switch
// to the catalog screen. See ADR-0004 (no blocking in Update; all timing in Cmds).
package splash

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/edwardceballos/control/internal/theme"
)

// Tunables.
const (
	frameDur    = 50 * time.Millisecond // ~20 fps
	revealEvery = 3                     // frames between banner-line reveals
	minFrames   = 50                    // min on-screen time (~1.4s) before finishing
	fadeFrames  = 8                     // fade-out duration in frames
	rampSteps   = 48                    // gradient resolution
)

// ANSI Shadow "CONTROL".
var banner = []string{
	" ██████╗ ██████╗ ███╗   ██╗████████╗██████╗  ██████╗ ██╗     ",
	"██╔════╝██╔═══██╗████╗  ██║╚══██╔══╝██╔══██╗██╔═══██╗██║     ",
	"██║     ██║   ██║██╔██╗ ██║   ██║   ██████╔╝██║   ██║██║     ",
	"██║     ██║   ██║██║╚██╗██║   ██║   ██╔══██╗██║   ██║██║     ",
	"╚██████╗╚██████╔╝██║ ╚████║   ██║   ██║  ██║╚██████╔╝███████╗",
	" ╚═════╝ ╚═════╝ ╚═╝  ╚═══╝   ╚═╝   ╚═╝  ╚═╝ ╚═════╝ ╚══════╝",
}

var spinnerFrames = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

type phase int

const (
	phaseReveal phase = iota
	phaseLoading
	phaseFade
	phaseDone
)

// --- Messages ---------------------------------------------------------------

type frameMsg time.Time

// LoadedMsg must be emitted by the load Cmd passed to New when async startup
// work finishes (e.g. schema.yml parsed). Err non-nil = startup failed.
type LoadedMsg struct{ Err error }

// DoneMsg is emitted when the splash finishes. The root model should react by
// pushing the catalog screen (or quitting on Err).
type DoneMsg struct{ Err error }

// --- Model ------------------------------------------------------------------

type Model struct {
	th      theme.Theme
	ramp    []string
	w, h    int
	frame   int
	reveal  int // number of banner lines currently shown
	loaded  bool
	loadErr error
	phase   phase
	fade    int
}

// New builds the splash. The animation runs until it receives a LoadedMsg
// (sent by the root model when startup work finishes) AND the minimum display
// time has elapsed. The root orchestrates loading, not the splash (ADR-0002).
func New(th theme.Theme) Model {
	return Model{
		th:   th,
		ramp: th.Ramp(rampSteps),
		w:    80,
		h:    24,
	}
}

func (m Model) Init() tea.Cmd {
	return tick()
}

func tick() tea.Cmd {
	return tea.Tick(frameDur, func(t time.Time) tea.Msg { return frameMsg(t) })
}

func done(err error) tea.Cmd {
	return func() tea.Msg { return DoneMsg{Err: err} }
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		// Any key skips straight to the app (don't trap the user in the intro).
		m.phase = phaseDone
		return m, done(m.loadErr)

	case LoadedMsg:
		m.loaded = true
		m.loadErr = msg.Err
		return m, nil

	case frameMsg:
		m.frame++
		switch m.phase {
		case phaseReveal:
			if m.frame%revealEvery == 0 && m.reveal < len(banner) {
				m.reveal++
			}
			if m.reveal >= len(banner) {
				m.phase = phaseLoading
			}
			return m, tick()
		case phaseLoading:
			// Wait for BOTH: load finished AND minimum display time elapsed.
			if m.loaded && m.frame >= minFrames {
				m.phase = phaseFade
			}
			return m, tick()
		case phaseFade:
			m.fade++
			if m.fade > fadeFrames {
				m.phase = phaseDone
				return m, done(m.loadErr)
			}
			return m, tick()
		}
	}
	return m, nil
}

func (m Model) View() string {
	if m.phase == phaseDone {
		return ""
	}

	dim := 0.0
	if m.phase == phaseFade {
		dim = float64(m.fade) / float64(fadeFrames)
	}

	// Banner: reveal line by line; keep total height stable with blank lines.
	lines := make([]string, len(banner))
	for i, ln := range banner {
		if i < m.reveal {
			lines[i] = applyGradient(ln, m.ramp, m.frame, dim, m.th.Bg)
		} else {
			lines[i] = ""
		}
	}
	bannerBlock := lipgloss.JoinVertical(lipgloss.Center, lines...)

	subStyle := lipgloss.NewStyle().Bold(true).Foreground(dimColor(m.th.Fg, m.th.Bg, dim))
	verStyle := lipgloss.NewStyle().Foreground(dimColor(m.th.Gray, m.th.Bg, dim))

	sub := ""
	if m.reveal >= len(banner) {
		sub = subStyle.Render("edward's runbooks && idp")
	}

	status := ""
	switch {
	case m.phase == phaseLoading && !m.loaded:
		sp := string(spinnerFrames[m.frame%len(spinnerFrames)])
		status = lipgloss.NewStyle().Foreground(lipgloss.Color(m.th.Yellow)).
			Render(sp + " Cargando catálogo…")
	case m.loadErr != nil:
		status = lipgloss.NewStyle().Foreground(lipgloss.Color(m.th.Red)).
			Render("✗ Error al cargar el catálogo")
	case m.loaded:
		status = lipgloss.NewStyle().Foreground(lipgloss.Color(m.th.Green)).
			Render("✓ Listo")
	}

	content := lipgloss.JoinVertical(lipgloss.Center,
		bannerBlock,
		"",
		sub,
		verStyle.Render("v0.1.0"),
		"",
		status,
	)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(dimColor(m.th.Gray, m.th.Bg, dim)).
		Padding(1, 4).
		Render(content)

	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, box)
}

// applyGradient colors each non-space rune by a moving position in the ramp,
// producing a horizontal sweep that animates as `offset` (the frame) grows.
func applyGradient(line string, ramp []string, offset int, dim float64, bg string) string {
	if len(ramp) == 0 {
		return line
	}
	var b strings.Builder
	col := 0
	for _, r := range line {
		if r == ' ' {
			b.WriteRune(' ')
			col++
			continue
		}
		c := ramp[(col+offset)%len(ramp)]
		if dim > 0 {
			c = theme.LerpHex(c, bg, dim)
		}
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(c)).Render(string(r)))
		col++
	}
	return b.String()
}

func dimColor(fg, bg string, dim float64) lipgloss.Color {
	if dim <= 0 {
		return lipgloss.Color(fg)
	}
	return lipgloss.Color(theme.LerpHex(fg, bg, dim))
}
