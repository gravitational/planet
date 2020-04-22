//+build mage

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
	"runtime/debug"
	"time"

	upgrade "github.com/gravitational/planet/test/etcd-upgrade"
	"github.com/gravitational/planet/test/leadership"

	"github.com/gravitational/trace"
	"github.com/magefile/mage/mg"
)

type Test mg.Namespace

func (Test) TestEtcdUpgrade() error {
	// The "to" version should match what etcd is built with
	// TODO(knisbet) integrate this variable with etcd versions set in Makefile
	err := upgrade.TestUpgradeBetweenVersions("v3.3.3", "v3.4.3")
	return trace.Wrap(err)
}

func (Test) TestLeaderToleratesEtcdDowntime() error {
	const timeout = 1 * time.Minute
	go func() {
		<-time.After(timeout)
		debug.SetTraceback("all")
		panic("test timed out")
	}()
	// TODO: use the latest version from the Makefile
	return leadership.TestCandidateToleratesClusterFailure("v3.4.3")
}
