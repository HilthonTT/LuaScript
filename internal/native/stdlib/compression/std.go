package compression

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"io"

	"github.com/hilthontt/luascript/internal/vm"
)

const maxDecodeBytes = 256 << 20

func inflate(site string, r io.Reader) (string, error) {
	out, err := io.ReadAll(io.LimitReader(r, maxDecodeBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(out)) > maxDecodeBytes {
		return "", vm.Errorf("%s: decompressed output exceeds %d bytes", site, int64(maxDecodeBytes))
	}
	return string(out), nil
}

func addStdCodecs(methods *vm.Table) {
	methods.Set("gzip_encode", &vm.GoFunc{Name: "compression:gzip_encode", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		data := vm.StringArg("compression.gzip_encode", 1, args)
		level := optLevel("compression.gzip_encode", 2, args)
		var buf bytes.Buffer
		w, err := gzip.NewWriterLevel(&buf, level)
		if err != nil {
			panic(vm.Errorf("compression.gzip_encode: %s", err.Error()))
		}
		if _, err := w.Write([]byte(data)); err != nil {
			panic(vm.Errorf("compression.gzip_encode: %s", err.Error()))
		}
		if err := w.Close(); err != nil {
			panic(vm.Errorf("compression.gzip_encode: %s", err.Error()))
		}
		return []vm.Value{buf.String()}
	}})

	methods.Set("gzip_decode", &vm.GoFunc{Name: "compression:gzip_decode", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		data := vm.StringArg("compression.gzip_decode", 1, args)
		r, err := gzip.NewReader(bytes.NewReader([]byte(data)))
		if err != nil {
			panic(vm.Errorf("compression.gzip_decode: %s", err.Error()))
		}
		out, err := inflate("compression.gzip_decode", r)
		_ = r.Close()
		if err != nil {
			panic(vm.Errorf("compression.gzip_decode: %s", err.Error()))
		}
		return []vm.Value{out}
	}})

	methods.Set("zlib_encode", &vm.GoFunc{Name: "compression:zlib_encode", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		data := vm.StringArg("compression.zlib_encode", 1, args)
		level := optLevel("compression.zlib_encode", 2, args)
		var buf bytes.Buffer
		w, err := zlib.NewWriterLevel(&buf, level)
		if err != nil {
			panic(vm.Errorf("compression.zlib_encode: %s", err.Error()))
		}
		if _, err := w.Write([]byte(data)); err != nil {
			panic(vm.Errorf("compression.zlib_encode: %s", err.Error()))
		}
		if err := w.Close(); err != nil {
			panic(vm.Errorf("compression.zlib_encode: %s", err.Error()))
		}
		return []vm.Value{buf.String()}
	}})

	methods.Set("zlib_decode", &vm.GoFunc{Name: "compression:zlib_decode", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		data := vm.StringArg("compression.zlib_decode", 1, args)
		r, err := zlib.NewReader(bytes.NewReader([]byte(data)))
		if err != nil {
			panic(vm.Errorf("compression.zlib_decode: %s", err.Error()))
		}
		out, err := inflate("compression.zlib_decode", r)
		_ = r.Close()
		if err != nil {
			panic(vm.Errorf("compression.zlib_decode: %s", err.Error()))
		}
		return []vm.Value{out}
	}})

	methods.Set("deflate_encode", &vm.GoFunc{Name: "compression:deflate_encode", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		data := vm.StringArg("compression.deflate_encode", 1, args)
		level := optLevel("compression.deflate_encode", 2, args)
		var buf bytes.Buffer
		w, err := flate.NewWriter(&buf, level)
		if err != nil {
			panic(vm.Errorf("compression.deflate_encode: %s", err.Error()))
		}
		if _, err := w.Write([]byte(data)); err != nil {
			panic(vm.Errorf("compression.deflate_encode: %s", err.Error()))
		}
		if err := w.Close(); err != nil {
			panic(vm.Errorf("compression.deflate_encode: %s", err.Error()))
		}
		return []vm.Value{buf.String()}
	}})

	methods.Set("deflate_decode", &vm.GoFunc{Name: "compression:deflate_decode", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		data := vm.StringArg("compression.deflate_decode", 1, args)
		r := flate.NewReader(bytes.NewReader([]byte(data)))
		out, err := inflate("compression.deflate_decode", r)
		_ = r.Close()
		if err != nil {
			panic(vm.Errorf("compression.deflate_decode: %s", err.Error()))
		}
		return []vm.Value{out}
	}})

	methods.Set("rle_encode", &vm.GoFunc{Name: "compression:rle_encode", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		data := vm.StringArg("compression.rle_encode", 1, args)
		return []vm.Value{string(rleEncode([]byte(data)))}
	}})

	methods.Set("rle_decode", &vm.GoFunc{Name: "compression:rle_decode", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		data := vm.StringArg("compression.rle_decode", 1, args)
		out, err := rleDecode("compression.rle_decode", []byte(data))
		if err != nil {
			panic(err)
		}
		return []vm.Value{string(out)}
	}})
}

func optLevel(site string, n int, args []vm.Value) int {
	if n < 1 || n > len(args) || args[n-1] == nil {
		return flate.DefaultCompression
	}
	lvl := vm.IntArg(site, n, args)
	if lvl < int64(flate.HuffmanOnly) || lvl > int64(flate.BestCompression) {
		panic(vm.Errorf("%s: level %d out of range (%d..%d)", site, lvl, flate.HuffmanOnly, flate.BestCompression))
	}
	return int(lvl)
}
