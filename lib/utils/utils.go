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
	"path/filepath"
	"unicode"

	"github.com/gravitational/planet/lib/constants"

	"github.com/ghodss/yaml"
	"github.com/gravitational/trace"
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
