package box

import (
	"os"
	"runtime"

	"github.com/opencontainers/runc/libcontainer"
	_ "github.com/opencontainers/runc/libcontainer/nsenter"
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
	factory, _ := libcontainer.New("")
	if err := factory.StartInitialization(); err != nil {
		// as the error is sent back to the parent there is no need to log
		// or write it to stderr because the parent process will handle this
		os.Exit(1)
	}
	panic("libcontainer: container init failed to exec")
}
