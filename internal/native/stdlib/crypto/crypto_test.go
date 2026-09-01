package crypto_test

import (
	"strings"
	"testing"

	"github.com/hilthontt/luascript/internal/compiler"
	"github.com/hilthontt/luascript/internal/compiler/parser"
	nativecrypto "github.com/hilthontt/luascript/internal/native/stdlib/crypto"
	"github.com/hilthontt/luascript/internal/vm"
)

func runCrypto(t *testing.T, src string) *vm.VM {
	t.Helper()
	chunks, err := compiler.CompileToInstructions(src, parser.NormalMode)
	if err != nil {
		t.Fatalf("compile error: %v\nsource:\n%s", err, src)
	}
	v := vm.New()
	nativecrypto.RegisterCryptoPreload(v)
	if err := v.Run(chunks[0]); err != nil {
		t.Fatalf("vm error: %v\nsource:\n%s", err, src)
	}
	return v
}

func eq(t *testing.T, v *vm.VM, name string, want vm.Value) {
	t.Helper()
	if got := v.Globals.Get(name); !vm.Equal(got, want) {
		t.Errorf("%s = %v (%T), want %v (%T)", name, got, got, want, want)
	}
}

func TestPasswordHashRoundTrip(t *testing.T) {
	v := runCrypto(t, `
		local crypto = require("crypto")
		local h = crypto.password_hash("correct horse")
		encoded = h
		good = crypto.password_verify("correct horse", h)
		bad = crypto.password_verify("wrong horse", h)
	`)
	eq(t, v, "good", true)
	eq(t, v, "bad", false)

	enc, _ := v.Globals.Get("encoded").(string)
	if !strings.HasPrefix(enc, "$argon2id$v=19$") {
		t.Errorf("encoded hash = %q, want the argon2id PHC format", enc)
	}
	if n := strings.Count(enc, "$"); n != 5 {
		t.Errorf("encoded hash has %d '$' separators, want 5: %q", n, enc)
	}
}

func TestPasswordHashIsSalted(t *testing.T) {
	v := runCrypto(t, `
		local crypto = require("crypto")
		local a = crypto.password_hash("same")
		local b = crypto.password_hash("same")
		differ = a ~= b
		bothVerify = crypto.password_verify("same", a) and crypto.password_verify("same", b)
	`)
	eq(t, v, "differ", true)
	eq(t, v, "bothVerify", true)
}

func TestPasswordVerifyRejectsMalformed(t *testing.T) {
	v := runCrypto(t, `
		local crypto = require("crypto")
		empty = crypto.password_verify("x", "")
		garbage = crypto.password_verify("x", "not-a-hash")
		truncated = crypto.password_verify("x", "$argon2id$v=19$m=65536,t=3,p=4$")
		wrongalg = crypto.password_verify("x", "$bcrypt$v=19$m=1,t=1,p=1$AAAA$AAAA")
	`)
	for _, name := range []string{"empty", "garbage", "truncated", "wrongalg"} {
		eq(t, v, name, false)
	}
}

func TestPasswordHashHonoursOptions(t *testing.T) {
	v := runCrypto(t, `
		local crypto = require("crypto")
		-- Deliberately cheap so the test stays fast.
		local h = crypto.password_hash("pw", { time = 1, memory = 8192, key_length = 16 })
		encoded = h
		ok = crypto.password_verify("pw", h)
	`)
	eq(t, v, "ok", true)
	enc, _ := v.Globals.Get("encoded").(string)
	if !strings.Contains(enc, "m=8192,t=1") {
		t.Errorf("encoded = %q, want it to record m=8192,t=1", enc)
	}
}

func TestHmacByAlgorithm(t *testing.T) {
	v := runCrypto(t, `
		local crypto = require("crypto")
		viaGeneric = crypto.hmac("sha256", "key", "message")
		viaNamed = crypto.hmac_sha256("key", "message")
		sha512len = #crypto.hmac("sha512", "key", "message")
		sha1len = #crypto.hmac("sha1", "key", "message")
	`)
	generic := v.Globals.Get("viaGeneric")
	if !vm.Equal(generic, v.Globals.Get("viaNamed")) {
		t.Errorf("hmac(\"sha256\", ...) = %v, want the same as hmac_sha256", generic)
	}
	eq(t, v, "sha512len", int64(128))
	eq(t, v, "sha1len", int64(40))
}

func TestHmacRejectsUnknownAlgorithm(t *testing.T) {
	chunks, err := compiler.CompileToInstructions(`
		local crypto = require("crypto")
		crypto.hmac("sha0", "k", "m")
	`, parser.NormalMode)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	v := vm.New()
	nativecrypto.RegisterCryptoPreload(v)
	e := v.Run(chunks[0])
	if e == nil {
		t.Fatal("crypto.hmac with an unknown algorithm succeeded; want an error")
	}
	if !strings.Contains(e.Error(), "unsupported hash") {
		t.Errorf("got %q, want an unsupported-hash error", e.Error())
	}
}

func TestBase64URLRoundTrip(t *testing.T) {
	v := runCrypto(t, `
		local crypto = require("crypto")
		-- These bytes encode to '+' and '/' under the standard alphabet.
		local raw = crypto.hex_decode("fbff")
		enc = crypto.base64url_encode(raw)
		roundTrip = crypto.base64url_decode(enc) == raw
		-- Padded input must decode too: encoders disagree about padding.
		padded = crypto.base64url_decode("-_8=") == crypto.base64url_decode("-_8")
	`)
	enc, _ := v.Globals.Get("enc").(string)
	if strings.ContainsAny(enc, "+/=") {
		t.Errorf("base64url_encode = %q, want no '+', '/' or padding", enc)
	}
	eq(t, v, "roundTrip", true)
	eq(t, v, "padded", true)
}

func TestPbkdf2(t *testing.T) {
	v := runCrypto(t, `
		local crypto = require("crypto")
		local a = crypto.pbkdf2("pw", "salt", 100, 32)
		local b = crypto.pbkdf2("pw", "salt", 100, 32)
		len = #a
		deterministic = a == b
		differsBySalt = a ~= crypto.pbkdf2("pw", "pepper", 100, 32)
		differsByAlg = a ~= crypto.pbkdf2("pw", "salt", 100, 32, "sha512")
	`)
	eq(t, v, "len", int64(32))
	eq(t, v, "deterministic", true)
	eq(t, v, "differsBySalt", true)
	eq(t, v, "differsByAlg", true)
}

func TestRandomIntStaysInRange(t *testing.T) {
	v := runCrypto(t, `
		local crypto = require("crypto")
		inRange = true
		sawSeveral = {}
		for i = 1, 200 do
			local n = crypto.random_int(10)
			if n < 0 or n > 9 then inRange = false end
			sawSeveral[n] = true
		end
		local distinct = 0
		for _ in pairs(sawSeveral) do distinct = distinct + 1 end
		variety = distinct
		alwaysZero = crypto.random_int(1)
	`)
	eq(t, v, "inRange", true)
	eq(t, v, "alwaysZero", int64(0))
	if got, _ := v.Globals.Get("variety").(int64); got < 5 {
		t.Errorf("random_int(10) produced only %d distinct values over 200 draws", got)
	}
}

func TestRandomIntRejectsBadBound(t *testing.T) {
	chunks, err := compiler.CompileToInstructions(`
		local crypto = require("crypto")
		crypto.random_int(0)
	`, parser.NormalMode)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	v := vm.New()
	nativecrypto.RegisterCryptoPreload(v)
	if e := v.Run(chunks[0]); e == nil {
		t.Fatal("crypto.random_int(0) succeeded; want an error")
	}
}
