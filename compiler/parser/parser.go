package parser

import (
	"github.com/hilthontt/sakura-lang/compiler/lexer"
	"github.com/hilthontt/sakura-lang/compiler/parser/errors"
	"github.com/hilthontt/sakura-lang/compiler/parser/events"
	"github.com/hilthontt/sakura-lang/compiler/parser/states"
	"github.com/hilthontt/sakura-lang/compiler/token"
	"github.com/looplab/fsm"
)

// Mode determines the running mode. These are the enums for marking parser's mode, which decides whether it should pop unused values.
type Mode int

// These are the enums for marking parser's mode, which decides whether it should pop unused values.
const (
	NormalMode Mode = iota + 1
	REPLMode
	TestMode
)

type Parser struct {
	Lexer *lexer.Lexer
	error *errors.Error

	curToken  token.Token
	peekToken token.Token

	prefixParseFns map[token.Token]prefixParseFn
	infixParseFns  map[token.Token]infixParseFn

	// Determine if call expression should accept block argument
	// currently only used when parsing while statement.
	// However, this is not a very good practice should change it in the future.
	acceptBlock bool
	fsm         *fsm.FSM
	Mode        Mode
}

func New(l *lexer.Lexer) *Parser {
	p := &Parser{
		Lexer:       l,
		acceptBlock: true,
	}

	p.fsm = fsm.NewFSM(
		states.Normal,
		fsm.Events{
			{
				Name: events.ParseFuncCall,
				Src:  []string{states.Normal},
				Dst:  states.ParsingFuncCall,
			},
			{
				Name: events.ParseMethodParam,
				Src:  []string{states.Normal, states.ParsingAssignment},
				Dst:  states.ParsingMethodParam,
			},
			{
				Name: events.ParseAssignment,
				Src:  []string{states.Normal, states.ParsingFuncCall},
				Dst:  states.ParsingAssignment,
			},
			{
				Name: events.BackToNormal,
				Src:  []string{states.ParsingFuncCall, states.ParsingMethodParam, states.ParsingAssignment},
				Dst:  states.Normal,
			},
		},
		fsm.Callbacks{},
	)

	p.prefixParseFns = make(map[token.Token]prefixParseFn)

	p.infixParseFns = make(map[token.Token]infixParseFn)

	return p
}
