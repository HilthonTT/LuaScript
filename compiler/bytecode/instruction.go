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
type Instruction struct {
	Opcode     uint8
	Params     []any
	line       int     // index inside its instruction set
	anchor     *anchor // if this instruction's first param is a forward-jump anchor
	sourceLine int
}

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
	is.Instructions = append(is.Instructions, i)
	is.count++
	return i
}
