package vm

func registerLibraryModules(v *VM) {
	v.Globals.Set("math", buildMathLibrary())
	stringLib := buildStringLibrary()
	v.Globals.Set("string", stringLib)
	v.Globals.Set("table", buildTableLibrary())
	v.Globals.Set("io", buildIOLibrary())

	v.stringMeta = NewTable(0, 1)
	v.stringMeta.Set("__index", stringLib)
}
