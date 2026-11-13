// Package config defines the catalog domain model and loads it from schema.yml.
// schema.yml is the single source of truth (ADR-0003).
package config

import (
	"path/filepath"
	"sort"
	"strings"
)

// Config is the root of schema.yml.
type Config struct {
	Customers  []Customer  `yaml:"customers"`
	Parameters []Parameter `yaml:"parameters"`
}

type Parameter struct {
	Theme string `yaml:"theme,omitempty"`
}

type Customer struct {
	Name     string    `yaml:"name"`
	Website  string    `yaml:"website,omitempty"`
	Projects []Project `yaml:"projects"`
}

type Project struct {
	Name         string        `yaml:"name"`
	Folder       string        `yaml:"folder,omitempty"`
	CICD         *CICD         `yaml:"cicd,omitempty"` // project-level CI/CD (ADR-0008)
	Environments []Environment `yaml:"environments"`
	Components   []Component   `yaml:"components"`
}

// CICD models a project's CI/CD: a local folder + executable pipelines, plus an
// optional GitHub Actions link for remote status (via the `gh` CLI). It is
// surfaced in the catalog as a synthetic component (see Component) so it reuses
// the runner, editor picker and repo actions with no extra screens (ADR-0008).
type CICD struct {
	Folder    string     `yaml:"folder"`
	Repo      string     `yaml:"repo,omitempty"`
	Actions   *Actions   `yaml:"actions,omitempty"`
	Pipelines []NamedCmd `yaml:"pipelines"`
}

// Actions configures GitHub Actions remote status via the `gh` CLI.
type Actions struct {
	Repo      string   `yaml:"repo"` // owner/repo, e.g. inteclatam/ibanking-cicd
	Workflows []string `yaml:"workflows,omitempty"`
}

// Component synthesizes a config.Component from the CI/CD block so existing
// machinery (runner, editor, repo) works unchanged. Pipelines become extra
// commands; if Actions is set, a leading "remote-status" pipeline runs `gh`.
func (c *CICD) Component(projectName string) Component {
	cmds := Commands{}
	if c.Actions != nil && c.Actions.Repo != "" {
		status := "gh run list --repo " + c.Actions.Repo + " --limit 15"
		for _, w := range c.Actions.Workflows {
			status += " --workflow " + w
		}
		cmds.Extra = append(cmds.Extra, NamedCmd{
			Name:        "remote-status",
			Description: "Lista las ejecuciones recientes de GitHub Actions.",
			Cmd:         status,
			Tags:        []string{"remote", "github", "actions", "status"},
		})
	}
	cmds.Extra = append(cmds.Extra, c.Pipelines...)
	return Component{
		Name:        projectName + " ci/cd",
		Description: "Pipelines y scripts de CI/CD",
		Type:        TypeService,
		Tags:        []string{"cicd"},
		Repo:        c.Repo,
		Folder:      c.Folder,
		SrcFolder:   c.Folder, // working dir for pipelines defaults here
		Commands:    cmds,
	}
}

// EnrichStackAdminHints annotates project-level CI/CD commands that call
// stack-admin.sh with the equivalent Docker Compose command when the target
// stack is also declared in one of the project's compose environments.
func EnrichStackAdminHints(c *Component, envs []Environment) {
	if c == nil {
		return
	}
	composeByStackAction := composeCommandIndex(envs)
	for i := range c.Commands.Extra {
		stack, action, ok := parseStackAdminCommand(c.Commands.Extra[i].Cmd)
		if !ok {
			continue
		}
		dockerCmd := composeByStackAction[stackActionKey(stack, action)]
		if dockerCmd == "" {
			continue
		}
		c.Commands.Extra[i].Hints = []CmdHint{
			{Label: "stack-admin", Cmd: c.Commands.Extra[i].Cmd},
			{Label: "docker", Cmd: dockerCmd},
		}
	}
}

type Environment struct {
	Name    string     `yaml:"name"`
	IDE     string     `yaml:"ide,omitempty"` // only meaningful on local
	URLs    []NamedURL `yaml:"urls,omitempty"`
	Compose *Compose   `yaml:"compose,omitempty"` // pointer: optional (ADR-0005)
}

type NamedURL struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

type Compose struct {
	Dir     string  `yaml:"dir"`
	EnvFile string  `yaml:"env-file,omitempty"`
	Stacks  []Stack `yaml:"stacks"`
}

type Stack struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description,omitempty"`
	Files       []string `yaml:"files"`
	Tags        []string `yaml:"tags,omitempty"`
}

// Component exposes an environment's compose stacks as runnable commands. It is
// intentionally synthetic: command execution stays on the same runner path used
// by ordinary components and CI/CD pipelines.
func (e Environment) Component(projectName string) (Component, bool) {
	if e.Compose == nil {
		return Component{}, false
	}
	cmds := Commands{}
	for _, s := range e.Compose.Stacks {
		cmds.Extra = append(cmds.Extra, composeStackCommands(e.Compose, s)...)
	}
	return Component{
		Name:        projectName + " " + e.Name + " compose",
		Description: "Acciones Docker Compose para los stacks del entorno " + e.Name + ".",
		Type:        TypeService,
		Layer:       "infra",
		Tags:        []string{"compose", "stack", e.Name},
		Folder:      e.Compose.Dir,
		SrcFolder:   e.Compose.Dir,
		Commands:    cmds,
	}, true
}

// ComponentType enumerates valid component kinds.
const (
	TypeUI      = "ui"
	TypeAPI     = "api"
	TypeMobile  = "mobile"
	TypeService = "service"
)

type Component struct {
	Name          string   `yaml:"name"`
	Description   string   `yaml:"description,omitempty"`
	Layer         string   `yaml:"layer,omitempty"` // frontend | backend | infra
	Type          string   `yaml:"type"`
	Tags          []string `yaml:"tags,omitempty"`
	Repo          string   `yaml:"repo,omitempty"`
	RepoPath      string   `yaml:"repo-path,omitempty"` // explicit tag: hyphen not inferable
	Folder        string   `yaml:"folder,omitempty"`
	SrcFolder     string   `yaml:"srcfolder,omitempty"`
	TestFolder    string   `yaml:"testfolder,omitempty"`
	ScriptsFolder string   `yaml:"scriptsfolder,omitempty"`
	Commands      Commands `yaml:"commands"`
}

type Commands struct {
	Run   string     `yaml:"run,omitempty"`
	Test  string     `yaml:"test,omitempty"`
	Build string     `yaml:"build,omitempty"`
	Extra []NamedCmd `yaml:"extra,omitempty"` // arbitrary extra commands
}

// NamedCmd is a user-defined command beyond run/test/build.
type NamedCmd struct {
	Name        string    `yaml:"name"`
	Description string    `yaml:"description,omitempty"`
	Cmd         string    `yaml:"cmd"`
	Dir         string    `yaml:"dir,omitempty"` // optional; relative to srcfolder, or absolute
	Tags        []string  `yaml:"tags,omitempty"`
	Hints       []CmdHint `yaml:"-"`
}

// RunCmd is a flattened, runnable command (shortcut or extra).
type RunCmd struct {
	Name        string
	Description string
	Cmd         string
	Dir         string // raw dir override from schema ("" = default)
	Tags        []string
	Hints       []CmdHint
}

type CmdHint struct {
	Label string
	Cmd   string
}

// RunnableCommands returns run/test/build (if set) followed by extras, in order.
func (c Component) RunnableCommands() []RunCmd {
	var out []RunCmd
	if c.Commands.Run != "" {
		out = append(out, RunCmd{Name: "run", Description: "Ejecuta el componente en modo desarrollo.", Cmd: c.Commands.Run, Tags: commandTags(c.Tags, "run", "dev")})
	}
	if c.Commands.Test != "" {
		out = append(out, RunCmd{Name: "test", Description: "Ejecuta las pruebas del componente.", Cmd: c.Commands.Test, Tags: commandTags(c.Tags, "test")})
	}
	if c.Commands.Build != "" {
		out = append(out, RunCmd{Name: "build", Description: "Compila o empaqueta el componente.", Cmd: c.Commands.Build, Tags: commandTags(c.Tags, "build")})
	}
	for _, e := range c.Commands.Extra {
		out = append(out, RunCmd{Name: e.Name, Description: e.Description, Cmd: e.Cmd, Dir: e.Dir, Tags: commandTags(c.Tags, e.Tags...), Hints: e.Hints})
	}
	return out
}

// WorkingDir resolves where a command runs. Default is srcfolder (user choice);
// an explicit per-command dir overrides (relative dirs resolve against srcfolder).
func (c Component) WorkingDir(rc RunCmd) string {
	if rc.Dir != "" {
		if filepath.IsAbs(rc.Dir) {
			return rc.Dir
		}
		base := c.SrcFolder
		if base == "" {
			base = c.Folder
		}
		return filepath.Join(base, rc.Dir)
	}
	if c.SrcFolder != "" {
		return c.SrcFolder
	}
	return c.Folder
}

// DiagramsDir returns the conventional diagrams folder for a project.
func (p Project) DiagramsDir() string {
	if p.Folder == "" {
		return ""
	}
	return filepath.Join(p.Folder, "docs", "diagrams")
}

// Theme returns the configured theme, defaulting to "gruvbox".
func (c *Config) Theme() string {
	for _, p := range c.Parameters {
		if p.Theme != "" {
			return p.Theme
		}
	}
	return "gruvbox"
}

// Stats summarizes the catalog (used for the load report / future header).
type Stats struct {
	Customers, Projects, Components, Environments, Stacks, Pipelines int
}

func (c *Config) Stats() Stats {
	var s Stats
	s.Customers = len(c.Customers)
	for _, cu := range c.Customers {
		s.Projects += len(cu.Projects)
		for _, p := range cu.Projects {
			s.Components += len(p.Components)
			s.Environments += len(p.Environments)
			if p.CICD != nil {
				s.Pipelines += len(p.CICD.Pipelines)
			}
			for _, e := range p.Environments {
				if e.Compose != nil {
					s.Stacks += len(e.Compose.Stacks)
				}
			}
		}
	}
	return s
}

func composeStackCommands(c *Compose, s Stack) []NamedCmd {
	actions := composeActions()
	out := make([]NamedCmd, 0, len(actions))
	for _, a := range actions {
		dockerCmd := composeCommand(c, s.Files, a.args)
		out = append(out, NamedCmd{
			Name:        s.Name + "-" + a.name,
			Description: strings.TrimSpace(s.Description + " " + a.description),
			Cmd:         dockerCmd,
			Tags:        commandTags(s.Tags, append([]string{"compose", "stack", s.Name}, a.tags...)...),
			Hints: []CmdHint{
				{Label: "stack-admin", Cmd: stackAdminCommand(s.Name, a.stackAction)},
				{Label: "docker", Cmd: dockerCmd},
			},
		})
	}
	return out
}

func composeCommandIndex(envs []Environment) map[string]string {
	out := map[string]string{}
	for _, env := range envs {
		if env.Compose == nil {
			continue
		}
		for _, stack := range env.Compose.Stacks {
			for _, action := range composeActions() {
				out[stackActionKey(stack.Name, action.stackAction)] = composeCommand(env.Compose, stack.Files, action.args)
			}
		}
	}
	return out
}

func composeActions() []struct {
	name        string
	description string
	args        string
	stackAction string
	tags        []string
} {
	return []struct {
		name        string
		description string
		args        string
		stackAction string
		tags        []string
	}{
		{"up", "Levanta el stack en segundo plano.", "up -d", "up", []string{"up", "dev"}},
		{"ps", "Lista el estado de los servicios del stack.", "ps", "ps", []string{"ps", "status"}},
		{"logs", "Muestra logs del stack.", "logs", "logs", []string{"logs", "debug"}},
		{"config-check", "Valida y renderiza la configuración compose efectiva.", "config", "config-check", []string{"config", "check"}},
		{"restart", "Reinicia servicios del stack.", "restart", "restart", []string{"restart"}},
		{"down", "Detiene el stack sin borrar volúmenes.", "down", "down", []string{"down"}},
	}
}

func parseStackAdminCommand(cmd string) (stack, action string, ok bool) {
	fields := strings.Fields(cmd)
	for i, f := range fields {
		if !strings.HasSuffix(f, "stack-admin.sh") {
			continue
		}
		if len(fields) <= i+2 {
			return "", "", false
		}
		stack = fields[i+1]
		action = fields[i+2]
		if strings.ContainsAny(stack, "&;|") || strings.ContainsAny(action, "&;|") {
			return "", "", false
		}
		return stack, action, true
	}
	return "", "", false
}

func stackActionKey(stack, action string) string {
	return stack + "\x00" + action
}

func composeCommand(c *Compose, files []string, args string) string {
	var b strings.Builder
	b.WriteString(`COMPOSE="${IBANKING_COMPOSE_CMD:-docker-compose}"; ${COMPOSE}`)
	if c.EnvFile != "" {
		b.WriteString(` --env-file "${IBANKING_ENV_FILE:-`)
		b.WriteString(shellQuotePath(c.EnvFile))
		b.WriteString(`}"`)
	}
	for _, f := range files {
		b.WriteString(` -f "`)
		b.WriteString(shellQuotePath(f))
		b.WriteString(`"`)
	}
	b.WriteString(" ")
	b.WriteString(args)
	return b.String()
}

func stackAdminCommand(stack, action string) string {
	return "./stack-admin.sh " + stack + " " + action
}

func shellQuotePath(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}

func commandTags(base []string, extra ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range append(append([]string{}, base...), extra...) {
		t = strings.TrimSpace(strings.ToLower(t))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}
