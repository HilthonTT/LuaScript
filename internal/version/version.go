package version

import "fmt"

const (
	Version   = "0.0.1"
	GitCommit = "e59e261"
	BuildTime = "unknown"
)

func GetVersionString() string {
	return fmt.Sprintf("Version: %s, GitCommit: %s, BuildTime: %s", Version, GitCommit, BuildTime)
}
