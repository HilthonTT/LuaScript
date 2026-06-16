package uuid

import "encoding/hex"

// formatUUID renders 16 bytes as the canonical 8-4-4-4-12 hex string.
func formatUUID(b [16]byte) string {
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}
