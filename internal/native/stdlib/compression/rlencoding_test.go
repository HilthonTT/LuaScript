package compression

import (
	"bytes"
	"strings"
	"testing"
)

func TestRLERoundTrip(t *testing.T) {
	cases := map[string]string{
		"empty":            "",
		"single":           "a",
		"short run":        "aaa",
		"digits in data":   "11",
		"mixed digits":     "aa3333bb",
		"binary/punct":     "a,b;c\t\n!@#",
		"nul bytes":        "\x00\x00\x00x\x00",
		"long run":         strings.Repeat("z", 1000),
		"long literals":    "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ" + "~`",
		"alternating":      strings.Repeat("ab", 200),
		"runs and literal": "aaaaabcdeeeefg",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			enc := rleEncode([]byte(in))
			dec, err := rleDecode("test", enc)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if !bytes.Equal(dec, []byte(in)) {
				t.Fatalf("roundtrip mismatch\n in  = %q\n out = %q", in, string(dec))
			}
		})
	}
}

func TestRLEAllBytes(t *testing.T) {
	in := make([]byte, 256)
	for i := range in {
		in[i] = byte(i)
	}
	dec, err := rleDecode("test", rleEncode(in))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !bytes.Equal(dec, in) {
		t.Fatal("all-bytes roundtrip mismatch")
	}
}

func TestRLECompressesRuns(t *testing.T) {
	run := strings.Repeat("q", 5000)
	if enc := rleEncode([]byte(run)); len(enc) >= len(run) {
		t.Errorf("run did not compress: %d -> %d", len(run), len(enc))
	}
	noDupes := []byte("abcdefghijklmnopqrstuvwxyz")
	if enc := rleEncode(noDupes); len(enc) > len(noDupes)+len(noDupes)/128+1 {
		t.Errorf("literal blowup too large: %d -> %d", len(noDupes), len(enc))
	}
}

func TestRLEDecodeRejectsTruncated(t *testing.T) {
	if _, err := rleDecode("test", []byte{0x82}); err == nil {
		t.Fatal("expected error for truncated repeat run")
	}
	if _, err := rleDecode("test", []byte{0x05, 'a', 'b'}); err == nil {
		t.Fatal("expected error for truncated literal run")
	}
}
