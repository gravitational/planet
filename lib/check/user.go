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

package check

import (
	"bytes"
	"os"
	"os/exec"
	"os/user"

	"github.com/gravitational/trace"
)

const (
	PlanetUser  string = "planet"
	PlanetGroup string = "planet"
)

// CheckUserGroup checks if a user specified with userName has been created.
// If no user has been created - it will attempt to create one.
// It will also attempt to create a group specified with groupName.
func CheckUserGroup(userName, groupName, uid, gid string) (u *user.User, err error) {
	// already exists?
	u, err = user.Lookup(userName)
	if err == nil {
		return u, nil
	}

	output, err := run(groupAddCommand(groupName, gid))
	if err != nil {
		return nil, trace.Wrap(err, "failed to create group '%s': %s", groupName, output)
	}

	output, err = run(userAddCommand(userName, uid, gid))
	if err != nil {
		return nil, trace.Wrap(err, "failed to create user '%s' in group '%s': %s", userName, groupName, output)
	}

	return user.Lookup(userName)
}

// run runs the command cmd and returns the output.
func run(cmd *exec.Cmd) ([]byte, error) {
	output, err := cmd.CombinedOutput()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, err
		}
		return bytes.TrimSpace(output), err
	}
	return nil, nil
}

func userAddCommand(userName, uid, gid string) *exec.Cmd {
	cmd := exec.Command("/usr/sbin/useradd",
		"--system",
		"--no-create-home",
		"--non-unique",
		"--gid", gid,
		"--uid", uid,
		userName)
	return cmd
}

func groupAddCommand(groupName, gid string) *exec.Cmd {
	cmd := exec.Command("/usr/sbin/groupadd",
		"--system",
		"--non-unique",
		"--gid", gid,
		groupName)
	return cmd
}
