package optimize

import (
	"math"

	"github.com/hilthontt/luascript/internal/compiler/ast"
)

// num is a numeric literal value tagged with its Lua subtype (integer/float).
type num struct {
	i       int64
	f       float64
	isFloat bool
}

func (n num) float() float64 {
	if n.isFloat {
		return n.f
	}
	return float64(n.i)
}

func asNum(e ast.Expression) (num, bool) {
	switch n := e.(type) {
	case *ast.IntegerLiteral:
		return num{i: n.Value}, true
	case *ast.FloatLiteral:
		return num{f: n.Value, isFloat: true}, true
	}
	return num{}, false
}

// tryFoldBinary returns a literal node when be has constant operands and the
// operation is safe to evaluate at compile time, or nil to keep be as-is.
func tryFoldBinary(be *ast.BinaryExpression) ast.Expression {
	switch be.Op {
	case "and", "or":
		return foldLogical(be)

	case "..":
		// v1: string-literal operands only. A numeric operand would need
		// Lua's exact integer / %.14g float formatting — deferred.
		l, lok := be.Left.(*ast.StringLiteral)
		r, rok := be.Right.(*ast.StringLiteral)
		if lok && rok {
			return mkString(be.BaseNode, l.Value+r.Value)
		}
		return nil

	case "==", "~=":
		if !isLiteral(be.Left) || !isLiteral(be.Right) {
			return nil
		}
		eq, ok := literalsEqual(be.Left, be.Right)
		if !ok {
			return nil
		}
		if be.Op == "~=" {
			eq = !eq
		}
		return mkBool(be.BaseNode, eq)
	}

	ln, lok := asNum(be.Left)
	rn, rok := asNum(be.Right)
	if !lok || !rok {
		// String relational comparison is the only remaining foldable case.
		switch be.Op {
		case "<", ">", "<=", ">=":
			l, lsok := be.Left.(*ast.StringLiteral)
			r, rsok := be.Right.(*ast.StringLiteral)
			if lsok && rsok {
				return mkBool(be.BaseNode, compareStrings(be.Op, l.Value, r.Value))
			}
		}
		return nil
	}

	switch be.Op {
	case "+", "-", "*", "//", "%":
		return foldArith(be, ln, rn)
	case "/", "^":
		// Lua 5.4: `/` and `^` always yield a float (division by zero gives
		// inf/NaN, which are valid float values — no runtime error).
		return foldFloatArith(be, ln, rn)
	case "<", ">", "<=", ">=":
		// Skip mixed int/float: Lua compares those mathematically exactly,
		// which a float() conversion would not reproduce for large integers.
		if ln.isFloat != rn.isFloat {
			return nil
		}
		return mkBool(be.BaseNode, compareNums(be.Op, ln, rn))
	case "&", "|", "~", "<<", ">>":
		return foldBitwise(be, ln, rn)
	}
	return nil
}

func tryFoldUnary(ue *ast.UnaryExpression) ast.Expression {
	switch ue.Op {
	case "-":
		switch n := ue.Operand.(type) {
		case *ast.IntegerLiteral:
			return mkInt(ue.BaseNode, -n.Value)
		case *ast.FloatLiteral:
			return mkFloat(ue.BaseNode, -n.Value)
		}
	case "not":
		if isLiteral(ue.Operand) {
			return mkBool(ue.BaseNode, !isTruthy(ue.Operand))
		}
	case "~":
		// Bitwise NOT — integer literals only (v1).
		if n, ok := ue.Operand.(*ast.IntegerLiteral); ok {
			return mkInt(ue.BaseNode, ^n.Value)
		}
	case "#":
		// Length of a string literal is its byte count. Tables are skipped:
		// `#` on a table can trigger a __len metamethod.
		if s, ok := ue.Operand.(*ast.StringLiteral); ok {
			return mkInt(ue.BaseNode, int64(len(s.Value)))
		}
	}
	return nil
}

// foldLogical evaluates `and`/`or` when the left operand is a known literal.
// This is safe because Lua short-circuits: the dropped side is never
// evaluated, so removing it cannot discard a side effect.
func foldLogical(be *ast.BinaryExpression) ast.Expression {
	if !isLiteral(be.Left) {
		return nil
	}
	leftTruthy := isTruthy(be.Left)
	if be.Op == "and" {
		if leftTruthy {
			return clampToSingle(be.BaseNode, be.Right)
		}
		return be.Left
	}
	// or
	if leftTruthy {
		return be.Left
	}
	return clampToSingle(be.BaseNode, be.Right)
}

// clampToSingle preserves single-value semantics when folding replaces a
// larger expression with one of its sub-expressions. `a and f()`, or an if
// expression arm, yields exactly one value — but returning a multi-valued
// node (call/vararg) verbatim would let all its values leak out in a
// multi-value position (return, call arg, table field). Wrapping it in a
// ParenExpression re-imposes the one-value adjustment; an already
// single-valued node is returned unchanged so later folding is not blocked.
func clampToSingle(b ast.BaseNode, e ast.Expression) ast.Expression {
	switch e.(type) {
	case *ast.CallExpression, *ast.MethodCallExpression, *ast.VarargExpression:
		return &ast.ParenExpression{BaseNode: b, Inner: e}
	}
	return e
}

// foldIfExpr folds an if expression's arms and prunes statically-known
// branches: a falsy-literal condition drops its clause, a truthy-literal
// condition truncates everything after it (its value becomes the `else`).
// When no clauses survive, the whole expression folds to its else-value.
// Dropping a condition is safe for the same reason foldLogical is: Lua
// evaluates conditions in order and never evaluates a dropped branch.
func foldIfExpr(n *ast.IfExpression) ast.Expression {
	kept := n.Clauses[:0]
	for _, cl := range n.Clauses {
		cl.Condition = foldExpr(cl.Condition)
		cl.Value = foldExpr(cl.Value)
		if isLiteral(cl.Condition) {
			if isTruthy(cl.Condition) {
				// This arm always wins over everything after it: it becomes
				// the else-value and the remaining clauses are dead.
				n.Clauses = kept
				n.Else = cl.Value
				if len(n.Clauses) == 0 {
					return clampToSingle(n.BaseNode, n.Else)
				}
				return n
			}
			continue // falsy literal: this arm can never be taken
		}
		kept = append(kept, cl)
	}
	n.Clauses = kept
	n.Else = foldExpr(n.Else)
	if len(n.Clauses) == 0 {
		return clampToSingle(n.BaseNode, n.Else)
	}
	return n
}

// foldArith handles + - * // % where at least one operand may be an integer.
func foldArith(be *ast.BinaryExpression, l, r num) ast.Expression {
	if l.isFloat || r.isFloat {
		lf, rf := l.float(), r.float()
		switch be.Op {
		case "+":
			return mkFloat(be.BaseNode, lf+rf)
		case "-":
			return mkFloat(be.BaseNode, lf-rf)
		case "*":
			return mkFloat(be.BaseNode, lf*rf)
		case "//":
			return mkFloat(be.BaseNode, math.Floor(lf/rf))
		case "%":
			// Lua float modulo is fmod with a sign correction (see vm/float.go).
			// math.Mod(x, ±Inf) == x, so `x % math.huge` folds to x, not NaN.
			m := math.Mod(lf, rf)
			if m != 0 && (m < 0) != (rf < 0) {
				m += rf
			}
			return mkFloat(be.BaseNode, m)
		}
		return nil
	}

	// Both integers. int64 arithmetic wraps, matching Lua's 64-bit integers.
	switch be.Op {
	case "+":
		return mkInt(be.BaseNode, l.i+r.i)
	case "-":
		return mkInt(be.BaseNode, l.i-r.i)
	case "*":
		return mkInt(be.BaseNode, l.i*r.i)
	case "//":
		if r.i == 0 {
			return nil // integer division by zero raises at runtime
		}
		return mkInt(be.BaseNode, floorDiv(l.i, r.i))
	case "%":
		if r.i == 0 {
			return nil // integer modulo by zero raises at runtime
		}
		return mkInt(be.BaseNode, floorMod(l.i, r.i))
	}
	return nil
}

// foldFloatArith handles `/` and `^`, which are always float in Lua 5.4.
func foldFloatArith(be *ast.BinaryExpression, l, r num) ast.Expression {
	lf, rf := l.float(), r.float()
	switch be.Op {
	case "/":
		return mkFloat(be.BaseNode, lf/rf)
	case "^":
		return mkFloat(be.BaseNode, math.Pow(lf, rf))
	}
	return nil
}

// foldBitwise handles & | ~ << >> on integer literals.
func foldBitwise(be *ast.BinaryExpression, l, r num) ast.Expression {
	if l.isFloat || r.isFloat {
		return nil // v1: integer literals only
	}
	switch be.Op {
	case "&":
		return mkInt(be.BaseNode, l.i&r.i)
	case "|":
		return mkInt(be.BaseNode, l.i|r.i)
	case "~":
		return mkInt(be.BaseNode, l.i^r.i)
	case "<<":
		return mkInt(be.BaseNode, shift(l.i, r.i))
	case ">>":
		return mkInt(be.BaseNode, shift(l.i, -r.i))
	}
	return nil
}

// shift implements Lua's 64-bit logical left shift. A negative count shifts
// right; a count with magnitude >= 64 yields 0.
func shift(a, n int64) int64 {
	switch {
	case n <= -64 || n >= 64:
		return 0
	case n >= 0:
		return int64(uint64(a) << uint(n))
	default:
		return int64(uint64(a) >> uint(-n))
	}
}

// floorDiv is Lua's `//` for integers: division rounding toward -infinity.
func floorDiv(a, b int64) int64 {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

// floorMod is Lua's `%` for integers: result takes the sign of the divisor.
func floorMod(a, b int64) int64 {
	m := a % b
	if m != 0 && ((m < 0) != (b < 0)) {
		m += b
	}
	return m
}

func compareNums(op string, l, r num) bool {
	if !l.isFloat && !r.isFloat {
		switch op {
		case "<":
			return l.i < r.i
		case ">":
			return l.i > r.i
		case "<=":
			return l.i <= r.i
		case ">=":
			return l.i >= r.i
		}
	}
	lf, rf := l.float(), r.float()
	switch op {
	case "<":
		return lf < rf
	case ">":
		return lf > rf
	case "<=":
		return lf <= rf
	case ">=":
		return lf >= rf
	}
	return false
}

func compareStrings(op, l, r string) bool {
	switch op {
	case "<":
		return l < r
	case ">":
		return l > r
	case "<=":
		return l <= r
	case ">=":
		return l >= r
	}
	return false
}

// literalsEqual compares two literal expressions with Lua `==` semantics.
// The second return is false when the comparison should not be folded
// (mixed int/float, where Lua's exact comparison can diverge from a float
// conversion for large integers).
func literalsEqual(l, r ast.Expression) (eq bool, ok bool) {
	switch lv := l.(type) {
	case *ast.NilLiteral:
		_, isNil := r.(*ast.NilLiteral)
		return isNil, true
	case *ast.BooleanLiteral:
		if rv, isBool := r.(*ast.BooleanLiteral); isBool {
			return lv.Value == rv.Value, true
		}
		return false, true
	case *ast.StringLiteral:
		if rv, isStr := r.(*ast.StringLiteral); isStr {
			return lv.Value == rv.Value, true
		}
		return false, true
	case *ast.IntegerLiteral:
		switch rv := r.(type) {
		case *ast.IntegerLiteral:
			return lv.Value == rv.Value, true
		case *ast.FloatLiteral:
			return false, false // mixed numeric: skip
		default:
			return false, true // int vs string/bool/nil: never equal
		}
	case *ast.FloatLiteral:
		switch rv := r.(type) {
		case *ast.FloatLiteral:
			return lv.Value == rv.Value, true
		case *ast.IntegerLiteral:
			return false, false // mixed numeric: skip
		default:
			return false, true
		}
	}
	return false, false
}

func mkInt(b ast.BaseNode, v int64) *ast.IntegerLiteral {
	return &ast.IntegerLiteral{BaseNode: b, Value: v}
}

func mkFloat(b ast.BaseNode, v float64) *ast.FloatLiteral {
	return &ast.FloatLiteral{BaseNode: b, Value: v}
}

func mkBool(b ast.BaseNode, v bool) *ast.BooleanLiteral {
	return &ast.BooleanLiteral{BaseNode: b, Value: v}
}

func mkString(b ast.BaseNode, v string) *ast.StringLiteral {
	return &ast.StringLiteral{BaseNode: b, Value: v}
}
