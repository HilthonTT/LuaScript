package crypto

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"

	"github.com/hilthontt/luascript/internal/vm"
)

// RegisterCryptoPreload installs the `crypto` module under package.preload.
func RegisterCryptoPreload(v *vm.VM) {
	vm.RegisterPreload(v, "crypto", cryptoLoader)
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

	// crypto.hmac_verify(key, msg, expected_hex) -> bool. Constant-time:
	// comparing an HMAC with == leaks how many leading characters match,
	// which lets an attacker forge MACs byte by byte.
	methods.Set("hmac_verify", &vm.GoFunc{Name: "crypto:hmac_verify", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		key := vm.StringArg("crypto.hmac_verify", 1, args)
		msg := vm.StringArg("crypto.hmac_verify", 2, args)
		expectedHex := vm.StringArg("crypto.hmac_verify", 3, args)
		expected, err := hex.DecodeString(expectedHex)
		if err != nil {
			return []vm.Value{false}
		}
		mac := hmac.New(sha256.New, []byte(key))
		mac.Write([]byte(msg))
		return []vm.Value{hmac.Equal(mac.Sum(nil), expected)}
	}})

	// crypto.constant_time_equal(a, b) -> bool. For comparing any two
	// secret-derived strings (tokens, digests) without a timing side channel.
	methods.Set("constant_time_equal", &vm.GoFunc{Name: "crypto:constant_time_equal", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := vm.StringArg("crypto.constant_time_equal", 1, args)
		b := vm.StringArg("crypto.constant_time_equal", 2, args)
		if len(a) != len(b) {
			return []vm.Value{false}
		}
		return []vm.Value{subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1}
	}})

	// codecFn wires an encode/decode pair (e.g. base64, hex). Decode is
	// allowed to fail; encoders can't, so they get the simpler signature.
	codecFn := func(family string, encode func([]byte) string, decode func(string) ([]byte, error)) {
		encName := family + "_encode"
		decName := family + "_decode"
		methods.Set(encName, &vm.GoFunc{Name: "crypto:" + encName, Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
			s := vm.StringArg("crypto."+encName, 1, args)
			return []vm.Value{encode([]byte(s))}
		}})
		methods.Set(decName, &vm.GoFunc{Name: "crypto:" + decName, Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
			s := vm.StringArg("crypto."+decName, 1, args)
			out, err := decode(s)
			if err != nil {
				panic(vm.Errorf("crypto.%s: %s", decName, err.Error()))
			}
			return []vm.Value{string(out)}
		}})
	}
	codecFn("base64", base64.StdEncoding.EncodeToString, base64.StdEncoding.DecodeString)
	codecFn("hex", hex.EncodeToString, hex.DecodeString)

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
