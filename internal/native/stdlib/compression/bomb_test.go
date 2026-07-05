package compression

import (
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
)

// TestInflateRejectsDecompressionBomb verifies the output cap: a small gzip
// stream whose inflated size exceeds maxDecodeBytes must raise rather than
// allocating the full expansion.
func TestInflateRejectsDecompressionBomb(t *testing.T) {
	// Compress a highly-repetitive payload larger than the cap. Zeroes
	// compress to a tiny stream, so this stays small on disk.
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	chunk := make([]byte, 1<<20) // 1 MiB of zeroes
	for written := int64(0); written <= maxDecodeBytes; written += int64(len(chunk)) {
		if _, err := w.Write(chunk); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	r, err := gzip.NewReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("new reader: %v", err)
	}
	_, err = inflate("test", r)
	if err == nil {
		t.Fatal("inflate accepted an over-cap stream; want an error")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error = %q, want it to mention exceeding the cap", err.Error())
	}
}

// TestInflateAcceptsNormalStream confirms a normal, under-cap payload still
// round-trips.
func TestInflateAcceptsNormalStream(t *testing.T) {
	const msg = "the quick brown fox jumps over the lazy dog"
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write([]byte(msg)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	r, err := gzip.NewReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("new reader: %v", err)
	}
	out, err := inflate("test", r)
	if err != nil {
		t.Fatalf("inflate: %v", err)
	}
	if out != msg {
		t.Errorf("roundtrip = %q, want %q", out, msg)
	}
}
