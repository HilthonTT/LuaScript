package version

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
)

const (
	Version   = "0.0.1"
	GitCommit = "unknown"
	BuildTime = "unknown"
)

type Info struct {
	Version    string `json:"version"`
	GitCommit  string `json:"git_commit"`
	BuildTime  string `json:"build_time"`
	GoVersion  string `json:"go_version"`
	GoOS       string `json:"go_os"`
	GoArch     string `json:"go_arch"`
	BuildFlags string `json:"build_flags,omitempty"`
}

func (i Info) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "lotus version %s\n", i.Version)
	fmt.Fprintf(&b, "  Commit:     %s\n", i.GitCommit)
	fmt.Fprintf(&b, "  Go version: %s\n", i.GoVersion)
	fmt.Fprintf(&b, "  Platform:   %s", i.GoOS)
	return b.String()
}

func (i Info) Short() string {
	return i.Version
}

func GetVersionInfo() Info {
	return Info{
		Version:   Version,
		GitCommit: GitCommit,
		BuildTime: BuildTime,
		GoVersion: runtime.Version(),
		GoOS:      runtime.GOOS,
		GoArch:    runtime.GOARCH,
	}
}

func GetVersionString() string {
	return fmt.Sprintf("Version: %s, GitCommit: %s, BuildTime: %s", Version, GitCommit, BuildTime)
}

func GetVersionJSON() string {
	info := GetVersionInfo()
	jsonBytes, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error marshaling version info: %v", err)
	}
	return string(jsonBytes)
}

func IsDev() bool {
	return Version == "dev"
}

func IsDirty() bool {
	return strings.HasSuffix(Version, "-dirty")
}
