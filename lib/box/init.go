package box

import (
	"os"
	"runtime"

	"github.com/gravitational/trace"
	"github.com/opencontainers/runc/libcontainer"
	_ "github.com/opencontainers/runc/libcontainer/nsenter"
	log "github.com/sirupsen/logrus"
)

func init() {
	if len(os.Args) > 1 && os.Args[1] == "init" {
		runtime.GOMAXPROCS(1)
		runtime.LockOSThread()
	}
}

// Init is implicitly called by the libcontainer logic and is used to start
// a process in the new namespaces and cgroups
func Init() error {
	factory, err := libcontainer.New("")
	if err != nil {
		return trace.Wrap(err, "failed to create container factory")
	}
	if err := factory.StartInitialization(); err != nil {
		log.Warnf("Failed to initialize container factory: %v.", err)
		return trace.Wrap(err, "failed to initialize container factory")
	}
	panic("libcontainer: container init failed to exec")
}
