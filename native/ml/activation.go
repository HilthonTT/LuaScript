package ml

// Mode denotes interference mode
type Mode int

const (
	ModeDefault Mode = iota
	ModeMultiClass
	ModeRegression
	ModeBinary
	ModeMultiLabel
)
