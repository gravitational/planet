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
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/gravitational/planet/lib/constants"

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
