package compression

import (
	"bytes"
	"strings"
	"testing"
)

// TestRLERoundTrip exercises the PackBits codec on inputs that the previous
// decimal/regex RLE mangled: digit bytes (which used to merge with the count),
// non-word bytes (which the \w decode regex dropped), NUL, and long runs that
// overflow a single control byte.
func TestRLERoundTrip(t *testing.T) {
	cases := map[string]string{
		"empty":            "",
		"single":           "a",
		"short run":        "aaa",
		"digits in data":   "11",                       // old encoder produced "21" -> 21 ones
		"mixed digits":     "aa3333bb",                 // counts must not collide with data digits
		"binary/punct":     "a,b;c\t\n!@#",             // non-\w bytes survive
		"nul bytes":        "\x00\x00\x00x\x00",         // NUL round-trips
		"long run":         strings.Repeat("z", 1000),   // exceeds the 128 run cap, splits into packets
		"long literals":    "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ" + "~`",
		"alternating":      strings.Repeat("ab", 200),   // worst case for runs; literals dominate
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

// TestRLEAllBytes round-trips every possible byte value to confirm full
// binary safety, not just the printable subset.
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

// TestRLECompressesRuns confirms long runs actually shrink and that the
// PackBits literal packing keeps incompressible data from blowing up 2x.
func TestRLECompressesRuns(t *testing.T) {
	run := strings.Repeat("q", 5000)
	if enc := rleEncode([]byte(run)); len(enc) >= len(run) {
		t.Errorf("run did not compress: %d -> %d", len(run), len(enc))
	}
	// Incompressible (no adjacent dupes): PackBits overhead is ~1 byte per 128.
	noDupes := []byte("abcdefghijklmnopqrstuvwxyz")
	if enc := rleEncode(noDupes); len(enc) > len(noDupes)+len(noDupes)/128+1 {
		t.Errorf("literal blowup too large: %d -> %d", len(noDupes), len(enc))
	}
}

// TestRLEDecodeRejectsTruncated confirms a malformed stream raises instead of
// returning a partial result.
func TestRLEDecodeRejectsTruncated(t *testing.T) {
	// Control byte 0x82 promises a repeat packet but the payload byte is absent.
	if _, err := rleDecode("test", []byte{0x82}); err == nil {
		t.Fatal("expected error for truncated repeat run")
	}
	// Control byte 0x05 promises 6 literal bytes but only 2 follow.
	if _, err := rleDecode("test", []byte{0x05, 'a', 'b'}); err == nil {
		t.Fatal("expected error for truncated literal run")
	}
}
