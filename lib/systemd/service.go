/*
Copyright 2020 Gravitational, Inc.

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

package systemd

import (
	"io/ioutil"
	"os"

	"github.com/gravitational/planet/lib/constants"
)

// MaskService masks a service. Deletes the unit file first.
// Returns an error if the unit file doesn't exist.
func MaskService(unitFilePath string) error {
	err := os.Remove(unitFilePath)
	if err != nil {
		return err
	}

	err = os.Symlink(
		"/dev/null",
		unitFilePath,
	)
	if err != nil {
		return err
	}

	return err
}

// DropIn creates a systemd drop-in unit
func DropIn(dropInFileName string, dropInDir string, dropInContents string) error {
	err := os.MkdirAll(dropInDir, os.FileMode(constants.OwnerGroupRWXOtherRX))
	if err != nil {
		return err
	}

	dropInFullPath := dropInDir + dropInFileName

	if err := ioutil.WriteFile(dropInFullPath, []byte(dropInContents), os.FileMode(constants.OwnerGroupRWXOtherRX)); err != nil {
		return err
	}
	return err
}
