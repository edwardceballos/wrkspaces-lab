package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// EnvConfigPath overrides the schema location.
const EnvConfigPath = "CONTROL_CONFIG"

// SchemaFile describes a catalog candidate discovered in the current working
// directory.
type SchemaFile struct {
	Path        string
	Name        string
	Description string
}

// Resolve picks the schema path with precedence:
//
//	explicit arg  >  $CONTROL_CONFIG  >  ~/.config/control/schema.yml  >  ./schema.yml
func Resolve(explicit string) (string, error) {
	var candidates []string
	if explicit != "" {
		candidates = append(candidates, explicit)
	}
	if p := os.Getenv(EnvConfigPath); p != "" {
		candidates = append(candidates, p)
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".config", "control", "schema.yml"))
	}
	candidates = append(candidates, "schema.yml")

	for _, c := range candidates {
		if fileExists(c) {
			return c, nil
		}
	}
	return "", fmt.Errorf("no se encontró schema.yml (rutas probadas: %v)", candidates)
}

// ResolveOverride returns an explicitly configured schema path, if one exists.
// It intentionally checks only direct overrides, not fallback locations.
func ResolveOverride(explicit string) (string, bool, error) {
	if explicit != "" {
		if !fileExists(explicit) {
			return "", true, fmt.Errorf("schema explícito no existe: %s", explicit)
		}
		return explicit, true, nil
	}
	if p := os.Getenv(EnvConfigPath); p != "" {
		if !fileExists(p) {
			return "", true, fmt.Errorf("%s apunta a un schema inexistente: %s", EnvConfigPath, p)
		}
		return p, true, nil
	}
	return "", false, nil
}

// DiscoverLocalSchemas lists schema*.yml files in dir. The default schema.yml
// is always ordered first when present, followed by profile schemas and then
// any other candidates alphabetically.
func DiscoverLocalSchemas(dir string) ([]SchemaFile, error) {
	if dir == "" {
		dir = "."
	}
	matches, err := filepath.Glob(filepath.Join(dir, "schema*.yml"))
	if err != nil {
		return nil, err
	}
	out := make([]SchemaFile, 0, len(matches))
	for _, path := range matches {
		if !fileExists(path) {
			continue
		}
		name := filepath.Base(path)
		out = append(out, SchemaFile{
			Path:        path,
			Name:        name,
			Description: schemaDescription(name),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		pi, pj := schemaRank(out[i].Name), schemaRank(out[j].Name)
		if pi != pj {
			return pi < pj
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func schemaRank(name string) int {
	switch name {
	case "schema.yml":
		return 0
	case "schema.backend.yml":
		return 1
	case "schema.frontend.yml":
		return 2
	case "schema.data.yml":
		return 3
	case "schema.integrations.yml":
		return 4
	case "schema.notifications.yml":
		return 5
	case "schema.czc-pre.yml":
		return 6
	case "schema.czc-pre-deploy-flow.yml":
		return 7
	case "schema.dm.yml":
		return 9
	default:
		return 10
	}
}

func schemaDescription(name string) string {
	switch name {
	case "schema.yml":
		return "Perfil diario: navegación rápida y comandos frecuentes"
	case "schema.backend.yml":
		return "Perfil backend: infra, Postgres y APIs desde DockerHub"
	case "schema.frontend.yml":
		return "Perfil frontend: infra y frontends locales"
	case "schema.data.yml":
		return "Perfil data: bases, checks y migraciones"
	case "schema.integrations.yml":
		return "Perfil integrations: Bento, Apigee y smoke tests"
	case "schema.notifications.yml":
		return "Perfil notifications: pruebas Crezcamos por plantilla"
	case "schema.czc-pre.yml":
		return "Perfil deploy: operación manual czc/pre por IAP"
	case "schema.czc-pre-deploy-flow.yml":
		return "Perfil deploy: flujo completo czc/pre con Cloud SQL"
	case "schema.dm.yml":
		return "Catálogo completo legacy para diagnóstico profundo"
	default:
		profile := strings.TrimSuffix(strings.TrimPrefix(name, "schema."), ".yml")
		if profile != name {
			return "Perfil " + profile
		}
		return "Catálogo"
	}
}

// Load reads and strictly parses the schema at path. Unknown fields are an
// error (KnownFields), so YAML typos and orphan keys fail loudly at startup.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("leyendo %s: %w", path, err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)

	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parseando %s: %w", path, err)
	}
	return &cfg, nil
}

func fileExists(p string) bool {
	if p == "" {
		return false
	}
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
