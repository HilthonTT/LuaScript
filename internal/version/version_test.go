package version

import (
	"runtime/debug"
	"strings"
	"testing"
)

func TestFromBuildInfo(t *testing.T) {
	commit, when := fromBuildInfo(nil)
	if commit != "unknown" || when != "unknown" {
		t.Fatalf("nil build info = %q, %q", commit, when)
	}
	info := &debug.BuildInfo{Settings: []debug.BuildSetting{
		{Key: "vcs.revision", Value: "936ac38f0123456789abcdef"},
		{Key: "vcs.time", Value: "2026-10-01T12:00:00Z"},
		{Key: "vcs.modified", Value: "true"},
	}}
	commit, when = fromBuildInfo(info)
	if commit != "936ac38-dirty" || when != "2026-10-01T12:00:00Z" {
		t.Fatalf("got %q, %q", commit, when)
	}
}

func TestVersionStringIsNotStale(t *testing.T) {
	s := GetVersionString()
	if !strings.HasPrefix(s, "Version: "+Version+", GitCommit: ") {
		t.Fatalf("unexpected format %q", s)
	}
	if strings.Contains(s, "2185d6c") {
		t.Fatalf("version string still carries the old hardcoded commit: %q", s)
	}
}
