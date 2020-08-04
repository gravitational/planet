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

package utils

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode"

	"github.com/gravitational/planet/lib/constants"

	"github.com/ghodss/yaml"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

// WriteHosts formats entries in hosts file format to writer
func WriteHosts(writer io.Writer, entries []HostEntry) error {
	for _, entry := range entries {
		line := fmt.Sprintf("%v %v", entry.IP, entry.Hostnames)
		if _, err := io.WriteString(writer, line+"\n"); err != nil {
			return trace.ConvertSystemError(err)
		}
	}
	return nil
}

// HostEntry maps a list of hostnames to an IP
type HostEntry struct {
	// Hostnames is a list of space separated hostnames
	Hostnames string
	// IP is the IP the hostnames should resolve to
	IP string
}

// WriteDropIn creates the file specified with dropInPath in directory specified with dropInDir
// with given contents
func WriteDropIn(dropInDir, dropInFile string, contents []byte) error {
	err := os.MkdirAll(dropInDir, constants.SharedDirMask)
	if err != nil {
		return trace.ConvertSystemError(err)
	}

	dropInPath := filepath.Join(dropInDir, dropInFile)
	err = ioutil.WriteFile(dropInPath, contents, constants.SharedReadMask)
	if err != nil {
		return trace.ConvertSystemError(err)
	}

	return nil
}

// DropInDir returns the name of the directory for the specified unit
func DropInDir(unit string) string {
	return fmt.Sprintf("%v.d", unit)
}

// SafeWriteFile is similar to ioutil.WriteFile, but operates by writing to a temporary file first
// and then relinking the file into the filename which should be an atomic operation. This should be
// safer, that if the destination file is being replaced, it won't be left in a partial written state.
func SafeWriteFile(filename string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(filename)

	tmpFile, err := ioutil.TempFile(dir, "safewrite")
	if err != nil {
		return trace.Wrap(err)
	}
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.Write(data)
	if err != nil {
		return trace.Wrap(err)
	}

	err = os.Chmod(tmpFile.Name(), perm)
	if err != nil {
		return trace.Wrap(err)
	}

	err = os.Rename(tmpFile.Name(), filename)
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}

// BoolPtr returns a pointer to a bool with value v
func BoolPtr(v bool) *bool {
	return &v
}

// Int32Ptr returns a pointer to an int64 with value v
func Int32Ptr(v int32) *int32 {
	return &v
}

// ExitStatusFromError returns the exit status from the specified error.
// If the error is not exit status error, return nil
func ExitStatusFromError(err error) *int {
	exitErr, ok := trace.Unwrap(err).(*exec.ExitError)
	if !ok {
		return nil
	}
	if waitStatus, ok := exitErr.ProcessState.Sys().(syscall.WaitStatus); ok {
		status := waitStatus.ExitStatus()
		return &status
	}
	return nil
}

// ToJSON converts a single YAML document into a JSON document
// or returns an error. If the document appears to be JSON the
// YAML decoding path is not used (so that error messages are
// JSON specific).
// Creds to: k8s.io for the code
func ToJSON(data []byte) ([]byte, error) {
	if hasJSONPrefix(data) {
		return data, nil
	}
	return yaml.YAMLToJSON(data)
}

// CopyFile copies contents of src to dst atomically
// using SharedReadWriteMask as permissions.
func CopyFile(dst, src string) error {
	return CopyFileWithPerms(dst, src, constants.SharedReadWriteMask)
}

// CopyDirContents copies all contents of the source directory to the destination
// directory
func CopyDirContents(srcDir, dstDir string) error {
	// create dest directory if it doesn't exist
	err := os.MkdirAll(dstDir, constants.SharedDirMask)
	if err != nil {
		return trace.Wrap(err)
	}
	srcDir = filepath.Clean(srcDir)
	err = filepath.Walk(srcDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return trace.Wrap(err)
		}

		// ignore root directory
		if path == srcDir {
			return nil
		}

		if fi.IsDir() {
			// create directory for the target file
			targetDir := filepath.Join(dstDir, strings.TrimPrefix(path, srcDir))
			err = os.MkdirAll(targetDir, constants.SharedDirMask)
			if err != nil {
				return trace.ConvertSystemError(err)
			}
			// copy sub-directories recursively
			return nil
		}

		relativePath := strings.TrimPrefix(filepath.Dir(path), srcDir)
		targetDir := filepath.Join(dstDir, relativePath)

		// copy file, preserve permissions
		toFileName := filepath.Join(targetDir, filepath.Base(fi.Name()))
		err = CopyFileWithPerms(toFileName, path, fi.Mode())
		if err != nil {
			return trace.Wrap(err)
		}
		return nil
	})
	return trace.Wrap(err)
}

// CopyFileWithPerms copies the contents from src to dst atomically.
// Uses CopyReaderWithPerms for its implementation - see function documentation
// for details of operation
func CopyFileWithPerms(dst, src string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return trace.ConvertSystemError(err)
	}
	defer in.Close()
	return CopyReaderWithPerms(dst, in, perm)
}

// CopyReaderWithPerms copies the contents from src to dst atomically.
// If dst does not exist, CopyReaderWithPerms creates it with permissions perm.
// If the copy fails, CopyReaderWithPerms aborts and dst is preserved.
// Adopted with modifications from https://go-review.googlesource.com/#/c/1591/9/src/io/ioutil/ioutil.go
func CopyReaderWithPerms(dst string, src io.Reader, perm os.FileMode) error {
	return CopyReaderWithOptions(dst, src, WithFileMode(perm))
}

// CopyReaderWithOptions copies the contents from src to dst atomically.
// If dst does not exist, CopyReaderWithOptions creates it.
// Callers choose the options to apply on the resulting file with options
func CopyReaderWithOptions(dst string, src io.Reader, options ...FileOption) error {
	tmp, err := ioutil.TempFile(filepath.Dir(dst), "")
	if err != nil {
		return trace.ConvertSystemError(err)
	}

	cleanup := func() {
		err := os.Remove(tmp.Name())
		if err != nil {
			log.WithError(err).Warnf("Failed to remove %q.", tmp.Name())
		}
	}

	_, err = io.Copy(tmp, src)
	if err != nil {
		tmp.Close()
		cleanup()
		return trace.ConvertSystemError(err)
	}
	if err = tmp.Close(); err != nil {
		cleanup()
		return trace.ConvertSystemError(err)
	}
	for _, option := range options {
		if err = option(tmp.Name()); err != nil {
			cleanup()
			return trace.ConvertSystemError(err)
		}
	}
	err = os.Rename(tmp.Name(), dst)
	if err != nil {
		cleanup()
		return trace.ConvertSystemError(err)
	}
	return nil
}

// FileOption defines a functional option to apply to specified path
type FileOption func(path string) error

// WithFileMode changes the file permissions on the specified file
// to perm
func WithFileMode(perm os.FileMode) FileOption {
	return func(path string) error {
		return os.Chmod(path, perm)
	}
}

// OwnerOption changes the owner on the specified file
// to (uid, gid)
func OwnerOption(uid, gid int) FileOption {
	return func(path string) error {
		return os.Chown(path, uid, gid)
	}
}

var jsonPrefix = []byte("{")

// hasJSONPrefix returns true if the provided buffer appears to start with
// a JSON open brace.
func hasJSONPrefix(buf []byte) bool {
	return hasPrefix(buf, jsonPrefix)
}

// Return true if the first non-whitespace bytes in buf is
// prefix.
func hasPrefix(buf []byte, prefix []byte) bool {
	trim := bytes.TrimLeftFunc(buf, unicode.IsSpace)
	return bytes.HasPrefix(trim, prefix)
}
