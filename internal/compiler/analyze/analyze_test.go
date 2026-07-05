package analyze

import (
	"strings"
	"testing"
)

func analyzeOK(t *testing.T, src string, opts Options) *Report {
	t.Helper()
	rep, err := Analyze(src, opts)
	if err != nil {
		t.Fatalf("Analyze error: %v", err)
	}
	return rep
}

func rules(rep *Report) []string {
	var rs []string
	for _, f := range rep.Findings {
		rs = append(rs, f.Rule)
	}
	return rs
}

func hasRule(rep *Report, rule string) bool {
	for _, f := range rep.Findings {
		if f.Rule == rule {
			return true
		}
	}
	return false
}

func TestCleanFileNoFindings(t *testing.T) {
	src := "local function add(a, b)\n  return a + b\nend\nprint(add(1, 2))\n"
	rep := analyzeOK(t, src, Options{})
	if len(rep.Findings) != 0 {
		t.Errorf("clean file produced findings: %v", rules(rep))
	}
	if rep.Metrics.Functions != 1 {
		t.Errorf("want 1 function, got %d", rep.Metrics.Functions)
	}
}

func TestHighComplexity(t *testing.T) {
	src := `local function f(n)
  if n == 1 then return 1 end
  if n == 2 then return 2 end
  if n == 3 then return 3 end
  return 0
end
print(f(1))
`
	rep := analyzeOK(t, src, Options{MaxComplexity: 2})
	if !hasRule(rep, "high-complexity") {
		t.Errorf("want high-complexity finding, got %v", rules(rep))
	}
	if rep.Metrics.MaxComplexity < 4 {
		t.Errorf("want MaxComplexity >= 4, got %d", rep.Metrics.MaxComplexity)
	}
	// At the default threshold the same file is clean of complexity flags.
	rep2 := analyzeOK(t, src, Options{})
	if hasRule(rep2, "high-complexity") {
		t.Errorf("unexpected high-complexity at default threshold")
	}
}

func TestUnusedLocal(t *testing.T) {
	rep := analyzeOK(t, "local unused = 5\n", Options{})
	if !hasRule(rep, "unused-local") {
		t.Errorf("want unused-local, got %v", rules(rep))
	}
	// An underscore-prefixed local is intentionally unused — not flagged.
	rep2 := analyzeOK(t, "local _scratch = 5\n", Options{})
	if hasRule(rep2, "unused-local") {
		t.Errorf("underscore local should not be flagged")
	}
}

func TestShadowing(t *testing.T) {
	src := "local x = 1\ndo\n  local x = 2\n  print(x)\nend\nprint(x)\n"
	rep := analyzeOK(t, src, Options{})
	if !hasRule(rep, "shadowing") {
		t.Errorf("want shadowing, got %v", rules(rep))
	}
}

func TestUnreachableCode(t *testing.T) {
	src := "while true do\n  break\n  print(\"dead\")\nend\n"
	rep := analyzeOK(t, src, Options{})
	if !hasRule(rep, "unreachable-code") {
		t.Errorf("want unreachable-code, got %v", rules(rep))
	}
}

func TestWeakCrypto(t *testing.T) {
	rep := analyzeOK(t, "print(crypto.md5(\"data\"))\n", Options{})
	if !hasRule(rep, "weak-crypto") {
		t.Errorf("want weak-crypto, got %v", rules(rep))
	}
}

func TestPlaintextHTTP(t *testing.T) {
	rep := analyzeOK(t, "print(\"http://example.com\")\n", Options{})
	if !hasRule(rep, "plaintext-http") {
		t.Errorf("want plaintext-http, got %v", rules(rep))
	}
}

func TestHardcodedCredential(t *testing.T) {
	rep := analyzeOK(t, "local password = \"hunter2\"\nprint(password)\n", Options{})
	if !hasRule(rep, "hardcoded-credential") {
		t.Errorf("want hardcoded-credential, got %v", rules(rep))
	}
	// Table-field form.
	rep2 := analyzeOK(t, "local cfg = { api_key = \"sk-12345\" }\nprint(cfg)\n", Options{})
	if !hasRule(rep2, "hardcoded-credential") {
		t.Errorf("want hardcoded-credential for table field, got %v", rules(rep2))
	}
}

func TestParseErrorReturned(t *testing.T) {
	_, err := Analyze("local = = =\n", Options{})
	if err == nil {
		t.Errorf("want parse error, got nil")
	}
}

func TestReportString(t *testing.T) {
	rep := analyzeOK(t, "local unused = 5\n", Options{})
	out := rep.String()
	if !strings.Contains(out, "metrics:") || !strings.Contains(out, "unused-local") {
		t.Errorf("report missing expected sections:\n%s", out)
	}
}
