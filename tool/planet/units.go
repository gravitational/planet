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
	systemdDbus "github.com/coreos/go-systemd/dbus"
	"github.com/godbus/dbus"
	"github.com/gravitational/trace"
	"github.com/sirupsen/logrus"
)

func newUnitMonitor() (*unitMonitor, error) {
	conn, err := systemdDbus.NewSystemConnection()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &unitMonitor{
		conn: conn,
		log:  logrus.WithField(trace.Component, "unitmonitor"),
	}, nil
}

type unitMonitor struct {
	conn *systemdDbus.Conn
	log  logrus.FieldLogger
}

func (r *unitMonitor) close() {
	r.conn.Close()
}

func (r *unitMonitor) start(name string) error {
	r.log.WithField("unit", name).Debug("Start service.")
	if _, err := r.conn.StartUnit(name, "replace", nil); err != nil {
		if isNoSuchUnitError(err) {
			return trace.NotFound("service %v not found", name)
		}
		return trace.Wrap(err, "failed to start service %v", name)
	}
	return nil
}

func (r *unitMonitor) stop(name string) error {
	r.log.WithField("unit", name).Debug("Shut down service.")
	if _, err := r.conn.StopUnit(name, "replace", nil); err != nil {
		if isNoSuchUnitError(err) {
			return trace.NotFound("service %v not found", name)
		}
		return trace.Wrap(err, "failed to stop service %v", name)
	}
	return nil
}

func (r *unitMonitor) filterUnits(states ...serviceActiveState) ([]systemdDbus.UnitStatus, error) {
	filter := make([]string, 0, len(states))
	for _, state := range states {
		filter = append(filter, string(state))
	}
	return r.conn.ListUnitsFiltered(filter)
}

// status validates whether the status of the specified service equals targetStatus
func (r *unitMonitor) status(service string) (*systemdDbus.UnitStatus, error) {
	units, err := r.conn.ListUnits()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	for _, unit := range units {
		if unit.Name == service {
			return &unit, nil
		}
	}
	return nil, trace.NotFound("service %v not found", service)
}

// resetStatus resets the status of the specified service
func (r *unitMonitor) resetStatus(service string) error {
	return r.conn.ResetFailedUnit(service)
}

func isNoSuchUnitError(err error) bool {
	if dbusErr, ok := trace.Unwrap(err).(dbus.Error); ok {
		return dbusErr.Name == "org.freedesktop.systemd1.NoSuchUnit"
	}
	return false
}

type serviceActiveState string

const (
	activeStateInactive serviceActiveState = "inactive"
	activeStateActive   serviceActiveState = "active"
	activeStateFailed   serviceActiveState = "failed"
)
