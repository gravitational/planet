package box

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
)

// http://www.freedesktop.org/software/systemd/man/sd_listen_fds.html
// First file descriptor used in socket activation.
// If a daemon is configured with multiple file descriptors, the remaining descriptors are passed
// in the order configured in socket unit file as 4, 5, 6...
const SD_LISTEN_FDS_START uintptr = 3

// socketListener is a net.Listener that does not unlink the socket file
// for pathname type unix domain sockets.
// It is required to properly support systemd socket activation and acts as a wrapper on the
// native UnixListener implementation that unconditionally unlinks the socket file in Close.
type socketListener struct {
	net.Listener
}

// Close is a no-op for a socket-activated listener since systemd is managing the socket file.
func (r *socketListener) Close() error {
	return nil
}

// newSockListener returns a net.Listener that is either a net.UnixListener
// or a socketListener if systemd socket-activation is used.
func newSockListener(socketPath string) (net.Listener, error) {
	listener, err := fileListener()
	if err == nil && listener != nil {
		log.Infof("using socket activation")
		// TODO: skip wrapping for abstract sockets
		return &socketListener{listener}, nil
	}
	if err != nil {
		return nil, trace.Wrap(err, "failed to use socket activation context")
	}

	log.Infof("no socket activation context - fall back to Listen")
	listener, err = net.Listen("unix", socketPath)
	return listener, err
}

func dial(socketPath string) (net.Conn, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, checkError(
			trace.Wrap(err, "failed to connect to planet socket"))
	}
	// FIXME: read deadline for a websocket connection to avoid blocking on a systemd
	// operated socket w/o server.
	// conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	return conn, nil
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

func serverSockPath(rootfs, socketPath string) string {
	if filepath.IsAbs(socketPath) || isAbstractSocket(socketPath) {
		return socketPath
	}
	return filepath.Join(rootfs, socketPath)
}

func isAbstractSocket(socketPath string) bool {
	return strings.HasPrefix(socketPath, "@")
}

func checkError(err error) error {
	var errOrig error
	if e, ok := err.(*trace.TraceErr); ok {
		errOrig = e.OrigError()
	} else {
		errOrig = err
	}

	if os.IsNotExist(errOrig) {
		return &ErrConnect{Err: err}
	}
	if _, ok := err.(*net.OpError); ok {
		return &ErrConnect{Err: err}
	}
	return err
}

// IsConnectError returns true if err is a connection error.
func IsConnectError(err error) bool {
	_, ok := err.(*ErrConnect)
	return ok
}
