package vm

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hilthontt/luascript/internal/compiler"
	"github.com/hilthontt/luascript/internal/compiler/bccache"
	"github.com/hilthontt/luascript/internal/compiler/parser"
)

const baseSearchPath = "./?.lsc;./?.lua;./?/init.lsc;./?/init.lua;./src/?.lsc;./src/?.lua;./src/?/init.lsc;./src/?/init.lua;./lua_modules/?.lsc;./lua_modules/?.lua;./lua_modules/?/init.lsc;./lua_modules/?/init.lua"

func registerLoader(v *VM) {
	pkg := NewTable(0, 8)

	path := baseSearchPath
	if libRoot := os.Getenv("LUASCRIPT_LIB"); libRoot != "" {
		libRoot = strings.TrimRight(libRoot, "/\\")
		path += ";" +
			libRoot + "/?.lsc;" +
			libRoot + "/?.lua;" +
			libRoot + "/?/init.lsc;" +
			libRoot + "/?/init.lua"
	}
	pkg.Set("path", path)

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

func (v *VM) AddScriptDir(dir string) {
	if dir == "" {
		return
	}
	pkg, ok := v.Globals.Get("package").(*Table)
	if !ok {
		return
	}
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
		loaded = NewTable(0, 16)
		pkg.Set("loaded", loaded)
	}

	if cached := loaded.Get(name); cached != nil {
		if cached == requireSentinel {
			panic(LuaError(fmt.Sprintf("loop or previous error loading module '%s'", name)))
		}
		return []Value{cached}
	}

	loaded.Set(name, requireSentinel)
	completed := false
	defer func() {
		if !completed && loaded.Get(name) == requireSentinel {
			loaded.Set(name, nil)
		}
	}()

	if preload, ok := pkg.Get("preload").(*Table); ok {
		if loader := preload.Get(name); loader != nil {
			results := v.CallValue(loader, []Value{name}, 1)
			ret := pickRet(results)
			loaded.Set(name, ret)
			completed = true
			return []Value{ret}
		}
	}

	pathStr, _ := pkg.Get("path").(string)
	fpath, tried := searchpath(name, pathStr, ".", "/")
	if fpath == "" {
		panic(LuaError(fmt.Sprintf("module '%s' not found:%s", name, tried)))
	}

	src, err := os.ReadFile(fpath)
	if err != nil {
		panic(Errorf("cannot open '%s': %s", fpath, err.Error()))
	}
	main, cerr := bccache.CompileCached(string(src))
	if cerr != nil {
		panic(Errorf("error loading module '%s' from file '%s':\n\t%s", name, fpath, cerr.Error()))
	}
	main.SetSource(fpath)
	cl := &Closure{Proto: main}
	results := v.CallValue(cl, []Value{name, fpath}, 1)
	ret := pickRet(results)
	loaded.Set(name, ret)
	completed = true
	return []Value{ret}
}

var requireSentinel Value = NewTable(0, 0)

func pickRet(results []Value) Value {
	if len(results) > 0 && results[0] != nil {
		return results[0]
	}
	return true
}

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
	main, cerr := bccache.CompileCached(string(src))
	if cerr != nil {
		return []Value{nil, cerr.Error()}
	}
	main.SetSource(fname)
	return []Value{&Closure{Proto: main}}
}

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

func chunkExcerpt(src string) string {
	line := src
	if i := strings.IndexAny(line, "\r\n"); i >= 0 {
		line = line[:i] + "..."
	}
	const maxExcerpt = 40
	if len(line) > maxExcerpt {
		line = line[:maxExcerpt] + "..."
	}
	return `[string "` + line + `"]`
}

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
	chunkname := chunkExcerpt(src)
	if len(args) >= 2 {
		if s, isStr := args[1].(string); isStr {
			chunkname = s
		}
	}
	chunks[0].SetSource(chunkname)
	return []Value{&Closure{Proto: chunks[0]}}
}
