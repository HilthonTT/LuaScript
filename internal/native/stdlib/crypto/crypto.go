package crypto

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha3"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"math/big"
	"strings"

	"golang.org/x/crypto/argon2"

	"github.com/hilthontt/luascript/internal/vm"
)

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
	hashFn("sha3", func(b []byte) string {
		h := sha3.Sum256(b)
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

	methods.Set("hmac_sha256", &vm.GoFunc{Name: "crypto:hmac_sha256", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		key := vm.StringArg("crypto.hmac_sha256", 1, args)
		msg := vm.StringArg("crypto.hmac_sha256", 2, args)
		mac := hmac.New(sha256.New, []byte(key))
		mac.Write([]byte(msg))
		return []vm.Value{hex.EncodeToString(mac.Sum(nil))}
	}})

	methods.Set("hmac", &vm.GoFunc{Name: "crypto:hmac", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		alg := vm.StringArg("crypto.hmac", 1, args)
		key := vm.StringArg("crypto.hmac", 2, args)
		msg := vm.StringArg("crypto.hmac", 3, args)
		mac := hmac.New(hashByName("crypto.hmac", alg), []byte(key))
		mac.Write([]byte(msg))
		return []vm.Value{hex.EncodeToString(mac.Sum(nil))}
	}})

	methods.Set("password_hash", &vm.GoFunc{Name: "crypto:password_hash", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		password := vm.StringArg("crypto.password_hash", 1, args)
		p := defaultArgonParams()
		if len(args) >= 2 && args[1] != nil {
			p = argonParamsFrom(vm.TableArg("crypto.password_hash", 2, args), p)
		}
		salt := make([]byte, p.saltLen)
		if _, err := rand.Read(salt); err != nil {
			panic(vm.Errorf("crypto.password_hash: %s", err.Error()))
		}
		return []vm.Value{encodeArgon(p, salt,
			argon2.IDKey([]byte(password), salt, p.time, p.memory, p.threads, p.keyLen))}
	}})

	methods.Set("password_verify", &vm.GoFunc{Name: "crypto:password_verify", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		password := vm.StringArg("crypto.password_verify", 1, args)
		encoded := vm.StringArg("crypto.password_verify", 2, args)
		p, salt, want, ok := decodeArgon(encoded)
		if !ok {
			return []vm.Value{false}
		}
		got := argon2.IDKey([]byte(password), salt, p.time, p.memory, p.threads, uint32(len(want)))
		return []vm.Value{subtle.ConstantTimeCompare(got, want) == 1}
	}})

	methods.Set("pbkdf2", &vm.GoFunc{Name: "crypto:pbkdf2", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		password := vm.StringArg("crypto.pbkdf2", 1, args)
		salt := vm.StringArg("crypto.pbkdf2", 2, args)
		iter := vm.IntArg("crypto.pbkdf2", 3, args)
		keyLen := vm.IntArg("crypto.pbkdf2", 4, args)
		alg := vm.OptString("crypto.pbkdf2", 5, args, "sha256")
		if iter < 1 {
			panic(vm.Errorf("crypto.pbkdf2: iterations must be >= 1, got %d", iter))
		}
		if keyLen < 1 || keyLen > maxDerivedKeyLen {
			panic(vm.Errorf("crypto.pbkdf2: keylen must be between 1 and %d, got %d",
				maxDerivedKeyLen, keyLen))
		}
		key, err := pbkdf2.Key(hashByName("crypto.pbkdf2", alg), password, []byte(salt), int(iter), int(keyLen))
		if err != nil {
			panic(vm.Errorf("crypto.pbkdf2: %s", err.Error()))
		}
		return []vm.Value{string(key)}
	}})

	methods.Set("random_int", &vm.GoFunc{Name: "crypto:random_int", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		n := vm.IntArg("crypto.random_int", 1, args)
		if n < 1 {
			panic(vm.Errorf("crypto.random_int: bound must be >= 1, got %d", n))
		}
		v, err := rand.Int(rand.Reader, big.NewInt(n))
		if err != nil {
			panic(vm.Errorf("crypto.random_int: %s", err.Error()))
		}
		return []vm.Value{v.Int64()}
	}})

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

	methods.Set("constant_time_equal", &vm.GoFunc{Name: "crypto:constant_time_equal", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := vm.StringArg("crypto.constant_time_equal", 1, args)
		b := vm.StringArg("crypto.constant_time_equal", 2, args)
		if len(a) != len(b) {
			return []vm.Value{false}
		}
		return []vm.Value{subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1}
	}})

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
	codecFn("base64url", base64.RawURLEncoding.EncodeToString, decodeBase64URL)

	methods.Set("random_bytes", &vm.GoFunc{Name: "crypto:random_bytes", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		n := vm.IntArg("crypto.random_bytes", 1, args)
		const maxRandomBytes = 64 * 1024 * 1024
		if n < 0 || n > maxRandomBytes {
			panic(vm.Errorf("crypto.random_bytes: count must be between 0 and %d, got %d", int64(maxRandomBytes), n))
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

const maxDerivedKeyLen = 1 << 20

func hashByName(site, name string) func() hash.Hash {
	switch strings.ToLower(name) {
	case "sha256":
		return sha256.New
	case "sha512":
		return sha512.New
	case "sha1":
		return sha1.New
	case "sha384":
		return sha512.New384
	case "md5":
		return md5.New
	}
	panic(vm.Errorf("%s: unsupported hash %q (want sha256, sha384, sha512, sha1 or md5)", site, name))
}

func decodeBase64URL(s string) ([]byte, error) {
	if strings.HasSuffix(s, "=") {
		return base64.URLEncoding.DecodeString(s)
	}
	return base64.RawURLEncoding.DecodeString(s)
}

type argonParams struct {
	time    uint32
	memory  uint32
	threads uint8
	keyLen  uint32
	saltLen uint32
}

func defaultArgonParams() argonParams {
	return argonParams{time: 3, memory: 64 * 1024, threads: 4, keyLen: 32, saltLen: 16}
}

func argonParamsFrom(t *vm.Table, p argonParams) argonParams {
	getPositive := func(key string, dflt uint32) uint32 {
		n, ok := vm.ToInteger(t.Get(key))
		if !ok || n < 1 {
			return dflt
		}
		return uint32(n)
	}
	p.time = getPositive("time", p.time)
	p.memory = getPositive("memory", p.memory)
	p.keyLen = getPositive("key_length", p.keyLen)
	p.saltLen = getPositive("salt_length", p.saltLen)
	if n, ok := vm.ToInteger(t.Get("threads")); ok && n >= 1 && n <= 255 {
		p.threads = uint8(n)
	}
	return p
}

func encodeArgon(p argonParams, salt, key []byte) string {
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.memory, p.time, p.threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key))
}

func decodeArgon(s string) (p argonParams, salt, key []byte, ok bool) {
	parts := strings.Split(s, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return p, nil, nil, false
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return p, nil, nil, false
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.time, &p.threads); err != nil {
		return p, nil, nil, false
	}
	if p.memory == 0 || p.time == 0 || p.threads == 0 {
		return p, nil, nil, false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) == 0 {
		return p, nil, nil, false
	}
	key, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(key) == 0 {
		return p, nil, nil, false
	}
	return p, salt, key, true
}
