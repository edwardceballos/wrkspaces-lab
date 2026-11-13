package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRepositorySchema(t *testing.T) {
	paths, err := filepath.Glob("../../schema*.yml")
	if err != nil {
		t.Fatalf("listing repository schemas: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("expected at least one repository schema")
	}
	for _, path := range paths {
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("%s should load with known fields: %v", path, err)
		}
		if stats := cfg.Stats(); stats.Customers == 0 || stats.Projects == 0 || stats.Stacks == 0 {
			t.Fatalf("%s loaded with unexpected stats: %#v", path, stats)
		}
	}
}

func TestDiscoverLocalSchemasOrdersDefaultAndProfiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"schema.frontend.yml", "schema.foo.yml", "schema.yml", "schema.backend.yml", "schema.notifications.yml", "schema.czc-pre-deploy-flow.yml"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("customers: []\n"), 0o600); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}

	schemas, err := DiscoverLocalSchemas(dir)
	if err != nil {
		t.Fatalf("discovering schemas: %v", err)
	}

	got := make([]string, 0, len(schemas))
	for _, schema := range schemas {
		got = append(got, schema.Name)
	}
	want := []string{"schema.yml", "schema.backend.yml", "schema.frontend.yml", "schema.notifications.yml", "schema.czc-pre-deploy-flow.yml", "schema.foo.yml"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected schema order: got %v want %v", got, want)
	}
}

func TestResolveOverrideRejectsMissingExplicitSchema(t *testing.T) {
	_, ok, err := ResolveOverride(filepath.Join(t.TempDir(), "missing.yml"))
	if !ok {
		t.Fatal("expected explicit override to be recognized")
	}
	if err == nil {
		t.Fatal("expected missing explicit schema to fail")
	}
}

func TestRunnableCommandsIncludeDescriptionAndTags(t *testing.T) {
	comp := Component{
		Name: "svc",
		Tags: []string{"backend", "czc"},
		Commands: Commands{
			Run: "go run .",
			Extra: []NamedCmd{{
				Name:        "smoke",
				Description: "smoke real",
				Cmd:         "./smoke.sh",
				Tags:        []string{"real", "smoke"},
			}},
		},
	}

	cmds := comp.RunnableCommands()
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(cmds))
	}
	if cmds[0].Name != "run" || !contains(cmds[0].Tags, "dev") || !contains(cmds[0].Tags, "backend") {
		t.Fatalf("run command tags not propagated: %#v", cmds[0])
	}
	if cmds[1].Description != "smoke real" || !contains(cmds[1].Tags, "smoke") || !contains(cmds[1].Tags, "czc") {
		t.Fatalf("extra command metadata not propagated: %#v", cmds[1])
	}
}

func TestEnvironmentComponentBuildsComposeStackCommands(t *testing.T) {
	env := Environment{
		Name: "local",
		Compose: &Compose{
			Dir:     "/tmp/compose",
			EnvFile: ".env.local",
			Stacks: []Stack{{
				Name:        "infra-only-ports",
				Description: "Infra con puertos",
				Files:       []string{"infra/compose-infra.yml", "compose.local.override.yml"},
				Tags:        []string{"infra", "ports"},
			}},
		},
	}

	comp, ok := env.Component("project")
	if !ok {
		t.Fatal("expected compose component")
	}
	cmds := comp.RunnableCommands()
	if len(cmds) != 6 {
		t.Fatalf("expected 6 compose commands, got %d", len(cmds))
	}
	up := cmds[0]
	if up.Name != "infra-only-ports-up" {
		t.Fatalf("unexpected first command: %s", up.Name)
	}
	for _, want := range []string{`--env-file "${IBANKING_ENV_FILE:-.env.local}"`, `-f "infra/compose-infra.yml"`, `-f "compose.local.override.yml"`, "up -d"} {
		if !strings.Contains(up.Cmd, want) {
			t.Fatalf("compose command missing %q: %s", want, up.Cmd)
		}
	}
	if len(up.Hints) != 2 {
		t.Fatalf("expected stack-admin and docker hints, got %#v", up.Hints)
	}
	if up.Hints[0].Label != "stack-admin" || up.Hints[0].Cmd != "./stack-admin.sh infra-only-ports up" {
		t.Fatalf("unexpected stack-admin hint: %#v", up.Hints[0])
	}
	if up.Hints[1].Label != "docker" || up.Hints[1].Cmd != up.Cmd {
		t.Fatalf("unexpected docker hint: %#v", up.Hints[1])
	}
	if !contains(up.Tags, "infra") || !contains(up.Tags, "ports") || !contains(up.Tags, "compose") {
		t.Fatalf("compose tags not propagated: %#v", up.Tags)
	}
}

func TestEnrichStackAdminHintsAddsDockerEquivalent(t *testing.T) {
	comp := Component{
		Commands: Commands{
			Extra: []NamedCmd{{
				Name: "real hub up",
				Cmd:  "./stack-admin.sh czc-real-hub up",
			}},
		},
	}
	envs := []Environment{{
		Compose: &Compose{
			Dir:     "/tmp/compose",
			EnvFile: ".env.local",
			Stacks: []Stack{{
				Name:  "czc-real-hub",
				Files: []string{"backend/compose.local-integrations-czc-dockerhub-stack.yml"},
			}},
		},
	}}

	EnrichStackAdminHints(&comp, envs)
	cmds := comp.RunnableCommands()
	if len(cmds) != 1 || len(cmds[0].Hints) != 2 {
		t.Fatalf("expected enriched command hints, got %#v", cmds)
	}
	if cmds[0].Hints[0].Cmd != "./stack-admin.sh czc-real-hub up" {
		t.Fatalf("unexpected stack-admin hint: %#v", cmds[0].Hints[0])
	}
	for _, want := range []string{`--env-file "${IBANKING_ENV_FILE:-.env.local}"`, `-f "backend/compose.local-integrations-czc-dockerhub-stack.yml"`, "up -d"} {
		if !strings.Contains(cmds[0].Hints[1].Cmd, want) {
			t.Fatalf("docker hint missing %q: %s", want, cmds[0].Hints[1].Cmd)
		}
	}
}

func TestCrezcamosNotificationTemplateCommands(t *testing.T) {
	templateCodes := []string{
		"NOTPTUD",
		"NOTPTUB",
		"NOTPTUC",
		"NOTPTUAA",
		"NOTPTBUR",
		"NOTPTBURC",
		"NOTPTBUPA",
		"NOTPTBUA",
		"NOTPTBUD",
	}
	stacks := []string{"czc-real", "czc-real-onboarding"}

	for _, schemaPath := range []string{"../../schema.notifications.yml", "../../windows/schema.notifications.yml"} {
		t.Run(schemaPath, func(t *testing.T) {
			cfg, err := Load(schemaPath)
			if err != nil {
				t.Fatalf("loading schema: %v", err)
			}
			cmds := allNamedCommands(cfg)

			for _, stack := range stacks {
				for _, code := range templateCodes {
					codeTag := strings.ToLower(code)
					wantName := stack + "-notifications-" + codeTag
					wantCmd := "CZC_NOTIFICATION_SMOKE_CODE=" + code + " ./stack-admin.sh " + stack + " notifications"

					var matches []NamedCmd
					for _, cmd := range cmds {
						if cmd.Name == wantName {
							matches = append(matches, cmd)
						}
					}
					if len(matches) != 1 {
						t.Fatalf("%s: expected exactly one command named %q, got %d", schemaPath, wantName, len(matches))
					}

					got := matches[0]
					if got.Cmd != wantCmd {
						t.Fatalf("%s: command %q mismatch:\ngot  %q\nwant %q", schemaPath, wantName, got.Cmd, wantCmd)
					}
					for _, tag := range []string{"notifications", "smoke", "template", codeTag} {
						if !contains(got.Tags, tag) {
							t.Fatalf("%s: command %q missing tag %q: %#v", schemaPath, wantName, tag, got.Tags)
						}
					}
					if stack == "czc-real-onboarding" && !contains(got.Tags, "onboarding") {
						t.Fatalf("%s: command %q missing onboarding tag: %#v", schemaPath, wantName, got.Tags)
					}
				}
			}
		})
	}
}

func TestCrezcamosNotificationTraceCommands(t *testing.T) {
	type traceCommand struct {
		name string
		cmd  string
		tags []string
	}

	traceCommands := []traceCommand{
		{
			name: "czc-real-notifications-smoke-trace",
			cmd:  "CZC_NOTIFICATION_SMOKE_TRACE_JSON=1 INTEGRATIONS_BENTO_REQUEST_LOGS=json ./stack-admin.sh czc-real notifications",
			tags: []string{"notifications", "smoke", "trace", "diagnostic"},
		},
		{
			name: "czc-real-onboarding-notifications-smoke-trace",
			cmd:  "CZC_NOTIFICATION_SMOKE_TRACE_JSON=1 INTEGRATIONS_BENTO_REQUEST_LOGS=json ./stack-admin.sh czc-real-onboarding notifications",
			tags: []string{"onboarding", "notifications", "smoke", "trace", "diagnostic"},
		},
	}

	for _, schemaPath := range []string{"../../schema.notifications.yml", "../../windows/schema.notifications.yml"} {
		t.Run(schemaPath, func(t *testing.T) {
			cfg, err := Load(schemaPath)
			if err != nil {
				t.Fatalf("loading schema: %v", err)
			}
			cmds := allNamedCommands(cfg)

			for _, want := range traceCommands {
				var matches []NamedCmd
				for _, cmd := range cmds {
					if cmd.Name == want.name {
						matches = append(matches, cmd)
					}
				}
				if len(matches) != 1 {
					t.Fatalf("%s: expected exactly one command named %q, got %d", schemaPath, want.name, len(matches))
				}

				got := matches[0]
				if got.Cmd != want.cmd {
					t.Fatalf("%s: command %q mismatch:\ngot  %q\nwant %q", schemaPath, want.name, got.Cmd, want.cmd)
				}
				for _, tag := range want.tags {
					if !contains(got.Tags, tag) {
						t.Fatalf("%s: command %q missing tag %q: %#v", schemaPath, want.name, tag, got.Tags)
					}
				}
			}
		})
	}
}

func TestRiderNotificationsCommands(t *testing.T) {
	type riderCommand struct {
		name string
		cmd  string
		tags []string
	}

	riderCommands := []riderCommand{
		{
			name: "rider-notifications-health",
			cmd:  "bash backend/integrations/smoke-rider-notifications.sh health",
			tags: []string{"rider", "notifications", "health", "smoke"},
		},
		{
			name: "rider-notifications-email-trace",
			cmd:  "bash backend/integrations/smoke-rider-notifications.sh email",
			tags: []string{"rider", "notifications", "email", "smoke", "trace", "diagnostic"},
		},
		{
			name: "rider-notifications-template-trace",
			cmd:  "bash backend/integrations/smoke-rider-notifications.sh template",
			tags: []string{"rider", "notifications", "template", "smoke", "trace", "diagnostic"},
		},
	}

	for _, schemaPath := range []string{"../../schema.notifications.yml", "../../windows/schema.notifications.yml"} {
		t.Run(schemaPath, func(t *testing.T) {
			cfg, err := Load(schemaPath)
			if err != nil {
				t.Fatalf("loading schema: %v", err)
			}
			cmds := allNamedCommands(cfg)

			for _, want := range riderCommands {
				var matches []NamedCmd
				for _, cmd := range cmds {
					if cmd.Name == want.name {
						matches = append(matches, cmd)
					}
				}
				if len(matches) != 1 {
					t.Fatalf("%s: expected exactly one command named %q, got %d", schemaPath, want.name, len(matches))
				}

				got := matches[0]
				if got.Cmd != want.cmd {
					t.Fatalf("%s: command %q mismatch:\ngot  %q\nwant %q", schemaPath, want.name, got.Cmd, want.cmd)
				}
				for _, tag := range want.tags {
					if !contains(got.Tags, tag) {
						t.Fatalf("%s: command %q missing tag %q: %#v", schemaPath, want.name, tag, got.Tags)
					}
				}
			}
		})
	}
}

func allNamedCommands(cfg *Config) []NamedCmd {
	var out []NamedCmd
	for _, customer := range cfg.Customers {
		for _, project := range customer.Projects {
			if project.CICD != nil {
				out = append(out, project.CICD.Pipelines...)
			}
			for _, component := range project.Components {
				out = append(out, component.Commands.Extra...)
			}
		}
	}
	return out
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
