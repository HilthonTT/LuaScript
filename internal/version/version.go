package version

import "fmt"

const (
	Version   = "2.0.0"
	GitCommit = "2185d6c"
	BuildTime = "unknown"
)

func GetVersionString() string {
	return fmt.Sprintf("Version: %s, GitCommit: %s, BuildTime: %s", Version, GitCommit, BuildTime)
}
