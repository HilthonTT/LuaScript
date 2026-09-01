package analyze

import (
	"fmt"
	"strings"

	"github.com/hilthontt/luascript/internal/compiler/ast"
)

type securityPass struct{}

func (securityPass) Name() string {
	return "security"
}

func (securityPass) Run(prog *ast.Program, _ Options, rep *Report) {
	w := &walker{}
	w.onExpr = func(e ast.Expression) {
		switch n := e.(type) {
		case *ast.CallExpression:
			switch dottedName(n.Func) {
			case "crypto.md5", "crypto.sha1":
				rep.add(Finding{
					Pass:     "security",
					Rule:     "weak-crypto",
					Severity: SeverityWarning,
					Line:     n.Line(),
					Message: fmt.Sprintf("%s is a weak hash — prefer sha256 or stronger",
						dottedName(n.Func)),
				})
			}
		case *ast.StringLiteral:
			if strings.HasPrefix(n.Value, "http://") {
				rep.add(Finding{
					Pass:     "security",
					Rule:     "plaintext-http",
					Severity: SeverityWarning,
					Line:     n.Line(),
					Message:  "plaintext http:// URL — prefer https://",
				})
			}
		case *ast.TableConstructor:
			for _, f := range n.Fields {
				if name := fieldKeyName(f); credentialName(name) {
					checkCredential(name, f.Value, rep)
				}
			}
		}
	}
	w.onStmt = func(s ast.Statement) {
		switch n := s.(type) {
		case *ast.LocalStatement:
			for i, ln := range n.Names {
				if i < len(n.Values) && credentialName(ln.Name) {
					checkCredential(ln.Name, n.Values[i], rep)
				}
			}
		case *ast.AssignStatement:
			for i, t := range n.Targets {
				name := targetName(t)
				if i < len(n.Values) && credentialName(name) {
					checkCredential(name, n.Values[i], rep)
				}
			}
		}
	}
	w.walkBlock(prog.Block)
}

func checkCredential(name string, value ast.Expression, rep *Report) {
	lit, ok := value.(*ast.StringLiteral)
	if !ok || lit.Value == "" {
		return
	}
	rep.add(Finding{
		Pass:     "security",
		Rule:     "hardcoded-credential",
		Severity: SeverityWarning,
		Line:     lit.Line(),
		Message:  fmt.Sprintf("'%s' assigned a hardcoded string — use config or env vars", name),
	})
}

func dottedName(e ast.Expression) string {
	ie, ok := e.(*ast.IndexExpression)
	if !ok || !ie.IsDot {
		return ""
	}
	obj, ok := ie.Object.(*ast.Identifier)
	if !ok {
		return ""
	}
	fld, ok := ie.Index.(*ast.StringLiteral)
	if !ok {
		return ""
	}
	return obj.Name + "." + fld.Value
}

func targetName(e ast.Expression) string {
	switch n := e.(type) {
	case *ast.Identifier:
		return n.Name
	case *ast.IndexExpression:
		if n.IsDot {
			if s, ok := n.Index.(*ast.StringLiteral); ok {
				return s.Value
			}
		}
	}
	return ""
}

func fieldKeyName(f ast.TableField) string {
	switch k := f.Key.(type) {
	case *ast.Identifier:
		return k.Name
	case *ast.StringLiteral:
		return k.Value
	}
	return ""
}

func credentialName(name string) bool {
	n := strings.ToLower(name)
	for _, pat := range []string{
		"password", "passwd", "secret", "token", "apikey", "api_key", "access_key",
	} {
		if strings.Contains(n, pat) {
			return true
		}
	}
	return false
}
