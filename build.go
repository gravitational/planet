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
	upgrade "github.com/gravitational/planet/test/etcd-upgrade"
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
