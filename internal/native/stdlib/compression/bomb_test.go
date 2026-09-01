package compression

import (
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
)

func TestInflateRejectsDecompressionBomb(t *testing.T) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	chunk := make([]byte, 1<<20)
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
