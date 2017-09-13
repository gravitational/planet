package check

import (
	"bytes"
	"os"
	"os/exec"
	"os/user"

	"github.com/gravitational/trace"
)

const (
	PlanetUser  string = "planet"
	PlanetGroup string = "planet"
)

// CheckUserGroup checks if a user specified with userName has been created.
// If no user has been created - it will attempt to create one.
// It will also attempt to create a group specified with groupName.
func CheckUserGroup(userName, groupName, uid, gid string) (u *user.User, err error) {
	// already exists?
	u, err = user.Lookup(userName)
	if err == nil {
		return u, nil
	}

	output, err := run(groupAddCommand(groupName, gid))
	if err != nil {
		output, err = run(groupAddCommand(groupName, gid, "--extrausers"))
		if err != nil {
			return nil, trace.Wrap(err,
				"failed to create group %q: %s", groupName, output)
		}
	}

	output, err = run(userAddCommand(userName, uid, gid))
	if err != nil {
		output, err = run(userAddCommand(userName, uid, gid, "--extrausers"))
		if err != nil {
			return nil, trace.Wrap(err,
				"failed to create user %q in group %q: %s", userName, groupName, output)
		}
	}

	return user.Lookup(userName)
}

// run runs the command cmd and returns the output.
func run(cmd *exec.Cmd) ([]byte, error) {
	output, err := cmd.CombinedOutput()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, err
		}
		return bytes.TrimSpace(output), err
	}
	return nil, nil
}

func userAddCommand(userName, uid, gid string, extraArgs ...string) *exec.Cmd {
	cmd := exec.Command("/usr/sbin/useradd", append([]string{
		"--system",
		"--no-create-home",
		"--non-unique",
		"--gid", gid,
		"--uid", uid,
		userName}, extraArgs...)...)
	return cmd
}

func groupAddCommand(groupName, gid string, extraArgs ...string) *exec.Cmd {
	cmd := exec.Command("/usr/sbin/groupadd", append([]string{
		"--system",
		"--non-unique",
		"--gid", gid,
		groupName}, extraArgs...)...)
	return cmd
}
