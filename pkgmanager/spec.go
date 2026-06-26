package pkgmanager

import (
	"fmt"
	"strings"
)

// Spec is a parsed dependency source: a `host/path` with an optional ref
// (tag, branch, or commit) after `@`. Example inputs:
//
//	github.com/alice/router            -> Source=github.com/alice/router, Ref=""
//	github.com/alice/router@v1.2.0     -> Ref="v1.2.0"
//	gitlab.com/team/lib@main           -> Ref="main"
//
// A bare `@` or empty source is rejected. Schemes (`https://`, `git@...`) are
// intentionally not accepted here: the source is a stable identity, and the
// clone URL is derived from it via CloneURL so the same package always
// resolves to the same lockfile key.
type Spec struct {
	Source string // host/path, no scheme, no ref
	Ref    string // tag/branch/commit, or "" for the default branch
}

// ParseSpec splits a `host/path[@ref]` string into a Spec.
func ParseSpec(s string) (Spec, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Spec{}, fmt.Errorf("empty package spec")
	}
	if strings.Contains(s, "://") || strings.HasPrefix(s, "git@") {
		return Spec{}, fmt.Errorf("spec %q must be a host/path (e.g. github.com/user/repo@v1), not a URL", s)
	}
	source, ref := s, ""
	if at := strings.LastIndex(s, "@"); at >= 0 {
		source, ref = s[:at], s[at+1:]
	}
	source = strings.Trim(source, "/")
	if source == "" {
		return Spec{}, fmt.Errorf("spec %q has no package path", s)
	}
	if !strings.Contains(source, "/") {
		return Spec{}, fmt.Errorf("spec %q must include a path (e.g. github.com/user/repo)", s)
	}
	return Spec{Source: source, Ref: ref}, nil
}

// String renders the spec back to `host/path[@ref]` form — the exact value
// stored as a manifest dependency.
func (s Spec) String() string {
	if s.Ref == "" {
		return s.Source
	}
	return s.Source + "@" + s.Ref
}

// DefaultName derives the package's local name (the `require` name) from the
// last path segment, with a trailing `.git` stripped if present.
func (s Spec) DefaultName() string {
	seg := s.Source
	if i := strings.LastIndex(seg, "/"); i >= 0 {
		seg = seg[i+1:]
	}
	return strings.TrimSuffix(seg, ".git")
}

// CloneURL is the https git URL derived from the source. We always clone over
// https so no SSH keys are required for public packages.
func (s Spec) CloneURL() string {
	return "https://" + s.Source + ".git"
}
