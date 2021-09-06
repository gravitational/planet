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

package main

import (
	"encoding/json"
	"os/exec"

	log "github.com/sirupsen/logrus"
)

func joinSerfClusterIfPossible(config agentConfig) {
	path, err := exec.LookPath("serf")
	if err != nil {
		if execErr, ok := err.(*exec.Error); ok && execErr.Err == exec.ErrNotFound {
			// No serf binary in container - ignore
			return
		}
		log.WithError(err).Debug("Failed to locate serf binary.")
		return
	}
	serf := serf{path: path}
	if !serf.isMember(config.agent.Name) {
		return
	}
	serf.join(config.peers...)
}

// serfMember describes a serf cluster member.
// It contains just enough information to identify the nodes
type serfMember struct {
	// Name identifies the serf cluster member by name
	Name string `json:"name"`
}

// isMember determines whether name is part of the serf cluster.
// It works on best-effort basis - only if serf binary is available
// and if the serf agent is running can it correctly determine the membership.
// The errors are logged at Debug.
func (r serf) isMember(name string) bool {
	out, err := exec.Command(r.path, "members", "-format", "json").CombinedOutput()
	if err != nil {
		log.WithError(err).WithField("out", string(out)).Debug("Unable to query serf members.")
	}
	var members []serfMember
	if err := json.Unmarshal(out, &members); err != nil {
		log.WithError(err).Debug("Unable to decode serf members.")
	}
	for _, m := range members {
		if name == m.Name {
			return true
		}
	}
	return false
}

// join joins the serf cluster given a list of existing peers.
// Works on best-effort basis and ignores all errors.
// The errors are logged at Debug.
func (r serf) join(peers ...string) {
	args := append([]string{"join"}, peers...)
	out, err := exec.Command(r.path, args...).CombinedOutput()
	if err != nil {
		log.WithError(err).WithField("out", string(out)).Debug("Failed to join serf cluster.")
	}
}

type serf struct {
	path string
}
