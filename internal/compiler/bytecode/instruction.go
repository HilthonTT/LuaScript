package bytecode

import (
	"fmt"
	"strings"
)

const (
	Program     = "ProgramStart"
	FunctionDef = "Function"
)

const (
	LoadNil uint8 = iota
	LoadTrue
	LoadFalse
	LoadInt
	LoadFloat
	LoadString
	LoadVararg
	Closure

	GetLocal
	SetLocal
	GetUpvalue
	SetUpvalue
	GetGlobal
	SetGlobal

	NewTable
	GetTable
	SetTable
	GetField
	SetField
	Self
	SetList

	Add
	Sub
	Mul
	Div
	FloorDiv
	Mod
	Pow
	Neg

	BitAnd
	BitOr
	BitXor
	Shl
	Shr
	BitNot

	Concat
	Len

	Eq
	NotEq
	Lt
	Le
	Gt
	Ge

	Not

	Jump
	JumpIfFalse
	JumpIfTrue
	JumpIfFalseKeep
	JumpIfTrueKeep

	Call
	Return

	ForPrep
	ForLoop

	TForCall
	TForLoop

	Pop
	Dup

	Leave

	MarkArgs

	Defer

	CloseUpvalues

	Try

	EndTry

	Throw

	MarkTBC

	CloseTBC

	InstructionCount
)

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
	MarkArgs:        "markargs",
	CloseUpvalues:   "closeupvalues",
	Defer:           "defer",
	Try:             "try",
	EndTry:          "endtry",
	Throw:           "throw",
	MarkTBC:         "marktbc",
	CloseTBC:        "closetbc",
}

type Instruction struct {
	Opcode   uint8
	A        int32
	B        int32
	BoxedAny any
	StrA     string

	cacheGen uint32
	cacheVal Value

	Params     []any
	line       int
	anchor     *anchor
	sourceLine int
}

type Value = any

func (i *Instruction) Inspect() string {
	parts := make([]string, 0, len(i.Params))
	for _, p := range i.Params {
		parts = append(parts, fmt.Sprint(p))
	}
	return fmt.Sprintf("%s: %s. source line: %d", i.ActionName(), strings.Join(parts, ", "), i.sourceLine)
}

func (i *Instruction) ActionName() string {
	return InstructionNameTable[i.Opcode]
}

func (i *Instruction) CacheGen() uint32 { return i.cacheGen }

func (i *Instruction) CacheVal() Value { return i.cacheVal }

func (i *Instruction) SetCache(gen uint32, val Value) {
	i.cacheGen = gen
	i.cacheVal = val
}

func (i *Instruction) AnchorLine() int {
	if i.anchor == nil {
		panic("AnchorLine called on instruction without an anchor")
	}
	return i.anchor.line
}

func (i *Instruction) Line() int {
	return i.line
}

func (i *Instruction) SourceLine() int {
	return i.sourceLine
}

type anchor struct {
	line int
}

type InstructionSet struct {
	name         string
	isType       string
	Instructions []*Instruction
	count        int

	NumParams int
	IsVararg  bool
	NumLocals int
	Upvalues  []UpvalueDesc
	Constants []any
	Protos    []*InstructionSet

	localsResolved bool

	hasTry bool

	source string
}

func (is *InstructionSet) LocalsResolved() bool { return is.localsResolved }

func (is *InstructionSet) MarkLocalsResolved() { is.localsResolved = true }

func (is *InstructionSet) HasTry() bool { return is.hasTry }

func (is *InstructionSet) SetHasTry(b bool) { is.hasTry = b }

type UpvalueDesc struct {
	Name    string
	InStack bool
	Index   int
}

func (is *InstructionSet) Name() string { return is.name }

const DefaultChunkName = "script"

func (is *InstructionSet) Source() string {
	if is.source == "" {
		return DefaultChunkName
	}
	if c := is.source[0]; c == '=' || c == '@' {
		return is.source[1:]
	}
	return is.source
}

func (is *InstructionSet) SetSource(name string) {
	if is == nil || is.source == name {
		return
	}
	is.source = name
	for _, p := range is.Protos {
		p.SetSource(name)
	}
}

func (is *InstructionSet) Type() string { return is.isType }

func (is *InstructionSet) define(action uint8, sourceLine int, params ...any) *Instruction {
	i := &Instruction{Opcode: action, Params: params, line: is.count, sourceLine: sourceLine}
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

func encodeParams(i *Instruction, op uint8, params []any) {
	switch op {
	case LoadInt, LoadFloat, LoadString:
		if len(params) >= 1 {
			i.BoxedAny = params[0]
		}

	case GetGlobal, SetGlobal, GetField, SetField, Self:
		if len(params) >= 1 {
			if s, ok := params[0].(string); ok {
				i.StrA = s
			}
		}

	case LoadNil, LoadVararg, Pop, Concat, Return,
		Closure, GetLocal, SetLocal, GetUpvalue, SetUpvalue, CloseUpvalues, EndTry, CloseTBC:
		if len(params) >= 1 {
			i.A = asInt32(params[0])
		}
	case MarkTBC:
		if len(params) >= 1 {
			i.A = asInt32(params[0])
		}
		if len(params) >= 2 {
			if s, ok := params[1].(string); ok {
				i.StrA = s
			}
		}
	case Jump, JumpIfFalse, JumpIfTrue, JumpIfFalseKeep, JumpIfTrueKeep, Try:
		if len(params) >= 1 {
			i.A = asAnchorOrInt32(params[0])
		}

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

	default:
	}
}

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

func asAnchorOrInt32(v any) int32 {
	if _, ok := v.(*anchor); ok {
		return 0
	}
	return asInt32(v)
}
