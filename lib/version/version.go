package version

import (
	"encoding/json"
	"fmt"
)

// Info describes build version with a semver-complaint version string and
// git-related commit/tree state details.
type Info struct {
	Version      string `json:"version"`
	GitCommit    string `json:"gitCommit"`
	GitTreeState string `json:"gitTreeState"`
}

// Get returns current build version.
func Get() Info {
	return Info{
		Version:      version,
		GitCommit:    gitCommit,
		GitTreeState: gitTreeState,
	}
}

func (r Info) String() string {
	return r.Version
}

// Print prints build version in default format.
func Print() {
	payload, err := json.Marshal(Get())
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s", payload)
}
