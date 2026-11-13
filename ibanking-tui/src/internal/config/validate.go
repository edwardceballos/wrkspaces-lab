package config

import (
	"fmt"
	"os"
)

var validTypes = map[string]bool{
	TypeUI: true, TypeAPI: true, TypeMobile: true, TypeService: true,
}

var validLayers = map[string]bool{"frontend": true, "backend": true, "infra": true, "": true}

// Validate returns hard errors (startup should abort) and warnings
// (non-fatal: missing paths, empty stacks). Path checks are warnings because
// the catalog may legitimately reference repos not yet cloned locally.
func (c *Config) Validate() (errs []error, warns []string) {
	if len(c.Customers) == 0 {
		errs = append(errs, fmt.Errorf("no hay customers definidos"))
	}

	for _, cu := range c.Customers {
		if cu.Name == "" {
			errs = append(errs, fmt.Errorf("customer sin name"))
		}
		for _, p := range cu.Projects {
			base := cu.Name + "/" + p.Name
			if p.Name == "" {
				errs = append(errs, fmt.Errorf("%s: project sin name", cu.Name))
			}

			if p.CICD != nil {
				cp := base + "/cicd"
				if p.CICD.Folder == "" {
					errs = append(errs, fmt.Errorf("%s: cicd.folder vacío", cp))
				} else if !dirExists(p.CICD.Folder) {
					warns = append(warns, fmt.Sprintf("%s: cicd.folder no existe: %s", cp, p.CICD.Folder))
				}
				if p.CICD.Actions != nil && p.CICD.Actions.Repo == "" {
					errs = append(errs, fmt.Errorf("%s: cicd.actions.repo vacío", cp))
				}
				for i, pl := range p.CICD.Pipelines {
					if pl.Name == "" || pl.Cmd == "" {
						errs = append(errs, fmt.Errorf("%s: pipeline[%d] requiere name y cmd", cp, i))
					}
				}
			}

			for _, comp := range p.Components {
				cp := base + "/" + comp.Name
				switch {
				case comp.Name == "":
					errs = append(errs, fmt.Errorf("%s: component sin name", base))
				case comp.Type != "" && !validTypes[comp.Type]:
					errs = append(errs, fmt.Errorf("%s: type inválido %q (ui|api|mobile|service)", cp, comp.Type))
				case !validLayers[comp.Layer]:
					warns = append(warns, fmt.Sprintf("%s: layer desconocido %q (frontend|backend|infra)", cp, comp.Layer))
				}
				if comp.Folder != "" && !dirExists(comp.Folder) {
					warns = append(warns, fmt.Sprintf("%s: folder no existe: %s", cp, comp.Folder))
				}
			}

			for _, env := range p.Environments {
				if env.Compose == nil {
					continue
				}
				ep := fmt.Sprintf("%s/env:%s", base, env.Name)
				if env.Compose.Dir == "" {
					errs = append(errs, fmt.Errorf("%s: compose.dir vacío", ep))
				} else if !dirExists(env.Compose.Dir) {
					warns = append(warns, fmt.Sprintf("%s: compose.dir no existe: %s", ep, env.Compose.Dir))
				}
				if len(env.Compose.Stacks) == 0 {
					warns = append(warns, fmt.Sprintf("%s: compose sin stacks", ep))
				}
			}
		}
	}
	return errs, warns
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
