package compression

import "github.com/hilthontt/luascript/internal/vm"

func rleEncode(data []byte) []byte {
	out := make([]byte, 0, len(data))
	n := len(data)
	for i := 0; i < n; {
		run := 1
		for i+run < n && data[i+run] == data[i] && run < 128 {
			run++
		}
		if run >= 2 {
			out = append(out, byte(257-run), data[i])
			i += run
			continue
		}
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

func rleDecode(site string, data []byte) ([]byte, error) {
	out := make([]byte, 0, len(data)*2)
	n := len(data)
	for i := 0; i < n; {
		ctrl := data[i]
		i++
		switch {
		case ctrl < 128:
			cnt := int(ctrl) + 1
			if i+cnt > n {
				return nil, vm.Errorf("%s: truncated literal run", site)
			}
			out = append(out, data[i:i+cnt]...)
			i += cnt
		case ctrl > 128:
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
