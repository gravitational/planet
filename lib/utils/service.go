package utils

import (
	"bytes"
	"context"
	"os/exec"

	"github.com/gravitational/trace"
)

// ServiceIsActive determines if the service given with name is active
func ServiceIsActive(ctx context.Context, name string) (active bool, out []byte, err error) {
	out, err = exec.CommandContext(ctx, servicectlBin, "is-active", name).CombinedOutput()
	if err != nil {
		return false, out, trace.Wrap(err)
	}
	return bytes.Equal(bytes.TrimSpace(out), []byte("active")), out, nil
}

// ServiceCtl executes the command cmd on service name.
// Command is blocking if blocking == true
func ServiceCtl(ctx context.Context, cmd, name string, blocking Blocking) (out []byte, err error) {
	args := []string{cmd, name}
	if !blocking {
		args = append(args, "--no-block")
	}
	out, err = exec.CommandContext(ctx, servicectlBin, args...).CombinedOutput()
	if err != nil {
		return out, trace.Wrap(err)
	}
	return out, nil
}

// Blocking controls whether a command is blocking
type Blocking bool

const servicectlBin = "/bin/systemctl"
