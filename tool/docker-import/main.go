/*
Copyright 2018 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"archive/tar"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gravitational/planet/lib/utils"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"

	"gopkg.in/alecthomas/kingpin.v2"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	app := kingpin.New("docker-import", "Import container images from a directory into private docker registry")
	tarballDir := app.Flag("dir", "Directory with image tarballs").Required().String()
	registryAddr := HostPort(app.Flag("registry-addr", "Address of the docker registry for import").Required())

	log.SetOutput(os.Stderr)
	log.SetLevel(log.InfoLevel)

	_, err := app.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse command line: %v.\nUse docker-import --help for help.\n", err)
		return trace.Wrap(err)
	}

	log.Infof("processing files in %s", *tarballDir)
	return bulkImport(*tarballDir, registryAddr.String())
}

// bulkImport imports all container images from dir into the docker registry
// specified with registryAddr.
// It expects the image files to be in the format as written by `docker save -o filename`.
func bulkImport(dir, registryAddr string) error {
	return filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return trace.Wrap(err)
		}
		if fi.IsDir() {
			if path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		log.Infof("processing file %s", path)
		read := func(path string) error {
			var f *os.File
			f, err = os.Open(path)
			if err != nil {
				return trace.Wrap(err, "failed to open tarball `%s` for reading", path)
			}
			defer f.Close()
			err = importImageFromTarball(f, path, registryAddr)
			if err != nil {
				return trace.Wrap(err, "failed to import image from tarball `%s`", path)
			}
			return nil
		}
		if err = read(path); err != nil {
			return trace.Wrap(err)
		}
		return nil
	})
}

// importImageFromTarball imports a container image from archive given in input
// into the docker registry specified with registryAddr.
// path refers to the same archive file and used for `docker load`.
func importImageFromTarball(input io.Reader, path, registryAddr string) error {
	const interval = 5 * time.Second
	const attempts = 6

	log.Infof("importing from tarball %s", path)
	r := tar.NewReader(input)
	var hdr *tar.Header
	var err error
	for {
		hdr, err = r.Next()
		if err != nil {
			if err != io.EOF {
				return trace.Wrap(err)
			}
			return nil
		}
		if hdr.Name == "repositories" {
			data, err := ioutil.ReadAll(r)
			if err != nil {
				return trace.Wrap(err)
			}
			var repo Repo
			if err = json.Unmarshal(data, &repo); err != nil {
				return trace.Wrap(err)
			}
			if err = utils.Retry(context.TODO(), attempts, interval, func() error {
				if err := importWithRepo(repo, path, registryAddr); err != nil {
					return trace.Wrap(err, "failed to import %v into docker: %v, will retry", path, err)
				}
				return nil
			}); err != nil {
				return trace.Wrap(err)
			}
		}
	}

	return nil
}

// importWithRepo imports the actual container image into the docker registry specified
// with registryAddr using repo for metadata.
func importWithRepo(repo Repo, path, registryAddr string) error {
	out, err := dockerCommand("load", "-i", path)
	if err != nil {
		return trace.Wrap(err, "failed to load image into docker:\n%s", out)
	}
	repoTag := fmt.Sprintf("%v/%v", registryAddr, repo.ImageURL())
	out, err = dockerCommand("tag", repo.ImageURL(), repoTag)
	if err != nil {
		return trace.Wrap(err, "failed to tag image in registry:\n%s", out)
	}
	out, err = dockerCommand("push", repoTag)
	if err != nil {
		return trace.Wrap(err, "failed to push image to registry:\n%s", out)
	}
	return nil
}

// dockerCommand executes an arbitrary docker command specified with args.
// Returns the command output upon failure.
func dockerCommand(args ...string) ([]byte, error) {
	out, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		return out, trace.Wrap(err)
	}
	return nil, nil
}

// Repo describes docker container image repository metadata.
type Repo map[string]Version

// Version describes the version part of a container image metadata.
type Version map[string]string

// ImageURL builds a container image repository URL:
//
// repository/image_name:image_version
func (r Repo) ImageURL() string {
	for imagePath, details := range r {
		repoURL, imageName := filepath.Split(imagePath)
		for version, _ := range details {
			return filepath.Join(repoURL, imageTag(imageName, version))
		}
	}
	return ""
}

func imageTag(name, version string) string {
	return fmt.Sprintf("%v:%v", name, version)
}

// HostPort returns an instance of the kingpin Flag to read `host:port` formatted input.
func HostPort(s kingpin.Settings) *hostPort {
	result := new(hostPort)

	s.SetValue(result)
	return result
}

// hostPort is a command line flag that understands input
// as a host:port pair.
type hostPort struct {
	host string
	port int64
}

// Set sets a value into the given HostPort instance.
func (r *hostPort) Set(input string) error {
	var err error
	var port string

	r.host, port, err = net.SplitHostPort(input)
	if err != nil {
		return err
	}

	r.port, err = strconv.ParseInt(port, 0, 0)
	return err
}

// String converts host:port into a string representation.
func (r hostPort) String() string {
	return net.JoinHostPort(r.host, fmt.Sprintf("%v", r.port))
}
