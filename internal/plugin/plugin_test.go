package plugin

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/hilthontt/luascript/internal/vm"
)

// specTable builds the Lua-side spec a script would pass to plugin.generate.
func specTable(pkgs, fns []map[string]string) *vm.Table {
	entries := func(ms []map[string]string) *vm.Table {
		list := vm.NewTable(len(ms), 0)
		for i, m := range ms {
			e := vm.NewTable(0, len(m))
			for k, v := range m {
				e.Set(k, v)
			}
			list.Set(int64(i+1), e)
		}
		return list
	}
	t := vm.NewTable(0, 2)
	t.Set("packages", entries(pkgs))
	t.Set("functions", entries(fns))
	return t
}

func parseSpecErr(t *testing.T, tbl *vm.Table) (s *spec, luaErr string) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			luaErr = vm.ToStringMM(nil, r)
		}
	}()
	return parseSpec(tbl), ""
}

func TestParseSpec(t *testing.T) {
	tbl := specTable(
		[]map[string]string{
			{"name": "database/sql"},
			{"prefix": "_", "name": "github.com/lib/pq"},
		},
		[]map[string]string{
			{"pkg": "sql", "name": "Open"},
			{"pkg": "sql", "name": "Drivers", "as": "ListDrivers"},
		},
	)

	s, luaErr := parseSpecErr(t, tbl)
	if luaErr != "" {
		t.Fatalf("unexpected error: %s", luaErr)
	}

	if len(s.Packages) != 2 || s.Packages[0] != (pkg{Name: "database/sql"}) ||
		s.Packages[1] != (pkg{Prefix: "_", Name: "github.com/lib/pq"}) {
		t.Fatalf("packages: got %#v", s.Packages)
	}
	// `as` defaults to `name`, and is honoured when given.
	if len(s.Functions) != 2 ||
		s.Functions[0] != (function{Pkg: "sql", Name: "Open", As: "Open"}) ||
		s.Functions[1] != (function{Pkg: "sql", Name: "Drivers", As: "ListDrivers"}) {
		t.Fatalf("functions: got %#v", s.Functions)
	}
}

func TestParseSpecErrors(t *testing.T) {
	tests := []struct {
		name string
		tbl  *vm.Table
		want string
	}{
		{
			name: "missing packages",
			tbl: func() *vm.Table {
				t := vm.NewTable(0, 1)
				t.Set("functions", vm.NewTable(0, 0))
				return t
			}(),
			want: "`packages` must be a table",
		},
		{
			name: "missing functions",
			tbl: func() *vm.Table {
				t := vm.NewTable(0, 1)
				t.Set("packages", vm.NewTable(0, 0))
				return t
			}(),
			want: "`functions` must be a table",
		},
		{
			name: "function without pkg selector",
			tbl: specTable(
				[]map[string]string{{"name": "strings"}},
				[]map[string]string{{"name": "ToUpper"}},
			),
			want: "functions[1].pkg must be a string",
		},
		{
			// spec.validate runs on the parsed spec, so a bad symbol never
			// reaches the Go source.
			name: "unexported symbol",
			tbl: specTable(
				[]map[string]string{{"name": "strings"}},
				[]map[string]string{{"pkg": "strings", "name": "toUpper"}},
			),
			want: "unexported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, luaErr := parseSpecErr(t, tt.tbl)
			if !strings.Contains(luaErr, tt.want) {
				t.Fatalf("expected error containing %q, got %q", tt.want, luaErr)
			}
		})
	}
}

func TestPluginDirHonoursEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LUASCRIPT_PLUGIN_DIR", dir)
	if got := pluginDir(); got != dir {
		t.Fatalf("pluginDir() = %q, want %q", got, dir)
	}

	t.Setenv("LUASCRIPT_PLUGIN_DIR", "")
	got := pluginDir()
	if filepath.Base(got) != "plugins" || filepath.Base(filepath.Dir(got)) != "luascript" {
		t.Fatalf("default pluginDir() = %q, want .../luascript/plugins", got)
	}
}

// The build directory is keyed by the generated source, so an unchanged spec
// reuses its .so and a changed one cannot collide with the stale artifact.
func TestSourceHashDistinguishesSources(t *testing.T) {
	a := sourceHash("package main\nvar X = strings.ToUpper\n")
	b := sourceHash("package main\nvar X = strings.ToLower\n")
	if a == b {
		t.Fatal("different sources hashed to the same directory key")
	}
	if a != sourceHash("package main\nvar X = strings.ToUpper\n") {
		t.Fatal("sourceHash is not stable")
	}
}

// require("plugin") must succeed on every platform — including the ones that
// cannot load plugins — so a script can inspect plugin.supported and degrade.
func TestModuleLoadsEverywhere(t *testing.T) {
	res := pluginLoader(nil, nil)
	m, ok := res[0].(*vm.Table)
	if !ok {
		t.Fatalf("loader returned %T, want *vm.Table", res[0])
	}
	for _, key := range []string{"supported", "unsupported_reason", "generate", "open", "dir"} {
		if m.Get(key) == nil {
			t.Errorf("module is missing %q", key)
		}
	}
	if _, ok := m.Get("supported").(bool); !ok {
		t.Errorf("plugin.supported = %#v, want a boolean", m.Get("supported"))
	}
}
