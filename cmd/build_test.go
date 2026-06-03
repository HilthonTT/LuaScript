package main

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/hilthontt/luascript/version"
)

// buildBundle is a test helper that mirrors runBuild's trailer-write
// logic without touching disk or os.Executable. Keep this in lockstep
// with the buf = append... block in runBuild.
func buildBundle(stub, script, ver string) []byte {
	out := make([]byte, 0, len(stub)+len(script)+len(ver)+trailerFixedLen)
	out = append(out, stub...)
	out = append(out, script...)
	out = append(out, ver...)
	out = binary.LittleEndian.AppendUint16(out, uint16(len(ver)))
	out = binary.LittleEndian.AppendUint64(out, uint64(len(script)))
	out = append(out, []byte(trailerMagic)...)
	return out
}

func TestTrailerRoundtrip(t *testing.T) {
	stub := "fake-stub-bytes-pretending-to-be-an-exe"
	script := "print('hello from a bundle')\nlocal x = 1\n"
	bundle := buildBundle(stub, script, version.Version)

	got, ok, err := readPayloadFrom(bytesReaderAt(bundle), int64(len(bundle)))
	if err != nil {
		t.Fatalf("readPayloadFrom: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true for a valid bundle")
	}
	if got != script {
		t.Fatalf("script mismatch:\n got:  %q\n want: %q", got, script)
	}

	// And the stub-strip offset should equal len(stub) so re-bundling
	// rebases off the original stub.
	off, err := stubOffsetFrom(bytesReaderAt(bundle), int64(len(bundle)))
	if err != nil {
		t.Fatalf("stubOffsetFrom: %v", err)
	}
	if off != int64(len(stub)) {
		t.Fatalf("stub offset = %d, want %d", off, len(stub))
	}
}

func TestNoTrailer(t *testing.T) {
	// A plain interpreter binary — no.lsc01 magic anywhere — must
	// return (false, nil), not an error.
	plain := []byte("not-a-bundle-just-an-exe-image-with-no-trailer")
	got, ok, err := readPayloadFrom(bytesReaderAt(plain), int64(len(plain)))
	if err != nil {
		t.Fatalf("readPayloadFrom plain: %v", err)
	}
	if ok {
		t.Fatalf("expected ok=false for plain binary, got payload %q", got)
	}

	// stubOffsetFrom on a plain binary should report the full size.
	off, err := stubOffsetFrom(bytesReaderAt(plain), int64(len(plain)))
	if err != nil {
		t.Fatalf("stubOffsetFrom plain: %v", err)
	}
	if off != int64(len(plain)) {
		t.Fatalf("stub offset on plain binary = %d, want %d", off, len(plain))
	}
}

func TestVersionMismatch(t *testing.T) {
	stub := "stub"
	script := "print('x')"
	// Pick a version string guaranteed to differ from version.Version.
	bogus := "definitely-not-" + version.Version
	bundle := buildBundle(stub, script, bogus)

	_, ok, err := readPayloadFrom(bytesReaderAt(bundle), int64(len(bundle)))
	if err == nil {
		t.Fatal("expected error on version mismatch, got nil")
	}
	if ok {
		t.Fatal("expected ok=false on version mismatch")
	}
	// Error should reference both versions so the user knows what to do.
	msg := err.Error()
	if !strings.Contains(msg, bogus) || !strings.Contains(msg, version.Version) {
		t.Fatalf("error %q should mention both %q and %q", msg, bogus, version.Version)
	}
}

func TestTooShortForTrailer(t *testing.T) {
	// File smaller than the fixed trailer size must not panic and must
	// report no payload.
	tiny := []byte("tiny")
	_, ok, err := readPayloadFrom(bytesReaderAt(tiny), int64(len(tiny)))
	if err != nil {
		t.Fatalf("readPayloadFrom tiny: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for too-short file")
	}
}
