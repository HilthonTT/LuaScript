package compression

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hilthontt/luascript/vm"
)

func RegisterCompressionPreload(v *vm.VM) {
	vm.RegisterPreload(v, "compression", compressionLoader)
}

func compressionLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	mod := newCompression()
	mod.Set("VERSION", "0.1.0")
	return []vm.Value{mod}
}

func newCompression() *vm.Table {
	m := vm.NewTable(0, 2)
	methods := vm.NewTable(0, 4)

	// compression.symbol_count(message) -> array of {symbol, freq} pairs,
	// sorted by ascending frequency (matches SymbolCountOrd's ordering).
	methods.Set("symbol_count", &vm.GoFunc{Name: "compression:symbol_count", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		msg := vm.StringArg("compression.symbol_count", 1, args)
		listfreq := SymbolCountOrd(msg)

		out := vm.NewTable(len(listfreq), 0)
		for i, sf := range listfreq {
			pair := vm.NewTable(2, 0)
			pair.Set(int64(1), string(sf.Symbol))
			pair.Set(int64(2), int64(sf.Freq))
			out.Set(int64(i+1), pair)
		}
		return []vm.Value{out}
	}})

	// compression.codes(message) -> array of {symbol, code} pairs.
	// Useful for inspecting the Huffman codebook without producing a bit stream.
	methods.Set("codes", &vm.GoFunc{Name: "compression:codes", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		msg := vm.StringArg("compression.codes", 1, args)
		codes, err := buildCodes(msg)
		if err != nil {
			panic(vm.Errorf("compression.codes: %s", err.Error()))
		}
		return []vm.Value{codesToTable(codes)}
	}})

	// compression.encode(message) -> { bits = "0101...", codes = {{sym, code}, ...} }
	// The returned table is opaque from.lsc's side — pass it straight to decode.
	methods.Set("encode", &vm.GoFunc{Name: "compression:encode", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		msg := vm.StringArg("compression.encode", 1, args)
		codes, err := buildCodes(msg)
		if err != nil {
			panic(vm.Errorf("compression.encode: %s", err.Error()))
		}
		bits := bitsToString(HuffEncode(codes, msg))

		out := vm.NewTable(0, 2)
		out.Set("bits", bits)
		out.Set("codes", codesToTable(codes))
		return []vm.Value{out}
	}})

	// compression.decode(encoded) -> message. Inverse of encode; accepts
	// the table encode returned (or any table with the same shape).
	methods.Set("decode", &vm.GoFunc{Name: "compression:decode", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		enc := vm.TableArg("compression.decode", 1, args)

		bits, ok := enc.Get("bits").(string)
		if !ok {
			panic(vm.Errorf("compression.decode: encoded table missing string field 'bits'"))
		}
		codesTable, ok := enc.Get("codes").(*vm.Table)
		if !ok {
			panic(vm.Errorf("compression.decode: encoded table missing table field 'codes'"))
		}

		reverse, err := tableToReverseMap(codesTable)
		if err != nil {
			panic(vm.Errorf("compression.decode: %s", err.Error()))
		}
		decoded, err := decodeBits(bits, reverse)
		if err != nil {
			panic(vm.Errorf("compression.decode: %s", err.Error()))
		}
		return []vm.Value{decoded}
	}})

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	m.SetMetatable(mt)
	return m
}

// buildCodes runs the count -> tree -> codebook pipeline. The single-symbol
// case is handled here because HuffEncoding would otherwise assign the lone
// leaf an empty bit code, which encodes to nothing and can't round-trip.
func buildCodes(msg string) (map[rune][]bool, error) {
	listfreq := SymbolCountOrd(msg)
	if len(listfreq) == 0 {
		return map[rune][]bool{}, nil
	}
	tree, err := HuffTree(listfreq)
	if err != nil {
		return nil, err
	}
	codes := make(map[rune][]bool, len(listfreq))
	if tree.symbol != -1 {
		// Only one distinct symbol — give it a one-bit code so repeated
		// occurrences still produce a recoverable bit string.
		codes[tree.symbol] = []bool{false}
		return codes, nil
	}
	HuffEncoding(tree, []bool{}, codes)
	return codes, nil
}

// decodeBits greedily walks bits left-to-right, emitting a symbol every
// time the running prefix matches a code. Huffman codes are prefix-free,
// so the greedy match is always correct; unmatched trailing bits mean
// the input is truncated or doesn't belong to this codebook.
func decodeBits(bits string, reverse map[string]rune) (string, error) {
	var out []rune
	var cur strings.Builder
	for i := 0; i < len(bits); i++ {
		c := bits[i]
		if c != '0' && c != '1' {
			return "", fmt.Errorf("bits[%d] = %q is not '0' or '1'", i, c)
		}
		cur.WriteByte(c)
		if r, ok := reverse[cur.String()]; ok {
			out = append(out, r)
			cur.Reset()
		}
	}
	if cur.Len() > 0 {
		return "", fmt.Errorf("%d trailing bit(s) did not match any code", cur.Len())
	}
	return string(out), nil
}

// bitsToString renders a []bool as a string of '0' and '1' bytes.
func bitsToString(bits []bool) string {
	buf := make([]byte, len(bits))
	for i, b := range bits {
		if b {
			buf[i] = '1'
		} else {
			buf[i] = '0'
		}
	}
	return string(buf)
}

// codesToTable serialises the codebook as an array of {symbol, code}
// pairs, sorted by symbol for reproducible output. Each pair is a
// 1-indexed sub-table so the codebook iterates cleanly with
// `for _, p in ipairs(codes)` on the.lsc side.
func codesToTable(codes map[rune][]bool) *vm.Table {
	runes := make([]rune, 0, len(codes))
	for r := range codes {
		runes = append(runes, r)
	}
	sort.Slice(runes, func(i, j int) bool {
		return runes[i] < runes[j]
	})

	t := vm.NewTable(len(runes), 0)
	for i, r := range runes {
		pair := vm.NewTable(2, 0)
		pair.Set(int64(1), string(r))
		pair.Set(int64(2), bitsToString(codes[r]))
		t.Set(int64(i+1), pair)
	}
	return t
}

// tableToReverseMap reads a codes table (array of {symbol, code} pairs)
// into a code -> symbol map ready for greedy decoding. Each symbol must
// be exactly one rune; multi-rune entries would mean the caller hand-built
// the table with something we'd never have produced.
func tableToReverseMap(t *vm.Table) (map[string]rune, error) {
	n := t.Len()
	rev := make(map[string]rune, n)
	for i := int64(1); i <= n; i++ {
		pair, ok := t.Get(i).(*vm.Table)
		if !ok {
			return nil, fmt.Errorf("codes[%d] is not a {symbol, code} pair", i)
		}
		sym, ok := pair.Get(int64(1)).(string)
		if !ok {
			return nil, fmt.Errorf("codes[%d][1] is not a string symbol", i)
		}
		code, ok := pair.Get(int64(2)).(string)
		if !ok {
			return nil, fmt.Errorf("codes[%d][2] is not a string code", i)
		}
		runes := []rune(sym)
		if len(runes) != 1 {
			return nil, fmt.Errorf("codes[%d][1] must be a single rune, got %q", i, sym)
		}
		rev[code] = runes[0]
	}
	return rev, nil
}
