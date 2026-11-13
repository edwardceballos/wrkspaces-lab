// Package runner is the process manager (ADR-0004). It lives at the root, so
// jobs keep running while the user navigates away. Output is delivered line by
// line over a channel; the buffer is mutated ONLY in the root's Update (single
// goroutine), so no mutex is needed on Job.Output. Goroutines touch only the
// channel and the process.
package runner

import (
	"bufio"
	"io"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
)

type Status int

const (
	Running Status = iota
	Exited
	Failed
	Killed
)

func (s Status) String() string {
	switch s {
	case Running:
		return "running"
	case Exited:
		return "exited"
	case Failed:
		return "failed"
	case Killed:
		return "killed"
	}
	return "?"
}

const maxLines = 5000 // bound memory; keep the most recent output

type Job struct {
	ID      int
	Label   string // e.g. "enterprise-banking · test"
	Line    string // shell command
	Dir     string
	Details []CommandDetail
	Status  Status
	ExitErr error
	Output  []string // mutated only in root Update

	proc   *os.Process
	lines  chan string
	errc   chan error
	killed bool
}

type CommandDetail struct {
	Label string
	Cmd   string
}

// Append adds a line, capping total to maxLines. Called only from root Update.
func (j *Job) Append(line string) {
	j.Output = append(j.Output, line)
	if len(j.Output) > maxLines {
		j.Output = j.Output[len(j.Output)-maxLines:]
	}
}

// Messages.
type OutputMsg struct {
	ID   int
	Line string
}
type DoneMsg struct {
	ID  int
	Err error
}
type TTYDoneMsg struct{ Err error } // result of a full-TTY (ExecProcess) run

type Manager struct {
	seq   int
	jobs  map[int]*Job
	order []int
}

func NewManager() *Manager { return &Manager{jobs: map[int]*Job{}} }

func (m *Manager) Get(id int) *Job { return m.jobs[id] }

func (m *Manager) Jobs() []*Job {
	out := make([]*Job, 0, len(m.order))
	for _, id := range m.order {
		out = append(out, m.jobs[id])
	}
	return out
}

func (m *Manager) RunningCount() int {
	n := 0
	for _, j := range m.jobs {
		if j.Status == Running {
			n++
		}
	}
	return n
}

// Start launches `line` in `dir` (via sh -c) and returns the job plus the first
// Wait cmd. stderr is merged into stdout. The child runs in its own process
// group so Kill can take down child processes (npm→node, mix→beam).
func (m *Manager) Start(label, line, dir string) (*Job, tea.Cmd) {
	return m.StartWithDetails(label, line, dir, nil)
}

func (m *Manager) StartWithDetails(label, line, dir string, details []CommandDetail) (*Job, tea.Cmd) {
	m.seq++
	job := &Job{
		ID: m.seq, Label: label, Line: line, Dir: dir, Details: details, Status: Running,
		lines: make(chan string, 512), errc: make(chan error, 1),
	}
	m.jobs[job.ID] = job
	m.order = append(m.order, job.ID)

	c := exec.Command("sh", "-c", line)
	c.Dir = dir
	setpgid(c) // platform-specific (see kill_unix.go / kill_other.go)

	pr, pw := io.Pipe()
	c.Stdout = pw
	c.Stderr = pw

	if err := c.Start(); err != nil {
		job.errc <- err
		close(job.lines)
		return job, m.Wait(job.ID)
	}
	job.proc = c.Process

	// Reader: scan merged output line by line until EOF, then close lines.
	go func() {
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024) // allow long lines
		for sc.Scan() {
			job.lines <- sc.Text()
		}
		close(job.lines)
	}()
	// Waiter: wait for exit, then close the write end so the reader sees EOF.
	go func() {
		err := c.Wait()
		_ = pw.Close()
		job.errc <- err
	}()

	return job, m.Wait(job.ID)
}

// Wait reads one line (or detects completion) and returns the matching msg.
// The root re-issues Wait after each OutputMsg to keep pumping, even when the
// runner screen is not visible.
func (m *Manager) Wait(id int) tea.Cmd {
	job := m.jobs[id]
	if job == nil {
		return nil
	}
	return func() tea.Msg {
		line, ok := <-job.lines
		if !ok {
			return DoneMsg{ID: id, Err: <-job.errc}
		}
		return OutputMsg{ID: id, Line: line}
	}
}

// Finish sets the terminal status. Called from root on DoneMsg.
func (m *Manager) Finish(id int, err error) {
	job := m.jobs[id]
	if job == nil {
		return
	}
	switch {
	case job.killed:
		job.Status = Killed
	case err != nil:
		job.Status = Failed
	default:
		job.Status = Exited
	}
	job.ExitErr = err
}

// Kill signals the job's process group. The pending Wait will then see EOF and
// produce a DoneMsg (which Finish marks as Killed).
func (m *Manager) Kill(id int) {
	job := m.jobs[id]
	if job == nil || job.proc == nil || job.Status != Running {
		return
	}
	job.killed = true
	killGroup(job.proc)
}
