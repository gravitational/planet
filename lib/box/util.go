/*
Copyright 2019 Gravitational, Inc.

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
	"os"

	"github.com/gravitational/trace"
)

// CgroupNSEnabled checks whether the system has cgroup namespaces enabled
// Based on internal function from runc
// https://github.com/opencontainers/runc/blob/029124da7af7360afa781a0234d1b083550f797c/libcontainer/configs/validate/validator.go#L122-L129
func CgroupNSEnabled() (bool, error) {
	_, err := os.Stat("/proc/self/ns/cgroup")
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, trace.ConvertSystemError(err)
	}
	return true, nil
}
