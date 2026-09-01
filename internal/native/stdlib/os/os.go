package os

import (
	"io"
	osStd "os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/hilthontt/luascript/internal/vm"
)

var processStart = time.Now()

func RegisterOSPreload(v *vm.VM) {
	vm.RegisterPreload(v, "os", osLoader)
}

func osLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	mod := newOS()
	mod.Set("VERSION", "0.1.0")

	mod.Set("platform", runtime.GOOS)
	mod.Set("arch", runtime.GOARCH)

	mod.Set("o_rdonly", int64(osStd.O_RDONLY))
	mod.Set("o_wronly", int64(osStd.O_WRONLY))
	mod.Set("o_rdwr", int64(osStd.O_RDWR))
	mod.Set("o_append", int64(osStd.O_APPEND))
	mod.Set("o_create", int64(osStd.O_CREATE))
	mod.Set("o_excl", int64(osStd.O_EXCL))
	mod.Set("o_sync", int64(osStd.O_SYNC))
	mod.Set("o_trunc", int64(osStd.O_TRUNC))

	mod.Set("mode_dir", int64(osStd.ModeDir))
	mod.Set("mode_append", int64(osStd.ModeAppend))
	mod.Set("mode_exclusive", int64(osStd.ModeExclusive))
	mod.Set("mode_temporary", int64(osStd.ModeTemporary))
	mod.Set("mode_symlink", int64(osStd.ModeSymlink))
	mod.Set("mode_device", int64(osStd.ModeDevice))
	mod.Set("mode_named_pipe", int64(osStd.ModeNamedPipe))
	mod.Set("mode_socket", int64(osStd.ModeSocket))
	mod.Set("mode_setuid", int64(osStd.ModeSetuid))
	mod.Set("mode_setgid", int64(osStd.ModeSetgid))
	mod.Set("mode_char_device", int64(osStd.ModeCharDevice))
	mod.Set("mode_sticky", int64(osStd.ModeSticky))
	mod.Set("mode_type", int64(osStd.ModeType))
	mod.Set("mode_perm", int64(osStd.ModePerm))

	mod.Set("path_separator", string(osStd.PathSeparator))
	mod.Set("path_list_separator", string(osStd.PathListSeparator))
	mod.Set("dev_null", osStd.DevNull)

	mod.Set("seek_set", int64(io.SeekStart))
	mod.Set("seek_cur", int64(io.SeekCurrent))
	mod.Set("seek_end", int64(io.SeekEnd))

	return []vm.Value{mod}
}

func newOS() *vm.Table {
	o := vm.NewTable(0, 31)
	methods := vm.NewTable(0, 8)

	methods.Set("pwd", &vm.GoFunc{Name: "os:pwd", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		pwd, err := osStd.Getwd()
		if err != nil {
			panic(vm.Errorf("os:pwd: %s", err.Error()))
		}
		return []vm.Value{pwd}
	}})

	methods.Set("exit", &vm.GoFunc{Name: "os:exit", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		exitCode := vm.IntArg("os:exit", 1, args)
		osStd.Exit(int(exitCode))
		return []vm.Value{}
	}})

	methods.Set("create", &vm.GoFunc{Name: "os:create", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		fileName := vm.StringArg("os:create", 1, args)
		f, err := osStd.Create(fileName)
		if err != nil {
			panic(vm.Errorf("os:create: %s", err.Error()))
		}
		return []vm.Value{newOSFile(f)}
	}})

	methods.Set("open", &vm.GoFunc{Name: "os:open", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		path := vm.StringArg("os:open", 1, args)
		flag := vm.IntArg("os:open", 2, args)
		perm := vm.IntArg("os:open", 3, args)
		f, err := osStd.OpenFile(path, int(flag), osStd.FileMode(perm))
		if err != nil {
			panic(vm.Errorf("os:open: %s", err.Error()))
		}
		return []vm.Value{newOSFile(f)}
	}})

	methods.Set("remove", &vm.GoFunc{Name: "os:remove", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		path := vm.StringArg("os:remove", 1, args)
		if err := osStd.Remove(path); err != nil {
			return []vm.Value{nil, err.Error()}
		}
		return []vm.Value{true}
	}})

	methods.Set("mkdir", &vm.GoFunc{Name: "os:mkdir", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		path := vm.StringArg("os:mkdir", 1, args)
		perm := vm.IntArg("os:mkdir", 2, args)
		if err := osStd.Mkdir(path, osStd.FileMode(perm)); err != nil {
			panic(vm.Errorf("os:mkdir: %s", err.Error()))
		}
		return nil
	}})

	methods.Set("getenv", &vm.GoFunc{Name: "os:getenv", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		key := vm.StringArg("os:getenv", 1, args)
		if val, ok := osStd.LookupEnv(key); ok {
			return []vm.Value{val}
		}
		return []vm.Value{nil}
	}})

	methods.Set("hostname", &vm.GoFunc{Name: "os:hostname", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		name, err := osStd.Hostname()
		if err != nil {
			panic(vm.Errorf("os:hostname: %s", err.Error()))
		}
		return []vm.Value{name}
	}})

	methods.Set("time", &vm.GoFunc{Name: "os:time", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		if len(args) == 0 || args[0] == nil {
			return []vm.Value{time.Now().Unix()}
		}
		t, ok := args[0].(*vm.Table)
		if !ok {
			panic(vm.Errorf("bad argument #1 to 'time' (table expected)"))
		}
		year := tableInt(t, "year", 1970)
		month := tableInt(t, "month", 1)
		day := tableInt(t, "day", 1)
		hour := tableInt(t, "hour", 12)
		min := tableInt(t, "min", 0)
		sec := tableInt(t, "sec", 0)
		tm := time.Date(int(year), time.Month(month), int(day),
			int(hour), int(min), int(sec), 0, time.Local)
		return []vm.Value{tm.Unix()}
	}})

	methods.Set("date", &vm.GoFunc{Name: "os:date", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		format := "%c"
		if len(args) >= 1 {
			if s, ok := args[0].(string); ok {
				format = s
			}
		}
		var t time.Time
		if len(args) >= 2 {
			ts, ok := vm.ToInteger(args[1])
			if !ok {
				panic(vm.Errorf("bad argument #2 to 'date' (number expected)"))
			}
			t = time.Unix(ts, 0)
		} else {
			t = time.Now()
		}
		if strings.HasPrefix(format, "!") {
			t = t.UTC()
			format = format[1:]
		} else {
			t = t.Local()
		}
		if format == "*t" || format == "*T" {
			out := vm.NewTable(0, 8)
			out.Set("year", int64(t.Year()))
			out.Set("month", int64(t.Month()))
			out.Set("day", int64(t.Day()))
			out.Set("hour", int64(t.Hour()))
			out.Set("min", int64(t.Minute()))
			out.Set("sec", int64(t.Second()))
			out.Set("wday", int64(int(t.Weekday())+1))
			out.Set("yday", int64(t.YearDay()))
			out.Set("isdst", false)
			return []vm.Value{out}
		}
		return []vm.Value{Strftime(format, t)}
	}})

	methods.Set("difftime", &vm.GoFunc{Name: "os:difftime", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		t2 := vm.FloatArg("difftime", 1, args)
		t1 := vm.FloatArg("difftime", 2, args)
		return []vm.Value{t2 - t1}
	}})

	methods.Set("clock", &vm.GoFunc{Name: "os:clock", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{time.Since(processStart).Seconds()}
	}})

	methods.Set("execute", &vm.GoFunc{Name: "os:execute", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		if len(args) == 0 || args[0] == nil {
			return []vm.Value{true}
		}
		cmdStr := vm.StringArg("execute", 1, args)
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.Command("cmd", "/C", cmdStr)
		} else {
			cmd = exec.Command("sh", "-c", cmdStr)
		}
		cmd.Stdin = osStd.Stdin
		cmd.Stdout = osStd.Stdout
		cmd.Stderr = osStd.Stderr
		err := cmd.Run()
		if err == nil {
			return []vm.Value{true, "exit", int64(0)}
		}
		if ee, ok := err.(*exec.ExitError); ok {
			return []vm.Value{nil, "exit", int64(ee.ExitCode())}
		}
		return []vm.Value{nil, "exit", int64(-1)}
	}})

	methods.Set("rename", &vm.GoFunc{Name: "os:rename", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		from := vm.StringArg("rename", 1, args)
		to := vm.StringArg("rename", 2, args)
		if err := osStd.Rename(from, to); err != nil {
			return []vm.Value{nil, err.Error()}
		}
		return []vm.Value{true}
	}})

	methods.Set("tmpname", &vm.GoFunc{Name: "os:tmpname", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		f, err := osStd.CreateTemp("", "lsc_tmp_*")
		if err != nil {
			panic(vm.Errorf("os:tmpname: %s", err.Error()))
		}
		name := f.Name()
		_ = f.Close()
		return []vm.Value{name}
	}})

	methods.Set("setlocale", &vm.GoFunc{Name: "os:setlocale", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		if len(args) == 0 || args[0] == nil {
			return []vm.Value{"C"}
		}
		if s, ok := args[0].(string); ok {
			return []vm.Value{s}
		}
		return []vm.Value{nil}
	}})

	methods.Set("getcwd", &vm.GoFunc{Name: "os:getcwd", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		pwd, err := osStd.Getwd()
		if err != nil {
			panic(vm.Errorf("os:getcwd: %s", err.Error()))
		}
		return []vm.Value{pwd}
	}})

	methods.Set("setenv", &vm.GoFunc{Name: "os:setenv", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		key := vm.StringArg("setenv", 1, args)
		val := vm.StringArg("setenv", 2, args)
		if err := osStd.Setenv(key, val); err != nil {
			return []vm.Value{nil, err.Error()}
		}
		return []vm.Value{true}
	}})

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	o.SetMetatable(mt)
	return o
}

func newOSFile(handle *osStd.File) *vm.Table {
	file := vm.NewTable(0, 1)
	methods := vm.NewTable(0, 9)

	methods.Set("read", &vm.GoFunc{Name: "file:read", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		n := vm.IntArg("file:read", 2, a)
		if n < 0 {
			panic(vm.Errorf("file:read: negative length %d", n))
		}
		data, err := io.ReadAll(io.LimitReader(handle, n))
		if err != nil {
			panic(vm.Errorf("file:read: %s", err.Error()))
		}
		if len(data) == 0 {
			return []vm.Value{nil}
		}
		return []vm.Value{string(data)}
	}})

	methods.Set("write", &vm.GoFunc{Name: "file:write", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		s := vm.StringArg("file:write", 2, a)
		n, err := handle.Write([]byte(s))
		if err != nil {
			panic(vm.Errorf("file:write: %s", err.Error()))
		}
		return []vm.Value{int64(n)}
	}})

	methods.Set("seek", &vm.GoFunc{Name: "file:seek", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		offset := vm.IntArg("file:seek", 2, a)
		whence := vm.IntArg("file:seek", 3, a)
		pos, err := handle.Seek(offset, int(whence))
		if err != nil {
			panic(vm.Errorf("file:seek: %s", err.Error()))
		}
		return []vm.Value{pos}
	}})

	methods.Set("name", &vm.GoFunc{Name: "file:name", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{handle.Name()}
	}})

	methods.Set("sync", &vm.GoFunc{Name: "file:sync", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		if err := handle.Sync(); err != nil {
			panic(vm.Errorf("file:sync: %s", err.Error()))
		}
		return nil
	}})

	methods.Set("stat", &vm.GoFunc{Name: "file:stat", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		info, err := handle.Stat()
		if err != nil {
			panic(vm.Errorf("file:stat: %s", err.Error()))
		}
		t := vm.NewTable(0, 5)
		t.Set("name", info.Name())
		t.Set("size", info.Size())
		t.Set("mode", int64(info.Mode()))
		t.Set("mod_time", info.ModTime().Format(time.RFC3339Nano))
		t.Set("is_dir", info.IsDir())
		return []vm.Value{t}
	}})

	methods.Set("truncate", &vm.GoFunc{Name: "file:truncate", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		size := vm.IntArg("file:truncate", 2, a)
		if err := handle.Truncate(size); err != nil {
			panic(vm.Errorf("file:truncate: %s", err.Error()))
		}
		return nil
	}})

	methods.Set("chmod", &vm.GoFunc{Name: "file:chmod", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		mode := vm.IntArg("file:chmod", 2, a)
		if err := handle.Chmod(osStd.FileMode(mode)); err != nil {
			panic(vm.Errorf("file:chmod: %s", err.Error()))
		}
		return nil
	}})

	methods.Set("close", &vm.GoFunc{Name: "file:close", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		if err := handle.Close(); err != nil {
			return []vm.Value{nil, err.Error()}
		}
		return []vm.Value{true}
	}})

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	file.SetMetatable(mt)
	return file
}
