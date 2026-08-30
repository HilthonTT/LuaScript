package vm

// registerLibraryModules installs the math / string / table / io modules.
// The string table is also wired in as the metatable's __index for string
// values so `("hi"):upper()` resolves to string.upper("hi").
func registerLibraryModules(v *VM) {
	v.Globals.Set("math", buildMathLibrary())
	stringLib := buildStringLibrary()
	v.Globals.Set("string", stringLib)
	v.Globals.Set("table", buildTableLibrary())
	v.Globals.Set("io", buildIOLibrary())

	// Strings carry a shared metatable whose __index is the `string` table.
	// This is what makes the colon-method syntax work on string values.
	v.stringMeta = NewTable(0, 1)
	v.stringMeta.Set("__index", stringLib)
}
