package vm

// formatBool renders a Lua boolean.
func formatBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
