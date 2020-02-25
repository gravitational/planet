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

package box

import (
	"bytes"
	"os/exec"
	"path/filepath"

	"github.com/gravitational/planet/lib/constants"
	"github.com/gravitational/planet/lib/defaults"

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

// getSELinuxProcLabel returns a new function to compute SELinux labels.
//
// The returned function will compute the appropriate SELinux domain for the
// command specified with cmd and fall back to defaults.ContainerProcessLabel
// if the domain cannot be determined.
// Assumes SELinux support is on
func getSELinuxProcLabel(rootfs string) selinuxLabelGetterFunc {
	return func(cmd string) (label string) {
		label, err := getProcLabel(filepath.Join(rootfs, cmd))
		if err != nil {
			log.WithFields(log.Fields{
				log.ErrorKey: err,
				"path":       cmd,
			}).Warn("Failed to compute process label.")
			label = defaults.ContainerProcessLabel
		}
		return label
	}
}

// getProcLabel computes the label for the new process initiating from the file
// given wih path. The label is computed in the context of the init process.
func getProcLabel(path string) (label string, err error) {
	out, err := exec.Command("selinuxexeccon", path, constants.ContainerInitProcessLabel).CombinedOutput()
	if err != nil {
		return "", trace.Wrap(err, "failed to compute process label for %v: %s",
			path, out)
	}
	return string(bytes.TrimSpace(out)), nil
}

type seLinuxLabelGetter interface {
	getSELinuxLabel(cmd string) (label string)
}

func (r selinuxLabelGetterFunc) getSELinuxLabel(cmd string) (label string) {
	return r(cmd)
}

type selinuxLabelGetterFunc func(cmd string) (label string)
