package plugin

import (
	"fmt"
	"regexp"
	"strings"
	"text/template"
)

const pluginTemplate = `package main

import (
{{- range .Packages}}
	{{if .Prefix}}{{.Prefix}} {{end}}"{{.Name}}"
{{- end}}
)

{{range .Functions}}var {{.As}} = {{.Pkg}}.{{.Name}}
{{end}}
func main() {}
`

var tmpl = template.Must(template.New("plugin").Parse(pluginTemplate))

type pkg struct {
	Prefix string
	Name   string
}

type function struct {
	Pkg  string
	Name string
	As   string
}

type spec struct {
	Packages  []pkg
	Functions []function
}

var (
	identRe      = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	importPathRe = regexp.MustCompile(`^[A-Za-z0-9_\-./~][A-Za-z0-9_\-./~+]*$`)
)

func (s *spec) validate() error {
	if len(s.Packages) == 0 {
		return fmt.Errorf("spec has no packages")
	}
	if len(s.Functions) == 0 {
		return fmt.Errorf("spec has no functions")
	}
	for _, p := range s.Packages {
		if !importPathRe.MatchString(p.Name) {
			return fmt.Errorf("package %q is not a valid Go import path", p.Name)
		}
		if p.Prefix != "" && p.Prefix != "_" && p.Prefix != "." && !identRe.MatchString(p.Prefix) {
			return fmt.Errorf("package %q: prefix %q is not a valid import alias", p.Name, p.Prefix)
		}
	}
	seen := make(map[string]bool, len(s.Functions))
	for _, f := range s.Functions {
		if !identRe.MatchString(f.Pkg) {
			return fmt.Errorf("function %q: pkg %q is not a valid package selector", f.Name, f.Pkg)
		}
		if !identRe.MatchString(f.Name) {
			return fmt.Errorf("function %q is not a valid Go identifier", f.Name)
		}
		if !isExported(f.Name) {
			return fmt.Errorf("function %q is unexported; only exported (capitalized) symbols can be looked up in a plugin", f.Name)
		}
		if !identRe.MatchString(f.As) || !isExported(f.As) {
			return fmt.Errorf("function %q: `as` name %q must be an exported Go identifier", f.Name, f.As)
		}
		if seen[f.As] {
			return fmt.Errorf("duplicate plugin symbol %q; use `as` to give one of them a distinct name", f.As)
		}
		seen[f.As] = true
	}
	return nil
}

func isExported(name string) bool {
	return name != "" && name[0] >= 'A' && name[0] <= 'Z'
}

func externalImports(s *spec) []string {
	var out []string
	for _, p := range s.Packages {
		first, _, _ := strings.Cut(p.Name, "/")
		if strings.Contains(first, ".") {
			out = append(out, p.Name)
		}
	}
	return out
}

func generateSource(s *spec) (string, error) {
	if err := s.validate(); err != nil {
		return "", err
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, s); err != nil {
		return "", err
	}
	return b.String(), nil
}
