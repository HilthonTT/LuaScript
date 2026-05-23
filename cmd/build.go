package main

import (
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/hilthontt/sakura-lang/compiler"
	"github.com/hilthontt/sakura-lang/compiler/parser"
	"github.com/hilthontt/sakura-lang/version"
	"github.com/hilthontt/sakura-lang/vm"
)

// Trailer layout (read backwards from end of file):
//
//	<magic>        8 bytes  ("SAKURA01")
//	<scriptLen>    8 bytes  uint64 little-endian
//	<versionLen>   2 bytes  uint16 little-endian
//	<version>      versionLen bytes (ASCII)
//	<script>       scriptLen  bytes (UTF-8 source)
//
// A file with no trailer is just the plain interpreter binary.
const (
	trailerMagic    = "SAKURA01"
	trailerMagicLen = 8
	trailerFixedLen = trailerMagicLen + 8 /* scriptLen */ + 2 /* versionLen */

	// Sanity bounds to reject obviously corrupt trailers without trying
	// to read huge or negative-after-truncation amounts of data.
	maxPayloadBytes = 64 * 1024 * 1024 // 64 MiB
	maxVersionBytes = 256
)

// readPayloadFrom reads a trailer from r (assumed to be the bytes of a
// possibly-bundled binary). Returns (script, true, nil) when a valid
// trailer is present and the version stamp matches version.Version,
// (\"\", false, nil) when no trailer is present, or an error for a
// trailer that is present but malformed or version-mismatched.
func readPayloadFrom(r io.ReaderAt, size int64) (string, bool, error) {
	if size < int64(trailerFixedLen) {
		return "", false, nil
	}

	magic := make([]byte, trailerMagicLen)
	if _, err := r.ReadAt(magic, size-int64(trailerMagicLen)); err != nil {
		return "", false, fmt.Errorf("read trailer magic: %w", err)
	}
	if string(magic) != trailerMagic {
		return "", false, nil
	}

	var scriptLen uint64
	var versionLen uint16
	scriptLenOff := size - int64(trailerMagicLen) - 8
	versionLenOff := scriptLenOff - 2

	buf8 := make([]byte, 8)
	if _, err := r.ReadAt(buf8, scriptLenOff); err != nil {
		return "", false, fmt.Errorf("read trailer scriptLen: %w", err)
	}
	scriptLen = binary.LittleEndian.Uint64(buf8)

	buf2 := make([]byte, 2)
	if _, err := r.ReadAt(buf2, versionLenOff); err != nil {
		return "", false, fmt.Errorf("read trailer versionLen: %w", err)
	}
	versionLen = binary.LittleEndian.Uint16(buf2)

	if scriptLen > maxPayloadBytes {
		return "", false, fmt.Errorf("embedded script too large: %d bytes", scriptLen)
	}
	if versionLen == 0 || versionLen > maxVersionBytes {
		return "", false, fmt.Errorf("invalid embedded version length: %d", versionLen)
	}

	totalBack := int64(trailerFixedLen) + int64(versionLen) + int64(scriptLen)
	if totalBack > size {
		return "", false, fmt.Errorf("trailer claims more bytes (%d) than file holds (%d)", totalBack, size)
	}

	versionOff := versionLenOff - int64(versionLen)
	scriptOff := versionOff - int64(scriptLen)

	ver := make([]byte, versionLen)
	if _, err := r.ReadAt(ver, versionOff); err != nil {
		return "", false, fmt.Errorf("read trailer version: %w", err)
	}
	if string(ver) != version.Version {
		return "", false, fmt.Errorf("bundled with sakura %q, this is sakura %q", string(ver), version.Version)
	}

	src := make([]byte, scriptLen)
	if _, err := r.ReadAt(src, scriptOff); err != nil {
		return "", false, fmt.Errorf("read trailer script: %w", err)
	}
	return string(src), true, nil
}

// stubOffsetFrom returns the byte offset where the stub binary ends —
// i.e. the position where the trailer (including version string and
// script source) begins. Returns size when no trailer is present.
func stubOffsetFrom(r io.ReaderAt, size int64) (int64, error) {
	if size < int64(trailerFixedLen) {
		return size, nil
	}
	magic := make([]byte, trailerMagicLen)
	if _, err := r.ReadAt(magic, size-int64(trailerMagicLen)); err != nil {
		return 0, err
	}
	if string(magic) != trailerMagic {
		return size, nil
	}
	buf8 := make([]byte, 8)
	if _, err := r.ReadAt(buf8, size-int64(trailerMagicLen)-8); err != nil {
		return 0, err
	}
	scriptLen := binary.LittleEndian.Uint64(buf8)
	buf2 := make([]byte, 2)
	if _, err := r.ReadAt(buf2, size-int64(trailerMagicLen)-8-2); err != nil {
		return 0, err
	}
	versionLen := binary.LittleEndian.Uint16(buf2)
	totalBack := int64(trailerFixedLen) + int64(versionLen) + int64(scriptLen)
	if totalBack > size {
		return 0, errors.New("malformed trailer")
	}
	return size - totalBack, nil
}

// readEmbeddedPayload opens the running executable and returns its
// embedded script source, if any.
func readEmbeddedPayload() (string, bool, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", false, err
	}
	f, err := os.Open(exe)
	if err != nil {
		return "", false, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", false, err
	}
	return readPayloadFrom(f, info.Size())
}

// runBundled executes an embedded script on a fresh VM, bypassing the
// REPL package entirely. Mirrors the registration done in run() for the
// normal CLI path, but uses parser.NormalMode (NOT REPLMode) so that
// chunk-root `local x = v` retains lexical-block semantics — REPLMode
// would silently promote those locals to globals.
func runBundled(src string) int {
	v := vm.New()
	registerAllNatives(v)

	chunks, err := compiler.CompileToInstructions(src, parser.NormalMode)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sakura:", err)
		return 1
	}
	if len(chunks) == 0 {
		return 0
	}
	if err := v.Run(chunks[0]); err != nil {
		fmt.Fprintln(os.Stderr, "sakura:", err)
		return 1
	}
	return 0
}

// runBuild implements `sakura build script.sakura -o out.exe`.
// Exit codes: 0 success, 1 I/O or parse error, 2 usage error.
func runBuild(argv []string) int {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	out := fs.String("o", "", "output binary path (required)")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(argv); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	args := fs.Args()
	if len(args) != 1 || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: sakura build -o <output> <script.sakura>")
		return 2
	}
	scriptPath := args[0]

	src, err := os.ReadFile(scriptPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "build:", err)
		return 1
	}

	// Fail at build time on syntax errors so the bundled .exe doesn't
	// die on first run. Compiled output is discarded — payload is source.
	if _, err := compiler.CompileToInstructions(string(src), parser.NormalMode); err != nil {
		fmt.Fprintln(os.Stderr, "build: parse:", err)
		return 1
	}

	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "build: locate sakura:", err)
		return 1
	}
	stubAll, err := os.ReadFile(exe)
	if err != nil {
		fmt.Fprintln(os.Stderr, "build: read sakura:", err)
		return 1
	}

	// Strip any existing trailer so re-bundling doesn't nest payloads.
	stubLen, err := stubOffsetFrom(bytesReaderAt(stubAll), int64(len(stubAll)))
	if err != nil {
		fmt.Fprintln(os.Stderr, "build: inspect sakura:", err)
		return 1
	}
	stub := stubAll[:stubLen]

	verBytes := []byte(version.Version)
	if len(verBytes) > maxVersionBytes {
		fmt.Fprintln(os.Stderr, "build: version string too long")
		return 1
	}

	buf := make([]byte, 0, len(stub)+len(src)+len(verBytes)+trailerFixedLen)
	buf = append(buf, stub...)
	buf = append(buf, src...)
	buf = append(buf, verBytes...)
	buf = binary.LittleEndian.AppendUint16(buf, uint16(len(verBytes)))
	buf = binary.LittleEndian.AppendUint64(buf, uint64(len(src)))
	buf = append(buf, []byte(trailerMagic)...)

	if err := os.WriteFile(*out, buf, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "build: write output:", err)
		return 1
	}
	return 0
}

// bytesReaderAt adapts a []byte to io.ReaderAt without pulling in
// bytes.Reader's seek state — keeps the stub-strip path branch-free.
type bytesReaderAt []byte

func (b bytesReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= int64(len(b)) {
		return 0, io.EOF
	}
	n := copy(p, b[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}
