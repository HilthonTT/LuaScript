package plugin

import (
	"fmt"
	"regexp"
	"strings"
	"text/template"
)

// The generated file is an ordinary `package main` whose only job is to
// re-export the requested library functions as package-level *vars*, because
// that is the only shape Go's plugin.Lookup can find:
//
//	package main
//
//	import (
//		"strings"
//		_ "github.com/lib/pq"
//	)
//
//	var ToUpper = strings.ToUpper
//
//	func main() {}
//
// Lookup("ToUpper") on the compiled .so then hands back a *pointer* to that
// var (a **func(string) string), which the backend dereferences before
// calling — see symbolValue in backend_native.go.
//
// text/template, not html/template: an HTML escaper would turn the quotes
// around import paths into &#34; and the file would not compile.
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

// pkg is one import line. Prefix is the import alias: "" for the default
// name, "_" for a blank import (the database/sql driver-registration idiom),
// or any identifier to rename.
type pkg struct {
	Prefix string
	Name   string
}

// function is one re-exported symbol. Pkg is the package selector to read it
// from (the alias, or the package's own name), Name is the exported Go
// identifier, and As is the name it is published under in the plugin — which
// is what scripts pass to p:call. As defaults to Name and exists so two
// packages exporting the same symbol can coexist in one plugin.
type function struct {
	Pkg  string
	Name string
	As   string
}

type spec struct {
	Packages  []pkg
	Functions []function
}

// Go source we emit is assembled from script-supplied strings, so every one
// of them is validated against these before it reaches the template. This is
// not a security boundary — building a plugin runs the Go compiler and loads
// native code, which is arbitrary code execution by construction — but it
// turns a corrupt spec into a clear Lua error instead of a wall of syntax
// errors from the Go toolchain.
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

// externalImports returns the import paths that are not part of the Go
// standard library. The heuristic is the same one the go tool uses: a
// stdlib path's first segment never contains a dot. Only these force the
// generated module through `go mod tidy`.
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

// generateSource renders the spec into compilable Go source.
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
