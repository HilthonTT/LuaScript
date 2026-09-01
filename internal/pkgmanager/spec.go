package pkgmanager

import (
	"fmt"
	"strings"
)

type Spec struct {
	Source string
	Ref    string
}

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

func (s Spec) String() string {
	if s.Ref == "" {
		return s.Source
	}
	return s.Source + "@" + s.Ref
}

func (s Spec) DefaultName() string {
	seg := s.Source
	if i := strings.LastIndex(seg, "/"); i >= 0 {
		seg = seg[i+1:]
	}
	return strings.TrimSuffix(seg, ".git")
}

func (s Spec) CloneURL() string {
	return "https://" + s.Source + ".git"
}
