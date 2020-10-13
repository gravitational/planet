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
	"os"
	"os/exec"
	"path/filepath"

	"github.com/gravitational/planet/lib/constants"
	"github.com/gravitational/planet/lib/defaults"

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

// getSELinuxProcLabel computes the appropriate SELinux domain for the
// command specified with cmd and falls back to defaults.ContainerProcessLabel
// if the domain cannot be determined.
// Assumes SELinux support is on
func getSELinuxProcLabel(rootfs, cmd string) (label string) {
	logger := log.WithField("path", cmd)
	if !filepath.IsAbs(cmd) {
		abspath, err := getAbsPathForCommand(rootfs, cmd)
		if err != nil {
			log.WithError(err).Warn("Failed to find absolute path to command in rootfs.")
		} else {
			cmd = abspath
		}
	}
	label, err := getProcLabel(filepath.Join(rootfs, cmd))
	if err != nil {
		logger.WithError(err).Warn("Failed to compute process label.")
		label = defaults.ContainerProcessLabel
	}
	return label
}

// getProcLabel computes the label for the new process initiating from the file
// given with path. The label is computed in the context of the init process.
func getProcLabel(path string) (label string, err error) {
	out, err := exec.Command("selinuxexeccon", path, constants.ContainerInitProcessLabel).CombinedOutput()
	if err != nil {
		return "", trace.Wrap(err, "failed to compute process label for %v: %s",
			path, out)
	}
	return string(bytes.TrimSpace(out)), nil
}

// getAbsPathForCommand returns a match for the specified command cmd
// in the context of the given rootfs using default container PATH configuration.
// Returns the suffix including the command but without the rootfs prefix
func getAbsPathForCommand(rootfs, cmd string) (path string, err error) {
	isFile := func(prefix string) bool {
		fi, err := os.Lstat(filepath.Join(rootfs, prefix, cmd))
		return err == nil && (fi.Mode()&os.ModeType) == 0
	}
	for _, prefix := range constants.ContainerEnvPath {
		if isFile(prefix) {
			return filepath.Join(prefix, cmd), nil
		}
	}
	return "", trace.NotFound("executable %v not found in PATH", cmd)
}
