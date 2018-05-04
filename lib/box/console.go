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
		if err != nil {
			select {
			case consoleCh <- &resp{
				err: err,
			}:
			case <-ctx.Done():
				log.Warnf("Context is closing: %v.", ctx.Err())
			}
			return
		}
		console, err := libconsole.ConsoleFromFile(f)
		if err != nil {
			select {
			case consoleCh <- &resp{
				err: err,
			}:
			case <-ctx.Done():
				log.Warnf("Context is closing: %v.", ctx.Err())
			}
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
