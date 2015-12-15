package version

// Version value defaults
var (
	// Version string, a slightly modified version of `git describe` to be semver-complaint
	version      string = "v0.0.0-master+$Format:%h$"
	gitCommit    string = "$Format:%H$"    // sha1 from git, output of $(git rev-parse HEAD)
	gitTreeState string = "not a git tree" // state of git tree, either "clean" or "dirty"
)
