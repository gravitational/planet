/*
Copyright 2018 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package box

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/scanner"
	"unicode"

	"github.com/gravitational/configure/cstrings"
	"github.com/gravitational/trace"
)

type Config struct {
	// InitArgs lists the command to execute and any arguments
	InitArgs []string
	// InitEnv lists the environment variables to pass to the process
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

	// Mounts is a list of device/directory/file mounts passed to the server
	Mounts Mounts
	// Devices is a list of devices to create inside the container
	Devices Devices
	// Capabilities is a list of capabilities of this container
	Capabilities []string
	// DataDir is a directory where libcontainer stores the container state
	DataDir string
	// ProcessLabel specifies the SELinux process label
	ProcessLabel string
	// SELinux turns on SELinux support
	SELinux bool
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
	In           io.Reader `json:"-"`
	Out          io.Writer `json:"-"`
	TTY          *TTY      `json:"tty,omitempty"`
	Args         []string  `json:"args"`
	User         string    `json:"user"`
	Env          EnvVars   `json:"env,omitempty"`
	ProcessLabel string    `json:"process_label,omitempty"`
}

// String returns human-readable description of this configuration
func (e *ProcessConfig) String() string {
	var buf bytes.Buffer
	fmt.Fprint(&buf, "ProcessConfig(")
	fmt.Fprintf(&buf, "args=%q,user=%q,env=%v", e.Args, e.User, e.Env)
	if e.ProcessLabel != "" {
		fmt.Fprintf(&buf, ",selinux_domain=%q", e.ProcessLabel)
	}
	fmt.Fprint(&buf, ")")
	return buf.String()
}

// Environment returns a slice of environment variables in key=value
// format as required by libcontainer
func (e *ProcessConfig) Environment() []string {
	if len(e.Env) == 0 {
		return []string{}
	}
	out := []string{}
	for _, keyval := range e.Env {
		out = append(out, fmt.Sprintf("%v=%v", keyval.Name, keyval.Val))
	}
	return out
}

// EnvPair defines an environment variable
type EnvPair struct {
	// Name is the name of the environment variable
	Name string `json:"name"`
	// Val defines the value of the environment variable
	Val string `json:"val"`
}

// EnvVars is a list of environment variables
type EnvVars []EnvPair

// Get returns the value of the environment variable named v
func (vars *EnvVars) Get(v string) string {
	for _, p := range *vars {
		if p.Name == v {
			return p.Val
		}
	}
	return ""
}

// Delete removes the environment variable named v from the list and returns its value
func (vars *EnvVars) Delete(v string) string {
	for i, p := range *vars {
		if p.Name == v {
			*vars = append((*vars)[:i], (*vars)[i+1:]...)
			return p.Val
		}
	}
	return ""
}

// Append adds a new environment variable given with k, v and delimiter delim
func (vars *EnvVars) Append(k, v string) {
	const delim = " "
	if existing := vars.Get(k); existing != "" {
		vars.Upsert(k, strings.Join([]string{existing, v}, delim))
	} else {
		vars.Upsert(k, v)
	}
}

// Upsert adds a new environment variable given with k and v.
// If the environment variable with the name k already exists, it is updated
func (vars *EnvVars) Upsert(k, v string) {
	for i, p := range *vars {
		if p.Name == k {
			(*vars)[i].Val = v
			return
		}
	}
	*vars = append(*vars, EnvPair{Name: k, Val: v})
}

// Set parses v as a comma-separated list of name=value pairs.
// If a value contains a comma, it must be quoted.
func (vars *EnvVars) Set(v string) error {
	p := newEnvParser(v)
	result, err := p.parse()
	if err != nil {
		return trace.Wrap(err)
	}
	*vars = append(*vars, result...)

	return nil
}

// String formats this object as a string with comma-separated list
// of name=value pairs
func (vars *EnvVars) String() string {
	if vars == nil || len(*vars) == 0 {
		return ""
	}
	b := &bytes.Buffer{}
	for i, v := range *vars {
		fmt.Fprintf(b, "%v=%v", v.Name, v.Val)
		if i != len(*vars)-1 {
			fmt.Fprintf(b, ",")
		}
	}
	return b.String()
}

// Environ returns this EnvVars as a list of name=value pairs
func (vars *EnvVars) Environ() (environ []string) {
	if vars == nil || len(*vars) == 0 {
		return nil
	}
	for _, envvar := range *vars {
		environ = append(environ, fmt.Sprintf("%v=%v",
			envvar.Name, envvar.Val))
	}
	return environ
}

func (vars *EnvVars) setItem(input string) error {
	indexEquals := strings.Index(input, "=")
	if indexEquals == -1 {
		return trace.Errorf(
			"set environment variable as name=value")
	}
	*vars = append(*vars, EnvPair{Name: input[:indexEquals], Val: input[indexEquals+1:]})
	return nil
}

// Mount defines a mapping from a host location to some location inside the container
type Mount struct {
	// Src defines the source for the mount on host
	Src string
	// Dst defines the mount point inside the container
	Dst string
	// Readonly specifies that the mount is created readonly
	Readonly bool
	// SkipIfMissing instructs to skip the mount if the Src is non-existent
	SkipIfMissing bool
	// Recursive indicates that all mount points inside this mount should also be mounted
	Recursive bool
}

type Mounts []Mount

func (m *Mounts) Set(v string) error {
	for _, i := range cstrings.SplitComma(v) {
		if err := m.setItem(i); err != nil {
			return err
		}
	}
	return nil
}

func (m *Mounts) setItem(v string) error {
	vals := strings.Split(v, ":")
	if len(vals) < 2 {
		return trace.BadParameter(
			"expected a mount specified as src:dst[:options], but got %q", v)
	}
	mount := Mount{Src: vals[0], Dst: vals[1]}
	if len(vals) > 2 {
		options := vals[2:]
		err := parseMountOptions(options, &mount)
		if err != nil {
			return trace.BadParameter("failed to parse mount options %q", options)
		}
	}
	*m = append(*m, mount)
	return nil
}

func (m *Mounts) String() string {
	if len(*m) == 0 {
		return ""
	}
	b := &bytes.Buffer{}
	for i, v := range *m {
		fmt.Fprintf(b, "%v:%v", v.Src, v.Dst)
		options := formatMountOptions(v)
		if len(options) != 0 {
			fmt.Fprint(b, ":", strings.Join(options, ":"))
		}
		if i != len(*m)-1 {
			fmt.Fprintf(b, " ")
		}
	}
	return b.String()
}

// DNSOverrides is a command-line flag parser for DNS host/zone overrides
type DNSOverrides map[string][]string

// Set sets the overrides value from a CLI flag
func (d *DNSOverrides) Set(v string) error {
	for _, i := range cstrings.SplitComma(v) {
		if err := d.setItem(i); err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

func (d *DNSOverrides) setItem(v string) error {
	vals := strings.Split(v, "/")
	if len(vals) != 2 {
		return trace.BadParameter(
			"expected <host>/<ip> overrides format, got: %q", v)
	}
	(*d)[vals[0]] = append((*d)[vals[0]], vals[1])
	return nil
}

// String formats overrides to a string
func (d *DNSOverrides) String() string {
	if len(*d) == 0 {
		return ""
	}
	var s []string
	for key, values := range *d {
		for _, value := range values {
			s = append(s, fmt.Sprintf("%v/%v", key, value))
		}
	}
	return strings.Join(s, ",")
}

func formatMountOptions(mount Mount) (options []string) {
	if mount.SkipIfMissing {
		options = append(options, "skip")
	}
	if mount.Readonly {
		options = append(options, "ro")
	}
	if mount.Recursive {
		options = append(options, "rec")
	}
	return options
}

func parseMountOptions(options []string, mount *Mount) error {
	for _, option := range options {
		switch option {
		case "r", "ro":
			mount.Readonly = true
		case "rw", "w":
			mount.Readonly = false
		case "skip":
			mount.SkipIfMissing = true
		case "rec", "recursive":
			mount.Recursive = true
		default:
			return trace.BadParameter("unknown option %q", option)
		}
	}
	return nil
}

// Device represents a device that should be created in planet
type Device struct {
	// Path is the device path, treated as a glob
	Path string
	// Permissions is the device permissions
	Permissions string
	// FileMode is the device file mode
	FileMode os.FileMode
	// UID is the device user ID
	UID uint32
	// GID is the device group ID
	GID uint32
}

// Format formats the device to a string
func (d Device) Format() string {
	parts := []string{fmt.Sprintf("%v=%v", devicePath, d.Path)}
	if d.Permissions != "" {
		parts = append(parts, fmt.Sprintf("%v=%v", devicePermissions, d.Permissions))
	}
	if d.FileMode != 0 {
		parts = append(parts, fmt.Sprintf("%v=0%o", deviceFileMode, d.FileMode))
	}
	if d.UID != 0 {
		parts = append(parts, fmt.Sprintf("%v=%v", deviceUID, d.UID))
	}
	if d.GID != 0 {
		parts = append(parts, fmt.Sprintf("%v=%v", deviceGID, d.GID))
	}
	return strings.Join(parts, ";")
}

// Devices represents a list of devices
type Devices []Device

// Set sets the devices from CLI flags
func (d *Devices) Set(v string) error {
	for _, i := range cstrings.SplitComma(v) {
		if err := d.setItem(i); err != nil {
			return err
		}
	}
	return nil
}

func (d *Devices) setItem(v string) error {
	device, err := parseDevice(v)
	if err != nil {
		return trace.Wrap(err)
	}
	*d = append(*d, *device)
	return nil
}

// String converts devices to a string
func (d *Devices) String() string {
	if len(*d) == 0 {
		return ""
	}
	var formats []string
	for _, device := range *d {
		formats = append(formats, device.Format())
	}
	return strings.Join(formats, ";")
}

// parseDevice parses a single device value in the format:
// path=/dev/nvidia*;permissions=rwm;fileMode=0666
func parseDevice(value string) (*Device, error) {
	device := &Device{}
	for _, part := range strings.Split(value, ";") {
		kv := strings.Split(part, "=")
		if len(kv) != 2 {
			return nil, trace.BadParameter("malformed device format: %q", value)
		}
		switch kv[0] {
		case devicePath:
			device.Path = kv[1]
		case devicePermissions:
			device.Permissions = kv[1]
		case deviceFileMode:
			fileMode, err := strconv.ParseUint(kv[1], 8, 32)
			if err != nil {
				return nil, trace.Wrap(err)
			}
			device.FileMode = os.FileMode(fileMode)
		case deviceUID:
			uid, err := strconv.ParseUint(kv[1], 0, 32)
			if err != nil {
				return nil, trace.Wrap(err)
			}
			device.UID = uint32(uid)
		case deviceGID:
			gid, err := strconv.ParseUint(kv[1], 0, 32)
			if err != nil {
				return nil, trace.Wrap(err)
			}
			device.GID = uint32(gid)
		}
	}
	return device, nil
}

const (
	devicePath        = "path"
	devicePermissions = "permissions"
	deviceFileMode    = "fileMode"
	deviceUID         = "uid"
	deviceGID         = "gid"
)

// newEnvParser returns a new parser for environment.
// input is expected to be a comma-separated list of name=value pairs.
// If a value contains a comma, it must be quoted.
func newEnvParser(input string) *envParser {
	var s scanner.Scanner
	s.Init(strings.NewReader(input))
	s.Mode = scanner.ScanIdents | scanner.ScanStrings
	s.IsIdentRune = isIdentRune
	return &envParser{s: s}
}

func (r *envParser) parse() (result EnvVars, err error) {
	mode := envKey
	var name string
	for tok := r.s.Scan(); tok != scanner.EOF; tok = r.s.Scan() {
		tokenText := r.s.TokenText()
		switch mode {
		case envKey:
			if tok != scanner.Ident {
				return nil, trace.BadParameter("expected environment variable but got %v", tokenText)
			}
			name = tokenText
			mode = envEquals
		case envEquals:
			if tok != '=' {
				return nil, trace.BadParameter("expected '=' but got %q", tokenText)
			}
			mode = envValue
		case envValue:
			v := tokenText
			if tok == scanner.String {
				v = tokenText[1 : len(tokenText)-1]
			}
			result = append(result, EnvPair{Name: name, Val: v})
			name = ""
			mode = envKey
			r.moveNextVariable()
		}
	}
	if mode != envKey {
		return nil, trace.BadParameter("unexpected token %v", r.s.TokenText())
	}
	return result, nil
}

func (r *envParser) moveNextVariable() {
	if r.s.Peek() == ',' {
		r.s.Next()
	}
}

// isIdentRune determines whether ch can be part of the environment variable name.
// It loosens the below definition by allowing lower-case letters.
// See: http://pubs.opengroup.org/onlinepubs/000095399/basedefs/xbd_chap08.html for reference
func isIdentRune(ch rune, i int) bool {
	if ch == '=' {
		return false
	}
	return ch == '_' || unicode.IsLetter(ch) || unicode.IsDigit(ch) && i > 0
}

type envParserMode byte

const (
	envKey envParserMode = iota
	envEquals
	envValue
)

// envParser implements parsing of environment variables
// specified a comma-separated list of name=value pairs.
// values can contain commas but must be quoted in this case.
type envParser struct {
	s scanner.Scanner
}
