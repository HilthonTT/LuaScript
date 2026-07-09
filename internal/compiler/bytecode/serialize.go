package bytecode

// Binary serialization of a compiled chunk. Serialize/Deserialize round-trip
// the main InstructionSet together with its nested function protos, so a
// deserialized chunk is directly runnable by the VM (all consumers — RunFile,
// require, loadfile — execute chunks[0] and reach nested functions through
// its Protos table).
//
// The format captures exactly what the VM's dispatch loop reads: per
// instruction the opcode, the typed fast-path fields (A, B, StrA, BoxedAny)
// and the source line. The Params slice is *reconstructed* on load from
// those fields (the mapping is the inverse of encodeParams), so the
// disassembler keeps working on cached chunks. Forward-jump anchors are
// already resolved to concrete lines by the time a chunk leaves
// GenerateInstructions, so no anchor state is serialized.
//
// The format is a cache artifact, not a distribution format: it is
// invalidated wholesale by SerialVersion (layout changes) and by the opcode
// count embedded in the header (opcode renumbering), and the bccache layer
// additionally keys files by interpreter version + source hash.

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

// serialMagic identifies a serialized luascript chunk.
const serialMagic = "LSCB"

// SerialVersion is the on-disk bytecode layout version. Bump on any change
// to the encoding below.
const SerialVersion = 1

// Sanity caps against corrupt/truncated input. Generous compared to any real
// chunk, tiny compared to what a hostile length prefix could ask for.
const (
	maxSerialCount  = 1 << 24 // instructions / protos / upvalues per set
	maxSerialString = 1 << 26 // bytes in one string
)

// boxed-value tags for Instruction.BoxedAny.
const (
	boxNil    byte = 0
	boxInt    byte = 1
	boxFloat  byte = 2
	boxString byte = 3
)

// SerializeChunk writes main (and, recursively, its nested protos) to w.
func SerializeChunk(w io.Writer, main *InstructionSet) error {
	bw := bufio.NewWriter(w)
	if _, err := bw.WriteString(serialMagic); err != nil {
		return err
	}
	// Header: layout version + opcode count. The opcode count catches cache
	// files produced by a build whose opcode numbering differs even when the
	// layout version didn't change.
	if err := bw.WriteByte(SerialVersion); err != nil {
		return err
	}
	writeUvarint(bw, uint64(InstructionCount))
	if err := writeInstructionSet(bw, main); err != nil {
		return err
	}
	return bw.Flush()
}

// DeserializeChunk reads a chunk previously written by SerializeChunk.
func DeserializeChunk(r io.Reader) (*InstructionSet, error) {
	br := bufio.NewReader(r)
	magic := make([]byte, len(serialMagic))
	if _, err := io.ReadFull(br, magic); err != nil {
		return nil, fmt.Errorf("bytecode: reading magic: %w", err)
	}
	if string(magic) != serialMagic {
		return nil, errors.New("bytecode: not a serialized luascript chunk")
	}
	ver, err := br.ReadByte()
	if err != nil {
		return nil, err
	}
	if ver != SerialVersion {
		return nil, fmt.Errorf("bytecode: serialized chunk version %d, want %d", ver, SerialVersion)
	}
	opCount, err := readUvarint(br)
	if err != nil {
		return nil, err
	}
	if opCount != uint64(InstructionCount) {
		return nil, fmt.Errorf("bytecode: chunk built with %d opcodes, this build has %d", opCount, InstructionCount)
	}
	return readInstructionSet(br, 0)
}

func writeInstructionSet(w *bufio.Writer, is *InstructionSet) error {
	writeString(w, is.name)
	writeString(w, is.isType)
	writeUvarint(w, uint64(is.NumParams))
	writeBool(w, is.IsVararg)
	writeUvarint(w, uint64(is.NumLocals))

	writeUvarint(w, uint64(len(is.Upvalues)))
	for _, u := range is.Upvalues {
		writeString(w, u.Name)
		writeBool(w, u.InStack)
		writeUvarint(w, uint64(u.Index))
	}

	writeUvarint(w, uint64(len(is.Instructions)))
	for _, ins := range is.Instructions {
		if err := serializeInstruction(w, ins); err != nil {
			return err
		}
	}

	writeUvarint(w, uint64(len(is.Protos)))
	for _, p := range is.Protos {
		if err := writeInstructionSet(w, p); err != nil {
			return err
		}
	}
	// Surface any buffered write error once per set rather than checking
	// every helper call; bufio latches the first error.
	_, err := w.Write(nil)
	return err
}

// maxSerialDepth bounds proto nesting during deserialization. Legitimate
// chunks nest one level per lexically nested function — nowhere near this —
// while a crafted/corrupt stream of single-child nesting levels would
// otherwise recurse to an unrecoverable Go stack overflow instead of the
// graceful decode-error fallback the bccache contract relies on.
const maxSerialDepth = 200

func readInstructionSet(r *bufio.Reader, depth int) (*InstructionSet, error) {
	if depth > maxSerialDepth {
		return nil, fmt.Errorf("bytecode chunk exceeds max proto nesting depth %d", maxSerialDepth)
	}
	is := &InstructionSet{}
	var err error
	if is.name, err = readString(r); err != nil {
		return nil, err
	}
	if is.isType, err = readString(r); err != nil {
		return nil, err
	}
	if is.NumParams, err = readCount(r); err != nil {
		return nil, err
	}
	if is.IsVararg, err = readBool(r); err != nil {
		return nil, err
	}
	if is.NumLocals, err = readCount(r); err != nil {
		return nil, err
	}

	nUp, err := readCount(r)
	if err != nil {
		return nil, err
	}
	if nUp > 0 {
		is.Upvalues = make([]UpvalueDesc, nUp)
		for i := range is.Upvalues {
			if is.Upvalues[i].Name, err = readString(r); err != nil {
				return nil, err
			}
			if is.Upvalues[i].InStack, err = readBool(r); err != nil {
				return nil, err
			}
			if is.Upvalues[i].Index, err = readCount(r); err != nil {
				return nil, err
			}
		}
	}

	nIns, err := readCount(r)
	if err != nil {
		return nil, err
	}
	is.Instructions = make([]*Instruction, nIns)
	for i := range is.Instructions {
		ins, err := deserializeInstruction(r)
		if err != nil {
			return nil, err
		}
		ins.line = i
		is.Instructions[i] = ins
	}
	is.count = nIns

	nProto, err := readCount(r)
	if err != nil {
		return nil, err
	}
	if nProto > 0 {
		is.Protos = make([]*InstructionSet, nProto)
		for i := range is.Protos {
			if is.Protos[i], err = readInstructionSet(r, depth+1); err != nil {
				return nil, err
			}
		}
	}
	return is, nil
}

func serializeInstruction(w *bufio.Writer, ins *Instruction) error {
	w.WriteByte(ins.Opcode)
	writeUvarint(w, uint64(ins.sourceLine))
	writeVarint(w, int64(ins.A))
	writeVarint(w, int64(ins.B))
	writeString(w, ins.StrA)
	switch v := ins.BoxedAny.(type) {
	case nil:
		w.WriteByte(boxNil)
	case int64:
		w.WriteByte(boxInt)
		writeVarint(w, v)
	case float64:
		w.WriteByte(boxFloat)
		var buf [8]byte
		binary.LittleEndian.PutUint64(buf[:], math.Float64bits(v))
		w.Write(buf[:])
	case string:
		w.WriteByte(boxString)
		writeString(w, v)
	default:
		return fmt.Errorf("bytecode: unserializable boxed value %T in %s", v, ins.ActionName())
	}
	return nil
}

func deserializeInstruction(r *bufio.Reader) (*Instruction, error) {
	op, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	if op >= InstructionCount {
		return nil, fmt.Errorf("bytecode: unknown opcode %d in serialized chunk", op)
	}
	ins := &Instruction{Opcode: op}
	src, err := readUvarint(r)
	if err != nil {
		return nil, err
	}
	ins.sourceLine = int(src)
	a, err := readVarint(r)
	if err != nil {
		return nil, err
	}
	ins.A = int32(a)
	b, err := readVarint(r)
	if err != nil {
		return nil, err
	}
	ins.B = int32(b)
	if ins.StrA, err = readString(r); err != nil {
		return nil, err
	}
	tag, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	switch tag {
	case boxNil:
	case boxInt:
		v, err := readVarint(r)
		if err != nil {
			return nil, err
		}
		ins.BoxedAny = v
	case boxFloat:
		var buf [8]byte
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return nil, err
		}
		ins.BoxedAny = math.Float64frombits(binary.LittleEndian.Uint64(buf[:]))
	case boxString:
		s, err := readString(r)
		if err != nil {
			return nil, err
		}
		ins.BoxedAny = s
	default:
		return nil, fmt.Errorf("bytecode: unknown boxed-value tag %d", tag)
	}
	ins.Params = rebuildParams(ins)
	return ins, nil
}

// rebuildParams reconstructs the Params slice from the typed fast-path
// fields — the inverse of encodeParams. Only the disassembler, Inspect, and
// tests read Params; the VM dispatch loop uses the typed fields directly.
func rebuildParams(ins *Instruction) []any {
	switch ins.Opcode {
	case LoadInt, LoadFloat, LoadString:
		return []any{ins.BoxedAny}
	case GetGlobal, SetGlobal, GetField, SetField, Self:
		return []any{ins.StrA}
	case LoadNil, LoadVararg, Pop, Concat, Return,
		Closure, GetLocal, SetLocal, GetUpvalue, SetUpvalue, CloseUpvalues,
		Jump, JumpIfFalse, JumpIfTrue, JumpIfFalseKeep, JumpIfTrueKeep:
		return []any{int(ins.A)}
	case NewTable, SetList, Call, TForCall, ForPrep, ForLoop, TForLoop:
		return []any{int(ins.A), int(ins.B)}
	}
	return nil
}

// --- primitive encoders --------------------------------------------------

func writeUvarint(w *bufio.Writer, v uint64) {
	var buf [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(buf[:], v)
	w.Write(buf[:n])
}

func writeVarint(w *bufio.Writer, v int64) {
	var buf [binary.MaxVarintLen64]byte
	n := binary.PutVarint(buf[:], v)
	w.Write(buf[:n])
}

func writeString(w *bufio.Writer, s string) {
	writeUvarint(w, uint64(len(s)))
	w.WriteString(s)
}

func writeBool(w *bufio.Writer, b bool) {
	if b {
		w.WriteByte(1)
		return
	}
	w.WriteByte(0)
}

func readUvarint(r *bufio.Reader) (uint64, error) {
	return binary.ReadUvarint(r)
}

func readVarint(r *bufio.Reader) (int64, error) {
	return binary.ReadVarint(r)
}

// readCount reads a uvarint bounded by maxSerialCount, as an int.
func readCount(r *bufio.Reader) (int, error) {
	v, err := readUvarint(r)
	if err != nil {
		return 0, err
	}
	if v > maxSerialCount {
		return 0, fmt.Errorf("bytecode: count %d exceeds sanity limit", v)
	}
	return int(v), nil
}

func readString(r *bufio.Reader) (string, error) {
	n, err := readUvarint(r)
	if err != nil {
		return "", err
	}
	if n > maxSerialString {
		return "", fmt.Errorf("bytecode: string length %d exceeds sanity limit", n)
	}
	if n == 0 {
		return "", nil
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

func readBool(r *bufio.Reader) (bool, error) {
	b, err := r.ReadByte()
	if err != nil {
		return false, err
	}
	return b != 0, nil
}
