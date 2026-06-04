package compression

// Standard-format codecs (gzip, zlib, raw deflate). These complement the
// Huffman codebook API in compression.go for the case where users just
// want a smaller byte string — they accept and return raw byte strings
// rather than the {bits=, codes=} envelope Huffman uses.
//
// Each codec has the same shape:
//
//	compression.<fmt>_encode(s [, level]) -> string
//	compression.<fmt>_decode(s)           -> string
//
// `level` accepts the standard flate range (-2 huffman-only, -1 default,
// 0 store-only, 1..9 fastest..best). An out-of-range or non-numeric
// level raises with the usual bad-argument message; the encoders never
// silently fall back, so a typo doesn't quietly disable compression.

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"io"

	"github.com/hilthontt/luascript/vm"
)

// addStdCodecs installs gzip/zlib/deflate encode+decode pairs on the
// shared methods table. Kept separate from newCompression() so adding
// or removing standard codecs doesn't tangle with the Huffman wiring.
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
		out, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil {
			panic(vm.Errorf("compression.gzip_decode: %s", err.Error()))
		}
		return []vm.Value{string(out)}
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
		out, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil {
			panic(vm.Errorf("compression.zlib_decode: %s", err.Error()))
		}
		return []vm.Value{string(out)}
	}})

	// deflate is the raw flate stream — no gzip headers, no zlib
	// envelope. Useful when interoperating with protocols that wrap
	// the deflate bits themselves (HTTP transfer-encoding, PNG IDAT
	// before the zlib step, etc.).
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
		out, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil {
			panic(vm.Errorf("compression.deflate_decode: %s", err.Error()))
		}
		return []vm.Value{string(out)}
	}})
}

// optLevel reads an optional compression-level argument. Absent or nil
// yields the flate default; a present value must be in the documented
// flate range so a wrong level fails loudly rather than picking a
// silent fallback.
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