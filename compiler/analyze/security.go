package analyze

import (
	"fmt"
	"strings"

	"github.com/hilthontt/luascript/compiler/ast"
)

// securityPass flags a small set of risky patterns. It is the.lsc-AST
// rewrite of the original ASTAnalyzer's SecurityAnalysisPass.
type securityPass struct{}

func (securityPass) Name() string {
	return "security"
}

func (securityPass) Run(prog *ast.Program, _ Options, rep *Report) {
	w := &walker{}
	w.onExpr = func(e ast.Expression) {
		switch n := e.(type) {
		case *ast.CallExpression:
			// crypto.md5(...) / crypto.sha1(...)
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

// checkCredential reports a credential-looking binding whose value is a
// non-empty string literal.
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

// dottedName renders a dotted index like `crypto.md5` into "crypto.md5", or
// "" when e is not an identifier.field form.
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

// targetName extracts the bound name from an assignment target.
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

// fieldKeyName extracts a table field's key name, for record (`name = v`) and
// bracketed-string (`["name"] = v`) entries.
func fieldKeyName(f ast.TableField) string {
	switch k := f.Key.(type) {
	case *ast.Identifier:
		return k.Name
	case *ast.StringLiteral:
		return k.Value
	}
	return ""
}

// credentialName reports whether name looks like it holds a secret.
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
