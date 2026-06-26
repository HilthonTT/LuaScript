package vm

// Module loading — `require`, `loadfile`, `dofile`, `load`, and the
// `package` table. The architecture is a hybrid: structurally we follow
// Goby's pattern (a VM-resolved root path read from an env var, modules
// loaded by reading a file → `compiler.CompileToInstructions` → run), but
// the surface is Lua's: `package.path` template with `?` substitution,
// dotted module names, return-value-bearing chunks, and a `package.loaded`
// cache so each module runs at most once.
//
// Path resolution:
//   - cwd-relative (`./?.lsc`, `./?.lua`, …)
//   - cwd-relative inside `./src/`
//   - $LUASCRIPT_LIB (the bundled-library root, Goby's `libPath` analogue);
//     omitted from the default path entirely when the env var is unset.
//
// Both `.lsc` and `.lua` are searched at every path entry, with
// `.lsc` preferred so project-local `.lsc` files can shadow vanilla
// Lua libs of the same name.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hilthontt/luascript/compiler"
	"github.com/hilthontt/luascript/compiler/parser"
)

// pathTemplate joins multiple search rules separated by `;`. `?` is
// substituted with the resolved module name. Within a single rule we list
// .lsc before .lua so .lsc wins.
// The `./lua_modules/...` entries are the install root used by the package
// manager (`luascript pkg`). They are searched after project-local files so a
// local `./foo.lsc` still shadows an installed package of the same name.
const baseSearchPath = "./?.lsc;./?.lua;./?/init.lsc;./?/init.lua;./src/?.lsc;./src/?.lua;./src/?/init.lsc;./src/?/init.lua;./lua_modules/?.lsc;./lua_modules/?.lua;./lua_modules/?/init.lsc;./lua_modules/?/init.lua"

// registerLoader installs the loader globals + the `package` table. Called
// from VM.New(). Reads $LUASCRIPT_LIB once at startup; any later changes to
// the env var are ignored (matches Goby's behaviour).
func registerLoader(v *VM) {
	pkg := NewTable(0, 8)

	path := baseSearchPath
	if libRoot := os.Getenv("LUASCRIPT_LIB"); libRoot != "" {
		// Normalize trailing slash to keep template assembly simple.
		libRoot = strings.TrimRight(libRoot, "/\\")
		path += ";" +
			libRoot + "/?.lsc;" +
			libRoot + "/?.lua;" +
			libRoot + "/?/init.lsc;" +
			libRoot + "/?/init.lua"
	}
	pkg.Set("path", path)

	// Lua's `package.config` reports platform-specific separators. We use
	// Lua's defaults (forward-slash dirsep, semicolon path-sep, `?` template
	// placeholder) so existing Lua code that consults config keeps working.
	pkg.Set("config", "/\n;\n?\n!\n-\n")

	pkg.Set("loaded", NewTable(0, 16))
	pkg.Set("preload", NewTable(0, 0))
	pkg.Set("searchpath", &GoFunc{Name: "package.searchpath", Fn: builtinSearchpath})

	v.Globals.Set("package", pkg)
	v.Globals.Set("require", &GoFunc{Name: "require", Fn: builtinRequire})
	v.Globals.Set("loadfile", &GoFunc{Name: "loadfile", Fn: builtinLoadfile})
	v.Globals.Set("dofile", &GoFunc{Name: "dofile", Fn: builtinDofile})
	v.Globals.Set("load", &GoFunc{Name: "load", Fn: builtinLoad})
}

// AddScriptDir prepends search rules rooted at `dir` to package.path so
// `require` can find modules sitting next to the script being run, no
// matter what the process's current working directory is. The CLI calls
// this once it knows the main script's location; the REPL (no script) and
// the bundled .exe (script lives in the binary, not on disk) never do.
//
// Entries are prepended so a module next to the script shadows a
// same-named one in the cwd — "modules near me win", the least-surprising
// default. A no-op if the loader/package table is missing or dir is empty.
func (v *VM) AddScriptDir(dir string) {
	if dir == "" {
		return
	}
	pkg, ok := v.Globals.Get("package").(*Table)
	if !ok {
		return
	}
	// Forward slashes to match baseSearchPath's template style and Lua's
	// package.config dirsep; os.Stat accepts either on Windows anyway.
	dir = strings.TrimRight(filepath.ToSlash(dir), "/")
	prefix := dir + "/?.lsc;" +
		dir + "/?.lua;" +
		dir + "/?/init.lsc;" +
		dir + "/?/init.lua"
	if cur, _ := pkg.Get("path").(string); cur != "" {
		pkg.Set("path", prefix+";"+cur)
		return
	}
	pkg.Set("path", prefix)
}

func builtinRequire(v *VM, args []Value) []Value {
	name := StringArg("require", 1, args)

	pkg, ok := v.Globals.Get("package").(*Table)
	if !ok {
		panic(LuaError("'package' table missing — was the loader registered?"))
	}
	loaded, _ := pkg.Get("loaded").(*Table)
	if loaded == nil {
		// Repair if user code clobbered it.
		loaded = NewTable(0, 16)
		pkg.Set("loaded", loaded)
	}

	// 1. Cache hit — return without re-running.
	if cached := loaded.Get(name); cached != nil {
		return []Value{cached}
	}

	// 2. preload[name] — a function the host registered before requireing.
	if preload, ok := pkg.Get("preload").(*Table); ok {
		if loader := preload.Get(name); loader != nil {
			results := v.CallValue(loader, []Value{name}, 1)
			ret := pickRet(results)
			loaded.Set(name, ret)
			return []Value{ret}
		}
	}

	// 3. File search via package.path template.
	pathStr, _ := pkg.Get("path").(string)
	fpath, tried := searchpath(name, pathStr, ".", "/")
	if fpath == "" {
		panic(LuaError(fmt.Sprintf("module '%s' not found:%s", name, tried)))
	}

	// 4. Read, compile, run. The chunk receives (modname, filepath) as `...`
	// — Lua's standard convention.
	src, err := os.ReadFile(fpath)
	if err != nil {
		panic(Errorf("cannot open '%s': %s", fpath, err.Error()))
	}
	chunks, cerr := compiler.CompileToInstructions(string(src), parser.NormalMode)
	if cerr != nil {
		panic(Errorf("error loading module '%s' from file '%s':\n\t%s", name, fpath, cerr.Error()))
	}
	cl := &Closure{Proto: chunks[0]}
	results := v.CallValue(cl, []Value{name, fpath}, 1)
	ret := pickRet(results)
	loaded.Set(name, ret)
	return []Value{ret}
}

// pickRet implements Lua's "module return value" rule: if the chunk
// returned a non-nil value, use that; otherwise the cache stores `true` so
// re-requires don't re-run the file.
func pickRet(results []Value) Value {
	if len(results) > 0 && results[0] != nil {
		return results[0]
	}
	return true
}

// searchpath substitutes the (sep→rep)-rewritten module name into each
// `;`-separated template in `path`, and returns the first existing file.
// On miss it returns ("", "<\n\tno file 'X'>...") matching Lua's spec.
func searchpath(name, path, sep, rep string) (string, string) {
	modPath := name
	if sep != "" && rep != "" {
		modPath = strings.ReplaceAll(modPath, sep, rep)
	}
	var tried strings.Builder
	for tmpl := range strings.SplitSeq(path, ";") {
		if tmpl == "" {
			continue
		}
		full := strings.ReplaceAll(tmpl, "?", modPath)
		if _, err := os.Stat(full); err == nil {
			return full, ""
		}
		tried.WriteString("\n\tno file '")
		tried.WriteString(full)
		tried.WriteString("'")
	}
	return "", tried.String()
}

func builtinSearchpath(_ *VM, args []Value) []Value {
	name := StringArg("searchpath", 1, args)
	path := StringArg("searchpath", 2, args)
	sep := OptString("searchpath", 3, args, ".")
	rep := OptString("searchpath", 4, args, "/")
	fpath, tried := searchpath(name, path, sep, rep)
	if fpath == "" {
		return []Value{nil, tried}
	}
	return []Value{fpath}
}

// builtinLoadfile compiles a file into a callable closure WITHOUT running
// it. Returns (closure) on success, (nil, errmsg) on failure — matches
// Lua's two-value error convention.
func builtinLoadfile(_ *VM, args []Value) []Value {
	if len(args) < 1 {
		return []Value{nil, "loadfile: filename expected"}
	}
	fname, ok := args[0].(string)
	if !ok {
		return []Value{nil, fmt.Sprintf("loadfile: string filename expected, got %s", TypeName(args[0]))}
	}
	src, err := os.ReadFile(fname)
	if err != nil {
		return []Value{nil, err.Error()}
	}
	chunks, cerr := compiler.CompileToInstructions(string(src), parser.NormalMode)
	if cerr != nil {
		return []Value{nil, cerr.Error()}
	}
	return []Value{&Closure{Proto: chunks[0]}}
}

// builtinDofile loads + runs a file, returning everything the chunk
// returned. Errors propagate as panics (Lua: `error()`).
func builtinDofile(v *VM, args []Value) []Value {
	res := builtinLoadfile(v, args)
	if res[0] == nil {
		msg := "dofile: load failed"
		if len(res) >= 2 {
			msg = ToString(res[1])
		}
		panic(LuaError(msg))
	}
	return v.CallValue(res[0], nil, -1)
}

// builtinLoad compiles a string chunk. Lua's `load` also accepts a function
// that yields successive chunk pieces; we only support the string form for
// now (the function form is rarely used in practice and would require
// re-entrant compilation).
func builtinLoad(_ *VM, args []Value) []Value {
	if len(args) < 1 {
		return []Value{nil, "load: chunk expected"}
	}
	src, ok := args[0].(string)
	if !ok {
		return []Value{nil, "load: only string chunks are supported in this VM"}
	}
	chunks, cerr := compiler.CompileToInstructions(src, parser.NormalMode)
	if cerr != nil {
		return []Value{nil, cerr.Error()}
	}
	return []Value{&Closure{Proto: chunks[0]}}
}
