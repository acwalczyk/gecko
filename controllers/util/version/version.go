// Package version provides build version information for the gecko-controllers.
// Version values are set at build time via ldflags.
package version

import "os"

// Environment variable for overriding UserAgent
const EnvUserAgent = "HYPERFLEET_USER_AGENT"

// Build-time variables set via ldflags
// Example: go build -ldflags "-X github.com/openshift-online/gecko/controllers/util/version.Version=1.0.0"
var (
	// Version is the semantic version of the controllers
	Version = "0.0.0-dev"

	// Commit is the git commit SHA
	Commit = "none"

	// BuildDate is the date when the binary was built
	BuildDate = "unknown"
)

// UserAgent returns the User-Agent string for HTTP clients.
// It first checks the HYPERFLEET_USER_AGENT environment variable (EnvUserAgent),
// and if not set, returns the default "gecko-controllers/{version}" string.
func UserAgent() string {
	if ua := os.Getenv(EnvUserAgent); ua != "" {
		return ua
	}
	return "gecko-controllers/" + Version
}

// Info returns all version information as a struct
func Info() VersionInfo {
	return VersionInfo{
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
	}
}

// VersionInfo contains all build version information
type VersionInfo struct {
	Version   string
	Commit    string
	BuildDate string
}
