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

package box

import (
	"os"
	"runtime"

	"github.com/gravitational/trace"
	"github.com/opencontainers/runc/libcontainer"
	_ "github.com/opencontainers/runc/libcontainer/nsenter"
	log "github.com/sirupsen/logrus"
)

func init() {
	if len(os.Args) > 1 && os.Args[1] == "init" {
		runtime.GOMAXPROCS(1)
		runtime.LockOSThread()
	}
}

// Init is implicitly called by the libcontainer logic and is used to start
// a process in the new namespaces and cgroups
func Init() error {
	factory, err := libcontainer.New("")
	if err != nil {
		return trace.Wrap(err, "failed to create container factory")
	}
	if err := factory.StartInitialization(); err != nil {
		log.Warnf("Failed to initialize container factory: %v.", err)
		return trace.Wrap(err, "failed to initialize container factory")
	}
	panic("libcontainer: container init failed to exec")
}
