package version

import (
	"fmt"
	"runtime"
)

var (
	// Version is the semantic release version.
	Version = "0.1.0-dev"
	// GitCommit is the SHA1 commit hash injected during build.
	GitCommit = "unknown"
	// BuildDate is the ISO-8601 build timestamp.
	BuildDate = "unknown"
)

// Info encapsulates all build and runtime version metadata.
type Info struct {
	Version   string `json:"version"`
	GitCommit string `json:"gitCommit"`
	BuildDate string `json:"buildDate"`
	GoVersion string `json:"goVersion"`
	Compiler  string `json:"compiler"`
	Platform  string `json:"platform"`
}

// Get returns the complete build information.
func Get() Info {
	return Info{
		Version:   Version,
		GitCommit: GitCommit,
		BuildDate: BuildDate,
		GoVersion: runtime.Version(),
		Compiler:  runtime.Compiler,
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// String returns a formatted version string.
func (i Info) String() string {
	return fmt.Sprintf("Aurora v%s (commit: %s, built: %s, %s)", i.Version, i.GitCommit, i.BuildDate, i.Platform)
}
