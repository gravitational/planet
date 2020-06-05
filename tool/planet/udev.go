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
	"context"
	"encoding/json"
	"os"

	"github.com/gravitational/planet/lib/box"
	"github.com/gravitational/planet/lib/constants"
	"github.com/gravitational/planet/lib/ctxgroup"

	"github.com/gravitational/go-udev"
	"github.com/gravitational/trace"
	"github.com/opencontainers/runc/libcontainer/configs"
	"github.com/opencontainers/runc/libcontainer/devices"
	log "github.com/sirupsen/logrus"
)

// newUdevListener creates a new udev event listener listening
// for events on block devices of type `disk`
func newUdevListener(seLinux bool) (*udevListener, error) {
	udev := udev.Udev{}
	monitor := udev.NewMonitorFromNetlink("udev")
	if monitor == nil {
		return nil, trace.BadParameter("failed to create udev monitor")
	}
	doneC := make(chan struct{})

	monitor.FilterAddMatchSubsystemDevtype("block", "disk")
	monitor.FilterAddMatchSubsystemDevtype("block", "partition")
	monitor.FilterAddMatchTag("systemd")

	ctx, cancel := context.WithCancel(context.Background())
	g := ctxgroup.WithContext(ctx)
	recvC, err := monitor.DeviceChan(ctx)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	listener := &udevListener{
		log:     log.WithField(trace.Component, "udev"),
		monitor: monitor,
		doneC:   doneC,
		recvC:   recvC,
		g:       g,
		cancel:  cancel,
		seLinux: seLinux,
	}
	return listener, nil
}

// Start starts the internal udev loop
func (r *udevListener) Start() {
	r.g.GoCtx(r.loop)
}

// Run starts the internal udev loop and waits for it to exit
func (r *udevListener) Run() error {
	r.g.GoCtx(r.loop)
	return r.g.Wait()
}

// Close closes the listener and removes the installed udev filters
func (r *udevListener) Close() error {
	removeFilters := func() {
		r.monitor.FilterRemove()
		r.monitor.FilterUpdate()
	}
	r.cancel()
	close(r.doneC)
	removeFilters()
	return nil
}

// udevListener defines the task of listening to udev events
// and dispatching corresponding device commands into the planet container
type udevListener struct {
	log     log.FieldLogger
	monitor *udev.Monitor
	cancel  context.CancelFunc
	g       ctxgroup.Group
	doneC   chan struct{}
	recvC   <-chan *udev.Device
	seLinux bool
}

// loop runs the actual udev event loop
func (r *udevListener) loop(ctx context.Context) error {
	const cgroupPermissions = "rwm"

	for {
		select {
		case device := <-r.recvC:
			switch device.Action() {
			case "add":
				r.log.WithField("devnode", device.Devnode()).Info("Add new device.")
				deviceData, err := devices.DeviceFromPath(device.Devnode(), cgroupPermissions)
				if err != nil {
					r.log.Warnf("Failed to query device: %v.", err)
					continue
				}
				if err := r.createDevice(deviceData); err != nil {
					r.log.Warnf("Failed to create device %q: %v.", device.Devnode(), err)
				}
			case "remove":
				r.log.WithField("devnode", device.Devnode()).Info("Remove device.")
				if err := r.removeDevice(device.Devnode()); err != nil {
					r.log.Warnf("Failed to remove device %q: %v.", device.Devnode(), err)
				}
			default:
				r.log.Debugf("Skipping unsupported action %q for %v (%q at %v).",
					device.Action(), device.Devnode(), device.Devtype(), device.Devpath())
			}

		case <-ctx.Done():
			return trace.Wrap(ctx.Err())
		}
	}
}

// createDevice dispatches a command to add a new device in the container
func (r *udevListener) createDevice(device *configs.Device) error {
	r.log.WithField("device", device).Debug("Create device.")

	deviceJson, err := json.Marshal(device)
	if err != nil {
		return trace.Wrap(err)
	}

	return trace.Wrap(enter(r.deviceCmd("add", "--data", string(deviceJson))))
}

// removeDevice dispatches a command to remove a device in the container
func (r *udevListener) removeDevice(node string) error {
	r.log.WithField("device", node).Debug("Remove device.")

	return trace.Wrap(enter(r.deviceCmd("remove", "--node", node)))
}

// deviceCmd creates a configuration object to invoke the device agent
// with the specified arguments
func (r *udevListener) deviceCmd(args ...string) box.EnterConfig {
	const cmd = "/usr/bin/planet"
	return box.EnterConfig{
		Process: box.ProcessConfig{
			User:         "root",
			Args:         append([]string{cmd, "--debug", "device"}, args...),
			In:           os.Stdin,
			Out:          os.Stdout,
			ProcessLabel: constants.ContainerRuntimeProcessLabel,
		},
		SELinux: r.seLinux,
	}
}
