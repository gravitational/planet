package check

import (
	"bytes"
	"os"
	"os/exec"
	"os/user"

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
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
		log.Warnf("failed to create group %q in regular groups database: %v %s", groupName, trace.DebugReport(err), output)
		extraOutput, extraErr := run(groupAddCommand(groupName, gid, "--extrausers"))
		if extraErr != nil {
			return nil, trace.NewAggregate(
				trace.Wrap(err, "failed to create group %q in regular groups database: %s", groupName, output),
				trace.Wrap(extraErr, "failed to create group %q in extrausers database: %s", groupName, extraOutput))
		}
		log.Infof("group %q created in extrausers database", groupName)
	}

	output, err = run(userAddCommand(userName, uid, gid))
	if err != nil {
		log.Warnf("failed to create user %q in regular users database: %v %s", userName, trace.DebugReport(err), output)
		extraOutput, extraErr := run(userAddCommand(userName, uid, gid, "--extrausers"))
		if extraErr != nil {
			return nil, trace.NewAggregate(
				trace.Wrap(err, "failed to create user %q in regular users database: %s", userName, output),
				trace.Wrap(extraErr, "failed to create user %q in extrausers database: %s", userName, extraOutput))
		}
		log.Infof("user %q created in extrausers database", userName)
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
