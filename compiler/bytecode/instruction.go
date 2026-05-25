// Package bytecode emits a stack-based instruction stream for sakura-lang
// (Lua 5.4 syntax). The VM model is conventional stack-based: every
// expression leaves a single value on top of the stack unless it is a
// "multi-value" producer (call, method call, vararg) appearing in a
// last-position context that requests variable results.
package bytecode

import (
	"fmt"
	"strings"
)

// Instruction-set names used as the `Type` of an InstructionSet.
const (
	Program     = "ProgramStart" // top-level chunk
	FunctionDef = "Function"     // any function body, including the main chunk's
)

// Opcodes for the Lua-flavored stack VM.
const (
	// --- constants & literals ---
	LoadNil    uint8 = iota // params: count               ; pushes `count` nils
	LoadTrue                // pushes true
	LoadFalse               // pushes false
	LoadInt                 // params: int64               ; pushes integer
	LoadFloat               // params: float64             ; pushes float
	LoadString              // params: string              ; pushes string
	LoadVararg              // params: count (-1 = all)    ; expands current frame's `...`
	Closure                 // params: protoIndex          ; pushes closure built from FunctionProto[i]

	// --- variables ---
	GetLocal   // params: slot
	SetLocal   // params: slot
	GetUpvalue // params: index
	SetUpvalue // params: index
	GetGlobal  // params: name
	SetGlobal  // params: name

	// --- tables ---
	NewTable // params: arrayHint, hashHint  ; pushes new empty table
	GetTable // pops key, table              ; pushes table[key]
	SetTable // pops value, key, table       ; table[key]=value
	GetField // params: key (string)         ; pops table; pushes table[key]
	SetField // params: key (string)         ; pops value, table; table[key]=value
	Self     // params: key (string)         ; for `obj:m`. pops obj; pushes obj[key], obj
	SetList  // params: count, offset        ; bulk-fill array part of a table built below the values

	// --- arithmetic ---
	Add
	Sub
	Mul
	Div
	FloorDiv
	Mod
	Pow
	Neg // unary -

	// --- bitwise (Lua 5.3+) ---
	BitAnd
	BitOr
	BitXor
	Shl
	Shr
	BitNot // unary ~

	// --- string / length ---
	Concat // params: count                ; concatenates top `count` values
	Len    // unary #

	// --- comparison ---
	Eq
	NotEq
	Lt
	Le
	Gt
	Ge

	// --- logical ---
	Not // unary not

	// --- control flow ---
	Jump            // params: targetLine
	JumpIfFalse     // params: targetLine ; pops; jump if falsy
	JumpIfTrue      // params: targetLine ; pops; jump if truthy
	JumpIfFalseKeep // params: targetLine ; peeks; if falsy keep & jump, else pop & continue (for `and`)
	JumpIfTrueKeep  // params: targetLine ; peeks; if truthy keep & jump, else pop & continue (for `or`)

	// --- calls / returns ---
	Call   // params: nargs, nresults (-1 = all)
	Return // params: count (-1 = all from base to top)

	// --- numeric for ---
	ForPrep // params: baseSlot, targetLine ; uses 3 stack values: start, limit, step
	ForLoop // params: baseSlot, targetLine

	// --- generic for ---
	TForCall // params: baseSlot, nresults
	TForLoop // params: baseSlot, targetLine

	// --- stack utility ---
	Pop // params: count
	Dup // duplicates top of stack

	// --- frame ---
	Leave // ends instruction set (REPL friendly fallback / chunk terminator)

	InstructionCount
)

// InstructionNameTable maps each opcode to a readable mnemonic.
var InstructionNameTable = []string{
	LoadNil:         "loadnil",
	LoadTrue:        "loadtrue",
	LoadFalse:       "loadfalse",
	LoadInt:         "loadint",
	LoadFloat:       "loadfloat",
	LoadString:      "loadstring",
	LoadVararg:      "loadvararg",
	Closure:         "closure",
	GetLocal:        "getlocal",
	SetLocal:        "setlocal",
	GetUpvalue:      "getupvalue",
	SetUpvalue:      "setupvalue",
	GetGlobal:       "getglobal",
	SetGlobal:       "setglobal",
	NewTable:        "newtable",
	GetTable:        "gettable",
	SetTable:        "settable",
	GetField:        "getfield",
	SetField:        "setfield",
	Self:            "self",
	SetList:         "setlist",
	Add:             "add",
	Sub:             "sub",
	Mul:             "mul",
	Div:             "div",
	FloorDiv:        "floordiv",
	Mod:             "mod",
	Pow:             "pow",
	Neg:             "neg",
	BitAnd:          "band",
	BitOr:           "bor",
	BitXor:          "bxor",
	Shl:             "shl",
	Shr:             "shr",
	BitNot:          "bnot",
	Concat:          "concat",
	Len:             "len",
	Eq:              "eq",
	NotEq:           "neq",
	Lt:              "lt",
	Le:              "le",
	Gt:              "gt",
	Ge:              "ge",
	Not:             "not",
	Jump:            "jump",
	JumpIfFalse:     "jumpiffalse",
	JumpIfTrue:      "jumpiftrue",
	JumpIfFalseKeep: "jumpiffalsekeep",
	JumpIfTrueKeep:  "jumpiftruekeep",
	Call:            "call",
	Return:          "return",
	ForPrep:         "forprep",
	ForLoop:         "forloop",
	TForCall:        "tforcall",
	TForLoop:        "tforloop",
	Pop:             "pop",
	Dup:             "dup",
	Leave:           "leave",
}

// Instruction is one emitted opcode plus its parameters and source line.
//
// Hot-path fields A / B / StrA / BoxedAny are pre-decoded from Params at
// emission time so the VM dispatch doesn't pay a per-instruction type
// assertion + slice bounds check. The exact mapping of an opcode's
// arguments onto these fields lives in encodeParams below. Params is
// retained for the disassembler, Inspect, and the bytecode test suite —
// the VM hot loop reads only the typed fields.
//
// Inline-cache state (cacheGen / cacheVal) is owned by the VM, written
// on a GetGlobal miss and read on subsequent hits. Per-Instruction
// monomorphic caching: each call site memoises its own global.
type Instruction struct {
	Opcode   uint8
	A        int32 // primary int param (slot, count, jump target, name index, ...)
	B        int32 // secondary int param (Call nresults, NewTable hashHint, SetList offset, jump target for For*)
	BoxedAny any   // LoadInt/LoadFloat/LoadString payload — preserves the shared-box optimization (see vm dispatch comment)
	StrA     string

	cacheGen uint32 // VM GetGlobal inline cache: matches Table.gen on hit
	cacheVal Value  // cached value at last lookup

	Params     []any
	line       int     // index inside its instruction set
	anchor     *anchor // if this instruction's first param is a forward-jump anchor
	sourceLine int
}

// Value is re-declared here as `any` to avoid an import cycle with vm/.
// The cacheVal field stores a vm.Value (which is `any`) — equivalent at
// the language level, and the cache is only ever read/written by the VM.
type Value = any

// Inspect renders the instruction in a human-readable form.
func (i *Instruction) Inspect() string {
	parts := make([]string, 0, len(i.Params))
	for _, p := range i.Params {
		parts = append(parts, fmt.Sprint(p))
	}
	return fmt.Sprintf("%s: %s. source line: %d", i.ActionName(), strings.Join(parts, ", "), i.sourceLine)
}

// ActionName returns the mnemonic for this instruction.
func (i *Instruction) ActionName() string {
	return InstructionNameTable[i.Opcode]
}

// CacheGen returns the generation observed at the last successful
// inline-cache lookup against this instruction (zero before first use).
func (i *Instruction) CacheGen() uint32 { return i.cacheGen }

// CacheVal returns the value memoised at the last cache miss.
func (i *Instruction) CacheVal() Value { return i.cacheVal }

// SetCache records (gen, val) on this instruction for the next hit.
func (i *Instruction) SetCache(gen uint32, val Value) {
	i.cacheGen = gen
	i.cacheVal = val
}

// AnchorLine returns the resolved target line of this instruction's anchor.
// Panics if the instruction has no anchor.
func (i *Instruction) AnchorLine() int {
	if i.anchor == nil {
		panic("AnchorLine called on instruction without an anchor")
	}
	return i.anchor.line
}

// Line returns the instruction's index inside its instruction set.
func (i *Instruction) Line() int {
	return i.line
}

// SourceLine returns the source-code line where the instruction originated.
func (i *Instruction) SourceLine() int {
	return i.sourceLine
}

type anchor struct {
	line int
}

// InstructionSet is the body of one function (or the top-level chunk).
type InstructionSet struct {
	name         string
	isType       string
	Instructions []*Instruction
	count        int

	// Function-proto data — only populated for functions (and the main chunk).
	NumParams int
	IsVararg  bool
	NumLocals int
	Upvalues  []UpvalueDesc
	Constants []any             // reserved for future constant-pool use
	Protos    []*InstructionSet // nested function instruction sets

	// localsResolved is set true by the VM once it has reconciled NumLocals
	// against an instruction-stream scan (see vm.callClosure). It exists so
	// that the one-time scan — needed because the main chunk's NumLocals is
	// left at 0 by the generator — does not repeat on every call.
	localsResolved bool
}

// LocalsResolved reports whether the VM has already reconciled NumLocals.
func (is *InstructionSet) LocalsResolved() bool { return is.localsResolved }

// MarkLocalsResolved records that NumLocals is now authoritative.
func (is *InstructionSet) MarkLocalsResolved() { is.localsResolved = true }

// UpvalueDesc describes how a function captures one upvalue.
//
//	InStack == true  -> Index is a local slot in the immediately enclosing function
//	InStack == false -> Index is an upvalue index in the immediately enclosing function
type UpvalueDesc struct {
	Name    string
	InStack bool
	Index   int
}

// Name returns the instruction set's name.
func (is *InstructionSet) Name() string { return is.name }

// Type returns the instruction set's category (Program / FunctionDef).
func (is *InstructionSet) Type() string { return is.isType }

func (is *InstructionSet) define(action uint8, sourceLine int, params ...any) *Instruction {
	i := &Instruction{Opcode: action, Params: params, line: is.count, sourceLine: sourceLine + 1}
	for _, p := range params {
		if a, ok := p.(*anchor); ok {
			i.anchor = a
			break
		}
	}
	encodeParams(i, action, params)
	is.Instructions = append(is.Instructions, i)
	is.count++
	return i
}

// encodeParams populates the typed fast-path fields (A, B, StrA, BoxedAny)
// from the variadic Params slice based on the opcode's parameter layout.
// Anchor params are left as zero in A/B at this stage — the generator's
// post-emit resolution pass writes the resolved target line back to A/B
// once forward-jump anchors are known (see generator.go).
//
// The encoding contract per opcode group is:
//
//	A only (int param):
//	    LoadNil, LoadVararg, Pop, Concat, Return,
//	    Closure, GetLocal, SetLocal, GetUpvalue, SetUpvalue,
//	    Jump, JumpIfFalse, JumpIfTrue, JumpIfFalseKeep, JumpIfTrueKeep
//	A + B (two int params):
//	    NewTable (arrHint, hashHint), SetList (count, offset),
//	    Call (nargs, nresults), TForCall (baseSlot, nresults),
//	    ForPrep / ForLoop / TForLoop (baseSlot, targetLine)
//	StrA only:
//	    GetGlobal, SetGlobal, GetField, SetField, Self
//	BoxedAny only (the literal value, kept as a shared `any` box):
//	    LoadInt, LoadFloat, LoadString
//	No params:
//	    LoadTrue, LoadFalse, Leave, Dup, GetTable, SetTable,
//	    Len, Not, Neg, BitNot, Add/Sub/Mul/Div/FloorDiv/Mod/Pow,
//	    BitAnd/BitOr/BitXor/Shl/Shr, Eq/NotEq/Lt/Le/Gt/Ge
//
// asInt32 / asAnchorOrInt32 handle the int/int64 heterogeneity that the
// callers actually pass (LoadInt uses int64, slot/count params use int,
// jump targets are *anchor pre-resolution or int post-resolution).
func encodeParams(i *Instruction, op uint8, params []any) {
	switch op {
	// --- BoxedAny: keep the original box so VM push is a single field read
	case LoadInt, LoadFloat, LoadString:
		if len(params) >= 1 {
			i.BoxedAny = params[0]
		}

	// --- StrA: string-keyed name/key/method
	case GetGlobal, SetGlobal, GetField, SetField, Self:
		if len(params) >= 1 {
			if s, ok := params[0].(string); ok {
				i.StrA = s
			}
		}

	// --- A only: single int param (or *anchor for jumps, resolved later)
	case LoadNil, LoadVararg, Pop, Concat, Return,
		Closure, GetLocal, SetLocal, GetUpvalue, SetUpvalue:
		if len(params) >= 1 {
			i.A = asInt32(params[0])
		}
	case Jump, JumpIfFalse, JumpIfTrue, JumpIfFalseKeep, JumpIfTrueKeep:
		if len(params) >= 1 {
			i.A = asAnchorOrInt32(params[0])
		}

	// --- A + B: two int params; B may be an anchor for For-family
	case NewTable, SetList, Call, TForCall:
		if len(params) >= 1 {
			i.A = asInt32(params[0])
		}
		if len(params) >= 2 {
			i.B = asInt32(params[1])
		}
	case ForPrep, ForLoop, TForLoop:
		if len(params) >= 1 {
			i.A = asInt32(params[0])
		}
		if len(params) >= 2 {
			i.B = asAnchorOrInt32(params[1])
		}

	// --- No params: nothing to encode
	default:
		// LoadTrue, LoadFalse, Leave, Dup, GetTable, SetTable,
		// arithmetic / bitwise / comparison / unary ops
	}
}

// asInt32 narrows the int/int64 the callers actually pass to int32 for
// storage in A/B. Out-of-range values from int64 wrap silently — the
// instruction set is small (max ~50 opcodes, slot counts fit in int32
// trivially) so 32 bits is plenty.
func asInt32(v any) int32 {
	switch n := v.(type) {
	case int:
		return int32(n)
	case int32:
		return n
	case int64:
		return int32(n)
	}
	return 0
}

// asAnchorOrInt32 returns 0 if v is a forward-jump *anchor (the resolved
// value will be written later by the generator's pending-list walk);
// otherwise it narrows the integer param.
func asAnchorOrInt32(v any) int32 {
	if _, ok := v.(*anchor); ok {
		return 0
	}
	return asInt32(v)
}
