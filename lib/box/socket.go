package box

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
)

// http://www.freedesktop.org/software/systemd/man/sd_listen_fds.html
// First file descriptor used in socket activation.
// If a daemon is configured with multiple file descriptors, the remaining descriptors are passed
// in the order configured in socket unit file as 4, 5, 6...
const SD_LISTEN_FDS_START uintptr = 3

func newSockListener(rootfs string) (net.Listener, error) {
	listener, err := fileListener()
	if err == nil && listener != nil {
		log.Infof("using socket activation")
		return listener, nil
	}
	if err != nil {
		return nil, trace.Wrap(err, "failed to use socket activation context")
	}

	log.Infof("no socket activation context - fall back to Listen")
	listener, err = net.Listen("unix", serverSockPath(rootfs))
	return listener, err
}

func fileListener() (net.Listener, error) {
	listenPid, exists := syscall.Getenv("LISTEN_PID")
	if !exists || strconv.FormatInt(int64(os.Getpid()), 10) != listenPid {
		return nil, nil
	}
	listenFds, exists := syscall.Getenv("LISTEN_FDS")
	if !exists {
		return nil, trace.Errorf("invalid socket activation context - expected a socket file descriptor")
	}
	numFds, err := strconv.Atoi(listenFds)
	if err != nil {
		return nil, trace.Wrap(err, "invalid number of socket file descriptors: %s (%v)", listenFds, err)
	}
	if numFds <= 0 {
		return nil, nil
	}
	listener, err := net.FileListener(os.NewFile(SD_LISTEN_FDS_START, ""))
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return listener, nil
}

func serverSockPath(rootfs string) string {
	return filepath.Join(rootfs, "run", "planet.socket")
}
