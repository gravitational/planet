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
	"context"
	"os"

	libconsole "github.com/containerd/console"
	"github.com/gravitational/trace"
	libcontainerutils "github.com/opencontainers/runc/libcontainer/utils"
	log "github.com/sirupsen/logrus"
)

// getContainerConsole returns the container console from the specified socket file.
// Returned console needs to be closed when no longer used.
func getContainerConsole(ctx context.Context, consoleSocket *os.File) (libconsole.Console, error) {
	type resp struct {
		libconsole.Console
		err error
	}
	consoleCh := make(chan *resp, 1)

	go func() {
		f, err := libcontainerutils.RecvFd(consoleSocket)
		defer func() {
			if err == nil {
				return
			}
			select {
			case consoleCh <- &resp{
				err: err,
			}:
			case <-ctx.Done():
				log.Warnf("Context is closing: %v.", ctx.Err())
			}
		}()
		if err != nil {
			return
		}
		console, err := libconsole.ConsoleFromFile(f)
		if err != nil {
			f.Close()
			return
		}
		libconsole.ClearONLCR(console.Fd())
		consoleCh <- &resp{
			Console: console,
		}
	}()

	select {
	case resp := <-consoleCh:
		if resp.err != nil {
			return nil, trace.Wrap(resp.err, "failed to set up console")
		}
		return resp.Console, nil
	case <-ctx.Done():
		return nil, trace.Wrap(ctx.Err())
	}
}
