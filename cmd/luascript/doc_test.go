package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestDocsMatchRuntime(t *testing.T) {
	undocumented, missing := auditDocs()
	for _, name := range undocumented {
		t.Errorf("undocumented at runtime: %s — add it to internal/docs", name)
	}
	for _, name := range missing {
		t.Errorf("documented but absent at runtime: %s — remove it from internal/docs", name)
	}
}

func TestRunDocIndex(t *testing.T) {
	out, code := captureDoc(t, nil)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	for _, want := range []string{"MODULES (require)", "json", "std.stack"} {
		if !strings.Contains(out, want) {
			t.Errorf("index missing %q", want)
		}
	}
}

func TestRunDocTopic(t *testing.T) {
	out, code := captureDoc(t, []string{"crypto"})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out, "crypto.sha256(s): string") {
		t.Errorf("crypto page missing sha256:\n%s", out)
	}
}

func TestRunDocEntryBothSpellings(t *testing.T) {
	dotted, code := captureDoc(t, []string{"string.format"})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	spaced, code := captureDoc(t, []string{"string", "format"})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if dotted != spaced {
		t.Error("`doc string.format` and `doc string format` should render the same page")
	}
	if !strings.Contains(dotted, "printf") {
		t.Errorf("string.format page missing its description:\n%s", dotted)
	}
}

func TestRunDocSearch(t *testing.T) {
	out, code := captureDoc(t, []string{"-k", "base64"})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out, "crypto.base64_encode") {
		t.Errorf("search missed crypto.base64_encode:\n%s", out)
	}
}

func TestRunDocList(t *testing.T) {
	out, code := captureDoc(t, []string{"-list"})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out, "\nndarray\n") {
		t.Errorf("-list should print bare topic names, got:\n%s", out)
	}
}

func TestRunDocUnknownTopicFails(t *testing.T) {
	_, code := captureDoc(t, []string{"definitely-not-a-topic"})
	if code != 1 {
		t.Errorf("exit code = %d, want 1 for an unknown topic", code)
	}
}

func TestRunDocEmptySearchFails(t *testing.T) {
	if _, code := captureDoc(t, []string{"-k", "zzzznope"}); code != 1 {
		t.Errorf("exit code = %d, want 1 when a search matches nothing", code)
	}
}

func captureDoc(t *testing.T, argv []string) (string, int) {
	t.Helper()
	t.Setenv("NO_COLOR", "1")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	saved := os.Stdout
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	code := runDoc(argv)

	os.Stdout = saved
	w.Close()
	out := <-done
	r.Close()
	return out, code
}
