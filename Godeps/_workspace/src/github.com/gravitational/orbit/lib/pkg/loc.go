package pkg

import (
	"encoding/hex"
	"fmt"
	"regexp"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/hashicorp/go-version"
)

// locRe expression specifies the format for package name that
// consists of the repository name and a version separated by the :
var locRe = regexp.MustCompile(`^([a-zA-Z0-9\-_\./]+):([0-9\.]+)$`)

func NewDigest(t string, val []byte) (*Digest, error) {
	if t != "sha256" {
		return nil, trace.Errorf("unsupported digest type: '%v'", t)
	}
	if len(val) == 0 {
		return nil, trace.Errorf("empty hash")
	}
	return &Digest{Type: t, Hash: val}, nil
}

func NewDigestFromHex(t string, val string) (*Digest, error) {
	if t != "sha256" {
		return nil, trace.Errorf("unsupported digest type: '%v'", t)
	}
	b, err := hex.DecodeString(val)
	if len(val) == 0 {
		return nil, trace.Wrap(err, "failed to decode hash string")
	}
	return &Digest{Type: t, Hash: b}, nil
}

type Digest struct {
	Type string
	Hash []byte
}

func (d Digest) Hex() string {
	return fmt.Sprintf("%x", d.Hash)
}

func (d Digest) String() string {
	return fmt.Sprintf("%x", d.Hash)
}

func NewLocator(repo, ver string) (*Locator, error) {
	if repo == "" {
		return nil, trace.Errorf(
			"repository has invalid format, should be just FQDN, e.g. example.com")
	}
	v, err := version.NewVersion(ver)
	if err != nil {
		return nil, trace.Errorf(
			"unsupported version format, need semver format: %v, e.g 1.0.0", err)
	}

	return &Locator{Repo: repo, Ver: ver, SemVer: *v}, nil
}

// Locator is a unique package locator. It consists of the repository name
// and version in the form of sem ver
type Locator struct {
	Repo   string
	Ver    string
	SemVer version.Version
}

func (l *Locator) Set(v string) error {
	p, err := ParseLocator(v)
	if err != nil {
		return err
	}
	l.Repo = p.Repo
	l.Ver = p.Ver
	return nil
}

func (l Locator) String() string {
	return fmt.Sprintf("%v:%v", l.Repo, l.Ver)
}

func ParseLocator(v string) (*Locator, error) {
	m := locRe.FindAllStringSubmatch(v, -1)
	if len(m) != 1 || len(m[0]) != 3 {
		return nil, trace.Errorf(
			"invalid package name, should be path:semver, e.g. example.com/test:1.0.0")
	}
	return NewLocator(m[0][1], m[0][2])
}
