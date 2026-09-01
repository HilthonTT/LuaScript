package main

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/hilthontt/luascript/internal/version"
)

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

	off, err := stubOffsetFrom(bytesReaderAt(bundle), int64(len(bundle)))
	if err != nil {
		t.Fatalf("stubOffsetFrom: %v", err)
	}
	if off != int64(len(stub)) {
		t.Fatalf("stub offset = %d, want %d", off, len(stub))
	}
}

func TestNoTrailer(t *testing.T) {
	plain := []byte("not-a-bundle-just-an-exe-image-with-no-trailer")
	got, ok, err := readPayloadFrom(bytesReaderAt(plain), int64(len(plain)))
	if err != nil {
		t.Fatalf("readPayloadFrom plain: %v", err)
	}
	if ok {
		t.Fatalf("expected ok=false for plain binary, got payload %q", got)
	}

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
	bogus := "definitely-not-" + version.Version
	bundle := buildBundle(stub, script, bogus)

	_, ok, err := readPayloadFrom(bytesReaderAt(bundle), int64(len(bundle)))
	if err == nil {
		t.Fatal("expected error on version mismatch, got nil")
	}
	if ok {
		t.Fatal("expected ok=false on version mismatch")
	}
	msg := err.Error()
	if !strings.Contains(msg, bogus) || !strings.Contains(msg, version.Version) {
		t.Fatalf("error %q should mention both %q and %q", msg, bogus, version.Version)
	}
}

func TestTooShortForTrailer(t *testing.T) {
	tiny := []byte("tiny")
	_, ok, err := readPayloadFrom(bytesReaderAt(tiny), int64(len(tiny)))
	if err != nil {
		t.Fatalf("readPayloadFrom tiny: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for too-short file")
	}
}
