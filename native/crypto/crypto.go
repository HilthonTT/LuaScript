package crypto

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"

	"github.com/hilthontt/sakura-lang/vm"
)

// RegisterCryptoPreload installs the `crypto` module under package.preload.
func RegisterCryptoPreload(v *vm.VM) {
	pkg, ok := v.Globals.Get("package").(*vm.Table)
	if !ok {
		return
	}
	preload, ok := pkg.Get("preload").(*vm.Table)
	if !ok {
		preload = vm.NewTable(0, 4)
		pkg.Set("preload", preload)
	}
	preload.Set("crypto", &vm.GoFunc{Name: "preload.crypto", Fn: cryptoLoader})
}

func cryptoLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	mod := newCrypto()
	mod.Set("VERSION", "0.1.0")
	return []vm.Value{mod}
}

func newCrypto() *vm.Table {
	m := vm.NewTable(0, 2)
	methods := vm.NewTable(0, 12)

	// hashFn wires one digest function: it takes a string and returns its
	// lowercase hex digest.
	hashFn := func(name string, sum func([]byte) string) {
		methods.Set(name, &vm.GoFunc{Name: "crypto:" + name, Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
			s := vm.StringArg("crypto."+name, 1, args)
			return []vm.Value{sum([]byte(s))}
		}})
	}
	hashFn("md5", func(b []byte) string {
		h := md5.Sum(b)
		return hex.EncodeToString(h[:])
	})
	hashFn("sha1", func(b []byte) string {
		h := sha1.Sum(b)
		return hex.EncodeToString(h[:])
	})
	hashFn("sha256", func(b []byte) string {
		h := sha256.Sum256(b)
		return hex.EncodeToString(h[:])
	})
	hashFn("sha512", func(b []byte) string {
		h := sha512.Sum512(b)
		return hex.EncodeToString(h[:])
	})

	// crypto.hmac_sha256(key, msg) -> hex string.
	methods.Set("hmac_sha256", &vm.GoFunc{Name: "crypto:hmac_sha256", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		key := vm.StringArg("crypto.hmac_sha256", 1, args)
		msg := vm.StringArg("crypto.hmac_sha256", 2, args)
		mac := hmac.New(sha256.New, []byte(key))
		mac.Write([]byte(msg))
		return []vm.Value{hex.EncodeToString(mac.Sum(nil))}
	}})

	// crypto.base64_encode(s) -> standard-encoded string.
	methods.Set("base64_encode", &vm.GoFunc{Name: "crypto:base64_encode", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		s := vm.StringArg("crypto.base64_encode", 1, args)
		return []vm.Value{base64.StdEncoding.EncodeToString([]byte(s))}
	}})

	// crypto.base64_decode(s) -> raw string. Malformed input raises.
	methods.Set("base64_decode", &vm.GoFunc{Name: "crypto:base64_decode", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		s := vm.StringArg("crypto.base64_decode", 1, args)
		out, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			panic(vm.Errorf("crypto.base64_decode: %s", err.Error()))
		}
		return []vm.Value{string(out)}
	}})

	// crypto.hex_encode(s) -> lowercase hex string.
	methods.Set("hex_encode", &vm.GoFunc{Name: "crypto:hex_encode", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		s := vm.StringArg("crypto.hex_encode", 1, args)
		return []vm.Value{hex.EncodeToString([]byte(s))}
	}})

	// crypto.hex_decode(s) -> raw string. Malformed input raises.
	methods.Set("hex_decode", &vm.GoFunc{Name: "crypto:hex_decode", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		s := vm.StringArg("crypto.hex_decode", 1, args)
		out, err := hex.DecodeString(s)
		if err != nil {
			panic(vm.Errorf("crypto.hex_decode: %s", err.Error()))
		}
		return []vm.Value{string(out)}
	}})

	// crypto.random_bytes(n) -> raw string of n crypto-random bytes.
	methods.Set("random_bytes", &vm.GoFunc{Name: "crypto:random_bytes", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		n := vm.IntArg("crypto.random_bytes", 1, args)
		if n < 0 {
			panic(vm.Errorf("crypto.random_bytes: count must be non-negative, got %d", n))
		}
		buf := make([]byte, n)
		if _, err := rand.Read(buf); err != nil {
			panic(vm.Errorf("crypto.random_bytes: %s", err.Error()))
		}
		return []vm.Value{string(buf)}
	}})

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	m.SetMetatable(mt)
	return m
}
