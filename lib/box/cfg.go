package box

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/orbit/lib/utils"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
)

type Config struct {
	//InitArgs list of arguments to exec as an init process
	InitArgs []string
	// InitEnv list of env variables to pass to executable
	InitEnv []string
	// InitUser is a user running the init process
	InitUser string

	// EnvFiles has a list of files that will generated when process starts
	EnvFiles []EnvFile
	// Files is an optional list of files that will be placed
	// in the container when started
	Files []File

	// Rootfs is a root filesystem of the container
	Rootfs string

	// Mounts is a list of device moutns passed to the server
	Mounts Mounts
	// Capabilities is a list of capabilities of this container
	Capabilities []string
	// DataDir is a directory where libcontainer stores the container state
	DataDir string
}

// File is a file that will be placed in the container before start
type File struct {
	Path     string
	Contents io.Reader
	Mode     os.FileMode
	Owners   *FileOwner
}

type FileOwner struct {
	UID int
	GID int
}

// environment file to write when container starts
type EnvFile struct {
	Path string
	Env  EnvVars
}

// TTY is a tty settings passed to the device when allocating terminal
type TTY struct {
	W int
	H int
}

// ProcessConfig is a configuration passed to the process started
// in the namespace of the container
type ProcessConfig struct {
	In   io.Reader `json:"-"`
	Out  io.Writer `json:"-"`
	TTY  *TTY      `json:"tty"`
	Args []string  `json:"args"`
	User string    `json:"user"`
}

type EnvPair struct {
	Name string
	Val  string
}

type EnvVars []EnvPair

func (vars *EnvVars) Get(v string) string {
	for _, p := range *vars {
		if p.Name == v {
			return p.Val
		}
	}
	return ""
}

func (vars *EnvVars) Upsert(k, v string) {
	for i, p := range *vars {
		if p.Name == v {
			(*vars)[i].Val = v
		}
	}
	*vars = append(*vars, EnvPair{Name: k, Val: v})
}

func (vars *EnvVars) Set(v string) error {
	for _, i := range utils.SplitComma(v) {
		if err := vars.setItem(i); err != nil {
			return err
		}
	}
	return nil
}

func (vars *EnvVars) setItem(v string) error {
	vals := strings.Split(v, ":")
	if len(vals) != 2 {
		return trace.Errorf(
			"set environment variable separated by ':', e.g. KEY:VAL")
	}
	*vars = append(*vars, EnvPair{Name: vals[0], Val: vals[1]})
	return nil
}

func (vars *EnvVars) String() string {
	if len(*vars) == 0 {
		return ""
	}
	b := &bytes.Buffer{}
	for i, v := range *vars {
		fmt.Fprintf(b, "%v=%v", v.Name, v.Val)
		if i != len(*vars)-1 {
			fmt.Fprintf(b, " ")
		}
	}
	return b.String()
}

type Mount struct {
	Src      string
	Dst      string
	Readonly bool
}

type Mounts []Mount

func (m *Mounts) Set(v string) error {
	for _, i := range utils.SplitComma(v) {
		if err := m.setItem(i); err != nil {
			return err
		}
	}
	return nil
}

func (m *Mounts) setItem(v string) error {
	vals := strings.Split(v, ":")
	if len(vals) != 2 {
		return trace.Errorf(
			"set mounts separated by : e.g. src:dst")
	}
	*m = append(*m, Mount{Src: vals[0], Dst: vals[1]})
	return nil
}

func (m *Mounts) String() string {
	if len(*m) == 0 {
		return ""
	}
	b := &bytes.Buffer{}
	for i, v := range *m {
		fmt.Fprintf(b, "%v:%v", v.Src, v.Dst)
		if i != len(*m)-1 {
			fmt.Fprintf(b, " ")
		}
	}
	return b.String()
}
