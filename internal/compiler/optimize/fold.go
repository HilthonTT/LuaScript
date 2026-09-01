package optimize

import (
	"math"

	"github.com/hilthontt/luascript/internal/compiler/ast"
)

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

func tryFoldBinary(be *ast.BinaryExpression) ast.Expression {
	switch be.Op {
	case "and", "or":
		return foldLogical(be)

	case "..":
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
		return foldFloatArith(be, ln, rn)
	case "<", ">", "<=", ">=":
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
		if n, ok := ue.Operand.(*ast.IntegerLiteral); ok {
			return mkInt(ue.BaseNode, ^n.Value)
		}
	case "#":
		if s, ok := ue.Operand.(*ast.StringLiteral); ok {
			return mkInt(ue.BaseNode, int64(len(s.Value)))
		}
	}
	return nil
}

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
	if leftTruthy {
		return be.Left
	}
	return clampToSingle(be.BaseNode, be.Right)
}

func clampToSingle(b ast.BaseNode, e ast.Expression) ast.Expression {
	switch e.(type) {
	case *ast.CallExpression, *ast.MethodCallExpression, *ast.VarargExpression:
		return &ast.ParenExpression{BaseNode: b, Inner: e}
	}
	return e
}

func foldIfExpr(n *ast.IfExpression) ast.Expression {
	kept := n.Clauses[:0]
	for _, cl := range n.Clauses {
		cl.Condition = foldExpr(cl.Condition)
		cl.Value = foldExpr(cl.Value)
		if isLiteral(cl.Condition) {
			if isTruthy(cl.Condition) {
				n.Clauses = kept
				n.Else = cl.Value
				if len(n.Clauses) == 0 {
					return clampToSingle(n.BaseNode, n.Else)
				}
				return n
			}
			continue
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
			m := math.Mod(lf, rf)
			if m != 0 && (m < 0) != (rf < 0) {
				m += rf
			}
			return mkFloat(be.BaseNode, m)
		}
		return nil
	}

	switch be.Op {
	case "+":
		return mkInt(be.BaseNode, l.i+r.i)
	case "-":
		return mkInt(be.BaseNode, l.i-r.i)
	case "*":
		return mkInt(be.BaseNode, l.i*r.i)
	case "//":
		if r.i == 0 {
			return nil
		}
		return mkInt(be.BaseNode, floorDiv(l.i, r.i))
	case "%":
		if r.i == 0 {
			return nil
		}
		return mkInt(be.BaseNode, floorMod(l.i, r.i))
	}
	return nil
}

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

func foldBitwise(be *ast.BinaryExpression, l, r num) ast.Expression {
	if l.isFloat || r.isFloat {
		return nil
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

func floorDiv(a, b int64) int64 {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

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
			return false, false
		default:
			return false, true
		}
	case *ast.FloatLiteral:
		switch rv := r.(type) {
		case *ast.FloatLiteral:
			return lv.Value == rv.Value, true
		case *ast.IntegerLiteral:
			return false, false
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
