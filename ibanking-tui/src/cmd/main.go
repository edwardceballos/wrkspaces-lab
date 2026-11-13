package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/edwardceballos/control/internal/config"
	"github.com/edwardceballos/control/internal/editor"
	"github.com/edwardceballos/control/internal/runner"
	"github.com/edwardceballos/control/internal/theme"
	"github.com/edwardceballos/control/internal/tui/catalog"
	"github.com/edwardceballos/control/internal/tui/cmdpicker"
	"github.com/edwardceballos/control/internal/tui/diagrams"
	"github.com/edwardceballos/control/internal/tui/docs"
	"github.com/edwardceballos/control/internal/tui/jobs"
	"github.com/edwardceballos/control/internal/tui/picker"
	"github.com/edwardceballos/control/internal/tui/runview"
	"github.com/edwardceballos/control/internal/tui/schemapicker"
	"github.com/edwardceballos/control/internal/tui/splash"
)

// screen is a node in the navigation stack (ADR-0002).
type screen interface {
	Update(tea.Msg) (screen, tea.Cmd)
	View() string
	Init() tea.Cmd
}

// --- adapters ---------------------------------------------------------------

type splashScreen struct{ m splash.Model }

func (s splashScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	nm, cmd := s.m.Update(msg)
	s.m = nm.(splash.Model)
	return s, cmd
}
func (s splashScreen) View() string  { return s.m.View() }
func (s splashScreen) Init() tea.Cmd { return s.m.Init() }

type catalogScreen struct{ m catalog.Model }

func (s catalogScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	nm, cmd := s.m.Update(msg)
	s.m = nm
	return s, cmd
}
func (s catalogScreen) View() string  { return s.m.View() }
func (s catalogScreen) Init() tea.Cmd { return s.m.Init() }

type docsScreen struct{ m docs.Model }

func (s docsScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	nm, cmd := s.m.Update(msg)
	s.m = nm
	return s, cmd
}
func (s docsScreen) View() string  { return s.m.View() }
func (s docsScreen) Init() tea.Cmd { return s.m.Init() }

type pickerScreen struct{ m picker.Model }

func (s pickerScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	nm, cmd := s.m.Update(msg)
	s.m = nm
	return s, cmd
}
func (s pickerScreen) View() string  { return s.m.View() }
func (s pickerScreen) Init() tea.Cmd { return s.m.Init() }

type cmdpickerScreen struct{ m cmdpicker.Model }

func (s cmdpickerScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	nm, cmd := s.m.Update(msg)
	s.m = nm
	return s, cmd
}
func (s cmdpickerScreen) View() string  { return s.m.View() }
func (s cmdpickerScreen) Init() tea.Cmd { return s.m.Init() }

type runScreen struct{ m runview.Model }

func (s runScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	nm, cmd := s.m.Update(msg)
	s.m = nm
	return s, cmd
}
func (s runScreen) View() string  { return s.m.View() }
func (s runScreen) Init() tea.Cmd { return s.m.Init() }

type jobsScreen struct{ m jobs.Model }

func (s jobsScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	nm, cmd := s.m.Update(msg)
	s.m = nm
	return s, cmd
}
func (s jobsScreen) View() string  { return s.m.View() }
func (s jobsScreen) Init() tea.Cmd { return s.m.Init() }

type diagramsScreen struct{ m diagrams.Model }

func (s diagramsScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	nm, cmd := s.m.Update(msg)
	s.m = nm
	return s, cmd
}
func (s diagramsScreen) View() string  { return s.m.View() }
func (s diagramsScreen) Init() tea.Cmd { return s.m.Init() }

type schemaPickerScreen struct{ m schemapicker.Model }

func (s schemaPickerScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	nm, cmd := s.m.Update(msg)
	s.m = nm
	return s, cmd
}
func (s schemaPickerScreen) View() string  { return s.m.View() }
func (s schemaPickerScreen) Init() tea.Cmd { return s.m.Init() }

// --- config loading ---------------------------------------------------------

type configLoadedMsg struct {
	cfg   *config.Config
	warns []string
	err   error
}

func loadConfigCmd(path string) tea.Cmd {
	return func() tea.Msg {
		var p string
		var err error
		if path != "" {
			var ok bool
			p, ok, err = config.ResolveOverride(path)
			if err != nil {
				return configLoadedMsg{err: err}
			}
			if !ok {
				return configLoadedMsg{err: fmt.Errorf("schema explícito vacío")}
			}
		} else {
			p, err = config.Resolve("")
			if err != nil {
				return configLoadedMsg{err: err}
			}
		}
		cfg, err := config.Load(p)
		if err != nil {
			return configLoadedMsg{err: err}
		}
		errs, warns := cfg.Validate()
		if len(errs) > 0 {
			return configLoadedMsg{cfg: cfg, warns: warns, err: joinErrs(errs)}
		}
		return configLoadedMsg{cfg: cfg, warns: warns}
	}
}

func joinErrs(errs []error) error {
	if len(errs) == 1 {
		return errs[0]
	}
	msg := fmt.Sprintf("%d errores de validación:", len(errs))
	for _, e := range errs {
		msg += "\n  - " + e.Error()
	}
	return fmt.Errorf("%s", msg)
}

// --- root -------------------------------------------------------------------

type rootModel struct {
	stack []screen
	th    theme.Theme
	mgr   *runner.Manager

	cfg     *config.Config
	cfgPath string
	warns   []string
	loadErr error
	w, h    int
	done    bool

	initialSchemaPath string
	loadOnInit        bool
}

func (m *rootModel) push(s screen) tea.Cmd {
	m.stack = append(m.stack, s)
	idx := len(m.stack) - 1
	var cmds []tea.Cmd
	if m.w > 0 {
		var cmd tea.Cmd
		m.stack[idx], cmd = m.stack[idx].Update(tea.WindowSizeMsg{Width: m.w, Height: m.h})
		cmds = append(cmds, cmd)
	}
	cmds = append(cmds, m.stack[idx].Init())
	return tea.Batch(cmds...)
}

func (m *rootModel) pop() {
	if len(m.stack) > 1 {
		m.stack = m.stack[:len(m.stack)-1]
	}
}

func (m rootModel) top() screen { return m.stack[len(m.stack)-1] }

func (m *rootModel) updateTop(msg tea.Msg) tea.Cmd {
	top, cmd := m.top().Update(msg)
	m.stack[len(m.stack)-1] = top
	return cmd
}

func (m rootModel) Init() tea.Cmd {
	cmds := []tea.Cmd{m.stack[0].Init()}
	if m.loadOnInit {
		cmds = append(cmds, loadConfigCmd(m.initialSchemaPath))
	}
	return tea.Batch(cmds...)
}

func (m rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		var cmds []tea.Cmd
		for i := range m.stack {
			var cmd tea.Cmd
			m.stack[i], cmd = m.stack[i].Update(msg)
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

	case configLoadedMsg:
		m.cfg, m.warns, m.loadErr = msg.cfg, msg.warns, msg.err
		return m, m.updateTop(splash.LoadedMsg{Err: msg.err})

	case schemapicker.ChosenMsg:
		m.initialSchemaPath = msg.Schema.Path
		m.stack = []screen{splashScreen{m: splash.New(m.th)}}
		var cmds []tea.Cmd
		if m.w > 0 {
			var resizeCmd tea.Cmd
			m.stack[0], resizeCmd = m.stack[0].Update(tea.WindowSizeMsg{Width: m.w, Height: m.h})
			cmds = append(cmds, resizeCmd)
		}
		cmds = append(cmds, m.stack[0].Init(), loadConfigCmd(msg.Schema.Path))
		return m, tea.Batch(cmds...)

	case schemapicker.CancelMsg:
		if len(m.stack) > 1 {
			m.pop()
			return m, nil
		}
		return m, tea.Quit

	case splash.DoneMsg:
		if m.loadErr != nil {
			m.done = true
			return m, tea.Quit
		}
		m.th = theme.Load(m.cfg.Theme())
		m.stack = m.stack[:0]
		return m, m.push(catalogScreen{m: catalog.New(m.th, m.cfg, m.mgr)})

	// --- diagrams ---
	case catalog.OpenDiagramsMsg:
		return m, m.push(diagramsScreen{m: diagrams.New(msg.Project, m.th)})
	case catalog.OpenSchemaPickerMsg:
		schemas, err := config.DiscoverLocalSchemas(".")
		if err != nil {
			return m, m.reportStatus(fmt.Sprintf("No se pudieron listar schemas: %v", err))
		}
		if len(schemas) < 2 {
			return m, m.reportStatus("No hay otro schema*.yml disponible")
		}
		return m, m.push(schemaPickerScreen{m: schemapicker.New(m.th, schemas)})
	case diagrams.PopMsg:
		m.pop()
		return m, nil

	// --- docs / editor / repo ---
	case catalog.OpenComponentMsg:
		return m, m.push(docsScreen{m: docs.New(m.th, msg.Component)})
	case catalog.OpenEditorMsg:
		return m, m.push(pickerScreen{m: picker.New(m.th, msg.Component)})
	case catalog.OpenRepoMsg:
		return m, editor.OpenURL(msg.URL)
	case picker.ChosenMsg:
		m.pop()
		return m, msg.Editor.Launch(msg.Path)
	case picker.CancelMsg:
		m.pop()
		return m, nil
	case editor.LaunchedMsg:
		return m, m.reportStatus(launchText(msg))
	case docs.PopMsg:
		m.pop()
		return m, nil

	// --- commands / runner / jobs ---
	case catalog.OpenCommandsMsg:
		return m, m.push(cmdpickerScreen{m: cmdpicker.New(m.th, msg.Component)})
	case catalog.OpenJobsMsg:
		return m, m.push(jobsScreen{m: jobs.New(m.th, m.mgr)})

	case cmdpicker.ChosenMsg:
		m.pop()
		dir := msg.Component.WorkingDir(msg.Cmd)
		label := msg.Component.Name + " · " + msg.Cmd.Name
		if msg.TTY {
			c := exec.Command("sh", "-c", msg.Cmd.Cmd)
			c.Dir = dir
			return m, tea.ExecProcess(c, func(err error) tea.Msg { return runner.TTYDoneMsg{Err: err} })
		}
		job, wait := m.mgr.StartWithDetails(label, msg.Cmd.Cmd, dir, commandDetails(msg.Cmd.Hints))
		pushCmd := m.push(runScreen{m: runview.New(m.th, job)})
		return m, tea.Batch(pushCmd, wait)
	case cmdpicker.CancelMsg:
		m.pop()
		return m, nil

	case runner.OutputMsg:
		if job := m.mgr.Get(msg.ID); job != nil {
			job.Append(msg.Line)
		}
		fwd := m.updateTop(msg) // refresh visible run view / jobs list
		return m, tea.Batch(fwd, m.mgr.Wait(msg.ID))
	case runner.DoneMsg:
		m.mgr.Finish(msg.ID, msg.Err)
		return m, m.updateTop(msg)
	case runner.TTYDoneMsg:
		txt := "✓ comando finalizado"
		if msg.Err != nil {
			txt = fmt.Sprintf("✗ comando: %v", msg.Err)
		}
		return m, m.reportStatus(txt)

	case runview.PopMsg:
		m.pop()
		return m, nil
	case runview.KillMsg:
		m.mgr.Kill(msg.ID)
		return m, nil

	case jobs.AttachMsg:
		if job := m.mgr.Get(msg.JobID); job != nil {
			return m, m.push(runScreen{m: runview.New(m.th, job)})
		}
		return m, nil
	case jobs.KillMsg:
		m.mgr.Kill(msg.JobID)
		return m, nil
	case jobs.CloseMsg:
		m.pop()
		return m, nil
	}

	return m, m.updateTop(msg)
}

// reportStatus pushes a transient status line into the catalog if it's on top.
func (m *rootModel) reportStatus(text string) tea.Cmd {
	if text == "" {
		return nil
	}
	return m.updateTop(catalog.StatusMsg{Text: text})
}

func launchText(msg editor.LaunchedMsg) string {
	if msg.Err != nil {
		return fmt.Sprintf("✗ no se pudo abrir (%s): %v", msg.Editor, msg.Err)
	}
	if msg.Editor != "browser" {
		return fmt.Sprintf("✓ abierto en %s", msg.Editor)
	}
	return ""
}

func commandDetails(hints []config.CmdHint) []runner.CommandDetail {
	if len(hints) == 0 {
		return nil
	}
	out := make([]runner.CommandDetail, 0, len(hints))
	for _, hint := range hints {
		if hint.Label == "" || hint.Cmd == "" {
			continue
		}
		out = append(out, runner.CommandDetail{Label: hint.Label, Cmd: hint.Cmd})
	}
	return out
}

func (m rootModel) View() string {
	if m.done {
		return ""
	}
	return m.top().View()
}

func main() {
	schemaPath := flag.String("schema", "", "ruta del schema.yml a cargar")
	flag.Parse()

	mgr := runner.NewManager()
	th := theme.Load("gruvbox")
	stack, initialSchemaPath, loadOnInit := initialStack(th, *schemaPath)
	root := rootModel{
		stack:             stack,
		th:                th,
		mgr:               mgr,
		initialSchemaPath: initialSchemaPath,
		loadOnInit:        loadOnInit,
	}

	final, err := tea.NewProgram(root).Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	rm, _ := final.(rootModel)
	if rm.loadErr != nil {
		fmt.Fprintln(os.Stderr, "✗ carga del catálogo falló:")
		fmt.Fprintln(os.Stderr, rm.loadErr)
		os.Exit(1)
	}
	if rm.cfg != nil {
		s := rm.cfg.Stats()
		fmt.Printf("✓ catálogo: %d customers · %d projects · %d components · %d environments · %d stacks · %d pipelines (theme: %s)\n",
			s.Customers, s.Projects, s.Components, s.Environments, s.Stacks, s.Pipelines, rm.cfg.Theme())
		for _, w := range rm.warns {
			fmt.Printf("  ⚠ %s\n", w)
		}
	}
}

func initialStack(th theme.Theme, explicit string) ([]screen, string, bool) {
	if explicit != "" || os.Getenv(config.EnvConfigPath) != "" {
		return []screen{splashScreen{m: splash.New(th)}}, explicit, true
	}

	schemas, err := config.DiscoverLocalSchemas(".")
	if err == nil {
		switch len(schemas) {
		case 0:
			return []screen{splashScreen{m: splash.New(th)}}, "", true
		case 1:
			return []screen{splashScreen{m: splash.New(th)}}, schemas[0].Path, true
		default:
			return []screen{schemaPickerScreen{m: schemapicker.New(th, schemas)}}, "", false
		}
	}
	return []screen{splashScreen{m: splash.New(th)}}, "", true
}
