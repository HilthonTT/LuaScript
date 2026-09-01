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

func Disabled() bool {
	return os.Getenv("LUASCRIPT_NOCACHE") != ""
}

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
	fmt.Fprintf(h, "luascript-bc\x00%d\x00%d\x00%s\x00%s\x00",
		bytecode.SerialVersion, bytecode.InstructionCount, version.GetVersionString(), buildStamp())
	h.Write([]byte(src))
	return filepath.Join(dir, hex.EncodeToString(h.Sum(nil))+".lscb"), true
}

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
		os.Remove(tmpName)
	}
}
