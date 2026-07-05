package compression

import "github.com/hilthontt/luascript/internal/vm"

// Run-length codec used by compression.rle_encode / rle_decode (wired in
// std.go). It uses the PackBits scheme so incompressible data grows by at most
// ~1/128 rather than the 2x blowup a naive [count][byte] RLE would pay, while
// still collapsing long runs. Binary-safe: every byte value round-trips,
// including digits and NUL, which the previous decimal/regex encoding could
// not.
//
// The stream is a sequence of (control, payload) packets:
//
//   - control 0..127   → the next control+1 bytes are literals (1..128 bytes)
//   - control 129..255 → repeat the single next byte 257-control times (2..128)
//   - control 128      → reserved no-op (never emitted; skipped on decode)

// rleEncode compresses data with the PackBits run-length scheme. Runs of two or
// more identical bytes are emitted as repeat packets; everything else
// accumulates into literal packets. Both lengths are capped at 128 so a single
// control byte always suffices.
func rleEncode(data []byte) []byte {
	out := make([]byte, 0, len(data))
	n := len(data)
	for i := 0; i < n; {
		// Measure the run of identical bytes starting at i (cap 128).
		run := 1
		for i+run < n && data[i+run] == data[i] && run < 128 {
			run++
		}
		if run >= 2 {
			out = append(out, byte(257-run), data[i])
			i += run
			continue
		}
		// Otherwise gather literals until a run of >=2 begins or we hit 128.
		start := i
		for i < n && i-start < 128 {
			if i+1 < n && data[i] == data[i+1] {
				break
			}
			i++
		}
		out = append(out, byte(i-start-1))
		out = append(out, data[start:i]...)
	}
	return out
}

// rleDecode reverses rleEncode. It enforces the same decompression-bomb cap as
// the flate decoders so a small crafted stream of repeat packets cannot expand
// without bound. A truncated or malformed stream raises rather than returning a
// partial result.
func rleDecode(site string, data []byte) ([]byte, error) {
	out := make([]byte, 0, len(data)*2)
	n := len(data)
	for i := 0; i < n; {
		ctrl := data[i]
		i++
		switch {
		case ctrl < 128: // literal run of ctrl+1 bytes
			cnt := int(ctrl) + 1
			if i+cnt > n {
				return nil, vm.Errorf("%s: truncated literal run", site)
			}
			out = append(out, data[i:i+cnt]...)
			i += cnt
		case ctrl > 128: // repeat the next byte 257-ctrl times
			cnt := 257 - int(ctrl)
			if i >= n {
				return nil, vm.Errorf("%s: truncated repeat run", site)
			}
			b := data[i]
			i++
			for j := 0; j < cnt; j++ {
				out = append(out, b)
			}
		}
		if int64(len(out)) > maxDecodeBytes {
			return nil, vm.Errorf("%s: decompressed output exceeds %d bytes", site, int64(maxDecodeBytes))
		}
	}
	return out, nil
}
