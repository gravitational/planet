package version

// Version information
type Info struct {
	Version      string `json:"version"`
	GitCommit    string `json:"gitCommit"`
	GitTreeState string `json:"gitTreeState"`
}

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
