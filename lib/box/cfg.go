package box

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/gravitational/planet/lib/constants"
	"github.com/gravitational/planet/lib/utils"

	"github.com/gravitational/configure/cstrings"
	"github.com/gravitational/trace"
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

	// SocketPath is a path to the socket file for remote command control.
	// Ignored with systemd socket-activation.
	SocketPath string

	// Mounts is a list of device/directory/file mounts passed to the server
	Mounts Mounts
	// Devices is a list of devices to create inside the container
	Devices Devices
	// Capabilities is a list of capabilities of this container
	Capabilities []string
	// DataDir is a directory where libcontainer stores the container state
	DataDir string
}

// ClientConfig defines a configuration to connect to a running box.
type ClientConfig struct {
	Rootfs     string
	SocketPath string
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
	Env  EnvVars   `json:"env"`
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

// Append adds a new environment variable given with k, v and delimiter delim
func (vars *EnvVars) Append(k, v, delim string) {
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

// Set parses v as a comma-separated list of name=value pairs
func (vars *EnvVars) Set(v string) error {
	for _, i := range cstrings.SplitComma(v) {
		if err := vars.setItem(i); err != nil {
			return err
		}
	}
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

// WriteTo writes the environment variables to the specified writer w.
// WriteTo implements io.WriterTo
func (vars EnvVars) WriteTo(w io.Writer) (n int64, err error) {
	cw := &countingWriter{}
	for _, v := range vars {
		// quote value as it may contain spaces
		if _, err := fmt.Fprintf(io.MultiWriter(w, cw), "%v=%q\n", v.Name, v.Val); err != nil {
			return 0, trace.Wrap(err)
		}
	}
	return cw.n, nil
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

// WriteEnvironment writes provided environment variables to a file at the
// specified path.
func WriteEnvironment(path string, env EnvVars) error {
	return utils.SafeWriteFile(path, env, constants.SharedReadMask)
}

// ReadEnvironment returns a list of all environment variables read from the file
// at the specified path.
func ReadEnvironment(path string) (vars EnvVars, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, trace.ConvertSystemError(err)
	}
	defer f.Close()
	return ReadEnvironmentFromReader(f)
}

func ReadEnvironmentFromReader(r io.Reader) (vars EnvVars, err error) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		keyVal := strings.SplitN(scanner.Text(), "=", 2)
		if len(keyVal) != 2 {
			continue
		}
		// the value may be quoted (if the file was previously written by WriteEnvironment above)
		val, err := strconv.Unquote(keyVal[1])
		if err != nil {
			vars.Upsert(keyVal[0], keyVal[1])
		} else {
			vars.Upsert(keyVal[0], val)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, trace.Wrap(err)
	}
	return vars, nil
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

// Write updates the number of bytes written with the length of the specified buffer.
// Write implements io.Writer
func (r *countingWriter) Write(p []byte) (n int, err error) {
	r.n += int64(len(p))
	return len(p), nil
}

// countingWriter is an io.Writer that keeps the internal count of bytes written
type countingWriter struct {
	n int64
}

const (
	devicePath        = "path"
	devicePermissions = "permissions"
	deviceFileMode    = "fileMode"
	deviceUID         = "uid"
	deviceGID         = "gid"
)
