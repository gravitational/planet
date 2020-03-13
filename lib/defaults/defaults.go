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

package defaults

const (
	// ContainerProcessLabel specifies the default SELinux label for processes inside the container
	ContainerProcessLabel = "system_u:system_r:gravity_container_t:s0"

	// ContainerFileLabel specifies the default SELinux label for files inside the container
	ContainerFileLabel = "system_u:object_r:gravity_container_file_t:s0"

	// PlanetDataDir specifies the location for libcontainer-specific data
	PlanetDataDir = "/var/run/planet"

	// InitUser specifies the user for the init process
	InitUser = "root"

	// RuncDataDir is the directory used to store runc runtime data within planet
	RuncDataDir = "/var/run/planet"

	// ContainerBaseUID specifies the initial user ID for the host-container
	// mapping
	ContainerBaseUID = 1000
	// ContainerBaseGID specifies the initial groupID for the host-container
	// mapping
	ContainerBaseGID = 1000
)

var (
	// InitArgs specifies the command line for the init process
	InitArgs = []string{"/bin/systemd"}
)
