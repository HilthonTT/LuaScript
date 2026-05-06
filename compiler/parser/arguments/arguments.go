package arguments

import "github.com/hilthontt/sakura-lang/compiler/token"

const (
	NormalArg = iota
	OptionedArg
	SplatArg
	RequiredKeywordArg
	OptionalKeywordArg
)

// Types is a table maps argument types enum to the their real name
var Types = map[int]string{
	NormalArg:          "Normal argument",
	OptionedArg:        "Optioned argument",
	RequiredKeywordArg: "Keyword argument",
	OptionalKeywordArg: "Optioned keyword argument",
	SplatArg:           "Splat argument",
}

// Tokens marks token types that can be used as method call arguments
var Tokens = map[token.Type]bool{
	token.Int:    true,
	token.String: true,
	token.True:   true,
	token.False:  true,
	token.Nil:    true,
	token.Ident:  true,
}
