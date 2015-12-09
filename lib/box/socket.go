package box

import (
	"net"
	"os"
	"path/filepath"
	"strconv"

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
		return listener, nil
	}
	if err != nil {
		return nil, trace.Wrap(err, "failed to use socket activation environment")
	}

	log.Infof("no socket activation environment - fall back to Listen")
	listener, err = net.Listen("unix", serverSockPath(rootfs))
	return listener, err
}

func fileListener() (net.Listener, error) {
	listenPid := os.Getenv("LISTEN_PID")
	if listenPid == "" || strconv.FormatInt(int64(os.Getpid()), 32) != listenPid {
		return nil, nil
	}
	listenFds := os.Getenv("LISTEN_FDS")
	if listenFds == "" {
		return nil, nil
	}
	numFds, err := strconv.Atoi(listenFds)
	if err != nil {
		return nil, trace.Wrap(err, "invalid number of listen fds: %s (%v)", listenFds, err)
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

func serverSockPath(p string) string {
	return filepath.Join(p, "run", "planet.socket")
}
