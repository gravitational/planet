// package pkg defines packaging format used by orbit
package pkg

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/docker/docker/pkg/archive"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/orbit/lib/configure"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
)

type Manifest struct {
	Version  string            `json:"version"`
	Config   *configure.Config `json:"config,omitempty"`
	Commands []Command         `json:"commands,omitempty"`
	Labels   []Label           `json:"labels,omitempty"`
}

type Label struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func (m *Manifest) Label(name string) string {
	if len(m.Labels) == 0 {
		return ""
	}
	for _, l := range m.Labels {
		if l.Name == name {
			return l.Value
		}
	}
	return ""
}

func (m *Manifest) NeedsConfig() bool {
	return m.Config != nil
}

func (m *Manifest) EncodeJSON() ([]byte, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return b, nil
}

func (m *Manifest) Command(name string) (*Command, error) {
	if len(m.Commands) == 0 {
		return nil, trace.Errorf("command %v is not found", name)
	}
	for _, c := range m.Commands {
		if c.Name == name {
			return &c, nil
		}
	}
	return nil, trace.Errorf("command %v is not found", name)
}

const Version = "0.0.1"

type manifestJSON struct {
	Version  string          `json:"version"`
	Config   json.RawMessage `json:"config,omitempty"`
	Commands []Command       `json:"commands,omitempty"`
	Labels   []Label         `json:"labels,omitempty"`
}

type Command struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Args        []string `json:"args"`
}

func Tar(path string) (io.ReadCloser, error) {
	f, err := os.Open(filepath.Join(path, "orbit.manifest.json"))
	if err != nil && !os.IsNotExist(err) {
		return nil, trace.Wrap(err, "failed to open manifest")
	}
	defer f.Close()
	if _, err := ParseManifestJSON(f); err != nil {
		return nil, err
	}
	return archive.TarWithOptions(
		path, &archive.TarOptions{Compression: archive.Gzip})
}

func Untar(r io.Reader, target string) (*Manifest, error) {
	if err := archive.Untar(
		r, target, &archive.TarOptions{NoLchown: true}); err != nil {
		return nil, trace.Wrap(err)
	}
	f, err := os.Open(filepath.Join(target, "orbit.manifest.json"))
	if err != nil && !os.IsNotExist(err) {
		return nil, trace.Wrap(err, "failed to open manifest")
	}
	defer f.Close()
	return ParseManifestJSON(f)
}

func OpenManifest(dir string) (*Manifest, error) {
	f, err := os.Open(filepath.Join(dir, "orbit.manifest.json"))
	if err != nil {
		return nil, trace.Wrap(err, "failed to open manifest")
	}
	defer f.Close()
	return ParseManifestJSON(f)
}

func ParseManifestJSON(r io.Reader) (*Manifest, error) {
	var j *manifestJSON
	if err := json.NewDecoder(r).Decode(&j); err != nil {
		return nil, trace.Wrap(err)
	}
	if j.Version != Version {
		return nil, trace.Errorf("unsupported version: %v", j.Version)
	}
	m := &Manifest{
		Version: j.Version,
	}
	if len(j.Config) != 0 {
		c, err := configure.ParseJSON(bytes.NewReader(j.Config))
		if err != nil {
			return nil, err
		}
		m.Config = c
	}

	seen := map[string]bool{}
	if len(j.Commands) != 0 {
		for _, c := range j.Commands {
			if err := checkWord(c.Name); err != nil {
				return nil, err
			}
			if seen[c.Name] {
				return nil, trace.Errorf(
					"command '%v' already defined",
					c.Name)
			}
			seen[c.Name] = true
			if len(c.Args) == 0 {
				return nil, trace.Errorf(
					"please supply at least one argument for command '%v'",
					c.Name)
			}
		}
	}
	m.Commands = j.Commands

	if len(j.Labels) != 0 {
		for _, c := range j.Labels {
			if err := checkWord(c.Name); err != nil {
				return nil, err
			}
		}
	}
	m.Labels = j.Labels
	return m, nil
}

func checkWord(val string) error {
	if !regexp.MustCompile("^[a-zA-z][a-zA-Z0-9_-]*$").MatchString(val) {
		return trace.Errorf("unlike '%v', workd can start with letter and contain letters, numbers, underscore and dash", val)
	}
	return nil
}
