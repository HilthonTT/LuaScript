package version

import (
	"fmt"
	"runtime/debug"
	"sync"
)

// Version is the language/interpreter release. It is a constant because
// bundled binaries (`luascript build`) compare it against the stub they run on.
const Version = "3.0.0"

// GitCommit and BuildTime are stamped at link time:
//
//	go build -ldflags "-X github.com/hilthontt/luascript/v3/internal/version.GitCommit=$(git rev-parse --short HEAD) \
//	                   -X github.com/hilthontt/luascript/v3/internal/version.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" ./cmd/luascript
//
// When left empty, Commit and Built fall back to the VCS stamp the Go
// toolchain embeds (`go build` inside a git checkout), then to "unknown".
var (
	GitCommit = ""
	BuildTime = ""
)

var (
	resolveOnce                  sync.Once
	resolvedCommit, resolvedTime string
)

func resolve() {
	resolvedCommit, resolvedTime = GitCommit, BuildTime
	if resolvedCommit != "" && resolvedTime != "" {
		return
	}
	var info *debug.BuildInfo
	if bi, ok := debug.ReadBuildInfo(); ok {
		info = bi
	}
	commit, when := fromBuildInfo(info)
	if resolvedCommit == "" {
		resolvedCommit = commit
	}
	if resolvedTime == "" {
		resolvedTime = when
	}
}

// fromBuildInfo extracts a short commit (suffixed "-dirty" for a modified
// tree) and the commit time from the toolchain's VCS settings.
func fromBuildInfo(info *debug.BuildInfo) (commit, when string) {
	commit, when = "unknown", "unknown"
	if info == nil {
		return
	}
	modified := false
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if s.Value != "" {
				commit = s.Value
				if len(commit) > 7 {
					commit = commit[:7]
				}
			}
		case "vcs.time":
			if s.Value != "" {
				when = s.Value
			}
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	if modified && commit != "unknown" {
		commit += "-dirty"
	}
	return
}

// Commit is the git commit the binary was built from, or "unknown".
func Commit() string {
	resolveOnce.Do(resolve)
	return resolvedCommit
}

// Built is when the binary was built (ldflags) or when its commit was made
// (VCS fallback), or "unknown".
func Built() string {
	resolveOnce.Do(resolve)
	return resolvedTime
}

func GetVersionString() string {
	return fmt.Sprintf("Version: %s, GitCommit: %s, BuildTime: %s", Version, Commit(), Built())
}
