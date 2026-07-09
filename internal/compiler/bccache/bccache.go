// Package bccache is the on-disk bytecode compile cache. Scripts compiled
// from files (RunFile, require, loadfile) store their emitted main chunk
// under the user cache directory, keyed by a hash of the source text, the
// interpreter version, and the serialization format — so a source edit, an
// interpreter upgrade, or a bytecode-layout change each miss cleanly and
// recompile. A hit skips the whole lex → parse → typecheck → fold → codegen
// pipeline. Semantics are unchanged: a chunk is only cached after a fully
// successful compile, so scripts with parse/type errors are never served
// from cache.
//
// The cache is best-effort throughout: any I/O or decode problem silently
// falls back to a fresh compile (and overwrites the bad entry). Set
// LUASCRIPT_NOCACHE=1 to disable it entirely.
package bccache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/hilthontt/luascript/internal/compiler"
	"github.com/hilthontt/luascript/internal/compiler/bytecode"
	"github.com/hilthontt/luascript/internal/compiler/parser"
	"github.com/hilthontt/luascript/internal/version"
)

// buildStamp identifies the actual interpreter build. The version constants
// are hardcoded (no -ldflags injection), so on their own they cannot tell
// two builds apart: a codegen or optimizer change that alters emitted
// bytecode without touching the opcode count or SerialVersion would silently
// serve stale chunks. The executable's size+mtime change on every rebuild
// (including the temp binaries `go run` produces), so they make dev
// iteration invalidate naturally.
var buildStamp = sync.OnceValue(func() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	fi, err := os.Stat(exe)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d:%d", fi.Size(), fi.ModTime().UnixNano())
})

// Disabled reports whether the cache is turned off for this process.
func Disabled() bool {
	return os.Getenv("LUASCRIPT_NOCACHE") != ""
}

// CompileCached compiles src as a normal-mode chunk, consulting the bytecode
// cache. It returns the runnable main chunk (nested functions are reachable
// through its Protos table). On a miss the freshly compiled chunk is stored
// best-effort before returning.
//
// Only file-backed, normal-mode compiles should go through here: REPL chunks
// carry REPL-mode codegen differences and `load()` string chunks are dynamic
// one-offs that would only pollute the cache.
func CompileCached(src string) (*bytecode.InstructionSet, error) {
	if Disabled() {
		return compileFresh(src)
	}
	path, ok := entryPath(src)
	if ok {
		if f, err := os.Open(path); err == nil {
			main, derr := bytecode.DeserializeChunk(f)
			f.Close()
			if derr == nil {
				return main, nil
			}
			// Corrupt/stale entry: fall through and overwrite it.
		}
	}
	main, err := compileFresh(src)
	if err != nil {
		return nil, err
	}
	if ok {
		store(path, main)
	}
	return main, nil
}

func compileFresh(src string) (*bytecode.InstructionSet, error) {
	chunks, err := compiler.CompileToInstructions(src, parser.NormalMode)
	if err != nil {
		return nil, err
	}
	return chunks[0], nil
}

// entryPath computes the cache file for src. ok is false when no cache
// directory is available (best-effort: caching is simply skipped then).
// LUASCRIPT_CACHE_DIR overrides the default user-cache location (used by
// tests and by deployments that want the cache somewhere specific).
func entryPath(src string) (string, bool) {
	dir := os.Getenv("LUASCRIPT_CACHE_DIR")
	if dir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return "", false
		}
		dir = filepath.Join(base, "luascript", "bytecode")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false
	}
	h := sha256.New()
	// Domain-separate the key by everything that invalidates an entry:
	// bytecode layout, opcode numbering, the interpreter version, and the
	// concrete build (see buildStamp).
	fmt.Fprintf(h, "luascript-bc\x00%d\x00%d\x00%s\x00%s\x00",
		bytecode.SerialVersion, bytecode.InstructionCount, version.GetVersionString(), buildStamp())
	h.Write([]byte(src))
	return filepath.Join(dir, hex.EncodeToString(h.Sum(nil))+".lscb"), true
}

// store writes the chunk atomically (temp file + rename) so a crash or a
// concurrent reader never observes a truncated entry.
func store(path string, main *bytecode.InstructionSet) {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".lscb-*")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	serr := bytecode.SerializeChunk(tmp, main)
	cerr := tmp.Close()
	if serr != nil || cerr != nil {
		os.Remove(tmpName)
		return
	}
	if err := os.Rename(tmpName, path); err != nil {
		// Windows: rename over an existing file can fail if a concurrent
		// process just wrote the same entry — identical content, keep theirs.
		os.Remove(tmpName)
	}
}
