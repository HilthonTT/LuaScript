package uuid

import (
	"crypto/rand"
	"regexp"

	"github.com/hilthontt/luascript/vm"
)

// uuidV4Pattern matches a canonical version-4 UUID, case-insensitively.
var uuidV4Pattern = regexp.MustCompile(
	`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-4[0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`,
)

// RegisterUUIDPreload installs the `uuid` module under package.preload.
func RegisterUUIDPreload(v *vm.VM) {
	vm.RegisterPreload(v, "uuid", uuidLoader)
}

func uuidLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	mod := newUUID()
	mod.Set("VERSION", "0.1.0")
	return []vm.Value{mod}
}

func newUUID() *vm.Table {
	m := vm.NewTable(0, 2)
	methods := vm.NewTable(0, 2)

	// uuid.v4() -> a new random UUID string (lowercase, 8-4-4-4-12).
	methods.Set("v4", &vm.GoFunc{Name: "uuid:v4", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		var b [16]byte
		if _, err := rand.Read(b[:]); err != nil {
			panic(vm.Errorf("uuid.v4: %s", err.Error()))
		}
		// Set the version (4) and variant (RFC 4122) bits.
		b[6] = (b[6] & 0x0f) | 0x40
		b[8] = (b[8] & 0x3f) | 0x80
		return []vm.Value{formatUUID(b)}
	}})

	// uuid.is_valid(s) -> true if s is a canonical version-4 UUID.
	methods.Set("is_valid", &vm.GoFunc{Name: "uuid:is_valid", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		s := vm.StringArg("uuid.is_valid", 1, args)
		return []vm.Value{uuidV4Pattern.MatchString(s)}
	}})

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	m.SetMetatable(mt)
	return m
}
