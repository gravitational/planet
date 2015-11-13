package check

import (
	"os"
	"os/exec"
	"os/user"
	"strings"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
)

const (
	PlanetUID   string = "1000" // planet tarball must have all files owned by UID:GID of 1000:1000
	PlanetGID   string = "1000"
	PlanetUser  string = "planet"
	PlanetGroup string = "planet"
)

// checks to se
func CheckPlanetUser() (u *user.User, err error) {
	// already exists?
	u, err = user.Lookup(PlanetUser)
	if err == nil {
		return u, nil
	}

	// create a new group:
	groupadd := exec.Command("/usr/sbin/groupadd",
		"--system",
		"--non-unique",
		"--gid", PlanetGID,
		PlanetGroup)
	output, err := groupadd.CombinedOutput()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, err
		}
		errMsg := flattenString(string(output))
		return nil, trace.Wrap(err, "failed to create group '%v': %v", PlanetUser, errMsg)
	}

	// create a new user:
	useradd := exec.Command("/usr/sbin/useradd",
		"--system",
		"--no-create-home",
		"--non-unique",
		"--gid", PlanetGID,
		"--uid", PlanetUID,
		PlanetUser)
	output, err = useradd.CombinedOutput()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, err
		}
		errMsg := flattenString(string(output))
		return nil, trace.Wrap(err, "failed to create user '%v': %v", PlanetUser, errMsg)
	}

	// now it should work:
	return user.Lookup(PlanetUser)
}

func flattenString(s string) string {
	return strings.Join(strings.Split(s, "\n"), " ")
}
