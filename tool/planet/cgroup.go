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

package main

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	systemdDbus "github.com/coreos/go-systemd/dbus"

	"github.com/gravitational/trace"
	"github.com/sirupsen/logrus"
)

// runSystemdScopeCleaner implements a workaround for a kubernetes/systemd issue with cgroups/systemd scopes that leak
// under certain circumstances, usually when using a kubernetes cronjob with a mount. This appears to be mostly a
// systemd or kernel issue, where the pids running within the scope do not cause the scope to complete and clean up
// resulting in leaking memory.
// https://github.com/kubernetes/kubernetes/issues/70324
// https://github.com/kubernetes/kubernetes/issues/64137
// https://github.com/gravitational/gravity/issues/1219
//
// Kubernetes is using systemd-run --scope when creating mounts within systemd.
// https://github.com/gravitational/kubernetes/blob/1c045a09db662c6660562d88deff2733ca17dcf8/pkg/util/mount/mount_linux.go#L108-L131
//
// To clean up this leak, we want to scan for run-xxx.scope cgroups that do not have any processes, are atleast a minute
// old, and then have systemd remove scopes that do not hold any processes / where all processes have exited.
func runSystemdCgroupCleaner(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			err := cleanSystemdScopes()
			if err != nil {
				logrus.WithError(err).Warn("Failed to clean systemd scopes that don't contain processes.")
			}
		case <-ctx.Done():
			return
		}
	}
}

func cleanSystemdScopes() error {
	log := logrus.WithField(trace.Component, "cgroup-cleaner")

	conn, err := systemdDbus.New()
	if err != nil {
		return trace.Wrap(err)
	}
	defer conn.Close()

	var paths []string

	baseTime := time.Now().Add(-time.Minute)

	root := "/sys/fs/cgroup/systemd/"
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return trace.ConvertSystemError(err)
		}

		// A run scope will have a directory name that looks something like run-r2343e8b13fd44b1297e241421fc1f6e3.scope
		// We also want to protect against potential races, where the cgroup is created but doesn't have any pids
		// added yet. So only consider paths that have existed for atleast a minute to be safe
		if strings.HasPrefix(info.Name(), "run-") &&
			strings.HasSuffix(info.Name(), ".scope") &&
			baseTime.After(info.ModTime()) {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		log.WithError(err).Warn("Error occurred while scanning cgroup hierarchy for unused systemd scopes.")
	}

	for _, path := range paths {
		unitName := filepath.Base(path)
		log := log.WithFields(logrus.Fields{
			"path": path,
			"unit": unitName,
		})

		// the cgroup virtual filesystem does not report file sizes, so we need to read the cgroup.procs file
		// to see if there are any contents (any processes listed)
		// http://man7.org/linux/man-pages/man7/cgroups.7.html
		// The cgroup.procs file can be read to obtain a list of the processes
		// that are members of a cgroup.  The returned list of PIDs is not guar‐
		// anteed to be in order.  Nor is it guaranteed to be free of dupli‐
		// cates.  (For example, a PID may be recycled while reading from the
		// list.)
		procsPath := filepath.Join(path, "cgroup.procs")
		pids, err := ioutil.ReadFile(procsPath)
		if err != nil {
			if !trace.IsNotFound(trace.ConvertSystemError(err)) {
				log.WithError(err).Warn("Failed to read process list belonging to cgroup.")
			}
			continue
		}

		if len(pids) != 0 {
			continue
		}

		_, err = conn.StopUnit(unitName, "replace", nil)
		if err != nil {
			log.WithError(err).Warn("Failed to stop systemd unit.")
			continue
		}

		log.Info("Stopped systemd scope unit with no pids.")
	}

	return nil
}

/*
Extra Notes for Cgroup Cleanup.
The issue can be reproduced on centos 7.7.1908 with the following kubernetes config
apiVersion: v1
kind: Secret
metadata:
  name: mysecret
type: Opaque
data:
  username: YWRtaW4=
  password: MWYyZDFlMmU2N2Rm
---
apiVersion: batch/v1beta1
kind: CronJob
metadata:
  name: hello
spec:
  schedule: "* * * * *"
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: hello
            image: busybox
            args:
            - /bin/sh
            - -c
            - date; echo Hello from the Kubernetes clusterl; sleep 60
            volumeMounts:
            - name: foo
              mountPath: "/etc/foo"
              readOnly: true
          restartPolicy: OnFailure
          volumes:
          - name: foo
            secret:
                    secretName: mysecret
*/
