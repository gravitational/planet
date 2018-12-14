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

package e2e

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"github.com/gravitational/trace"
)

type Config struct {
	KubeMasterAddr string
	KubeRepoPath   string
	AssetDir       string
}

// RunTests runs e2e tests using ginkgo as a test runner.
// extraArgs may specify additional arguments to the test runner.
func RunTests(config *Config, extraArgs []string) error {
	var args []string
	var cmd *exec.Cmd
	var binDir string
	var asset = func(app string) string {
		return filepath.Join(binDir, app)
	}
	var err error
	var kubeConfig string

	if config.AssetDir != "" {
		binDir = config.AssetDir
	} else {
		binDir, _ = filepath.Split(os.Args[0])
	}

	kubeConfig, err = createKubeConfig(config)
	if err != nil {
		return trace.Wrap(err, "failed to create kubeconfig")
	}
	defer os.Remove(kubeConfig)

	args = append(args, extraArgs...)
	args = append(args, asset("e2e.test"))
	args = append(args, "--") // pass arguments to test executable
	args = append(args, []string{
		"--provider=planet",
		fmt.Sprintf("-kubeconfig=%s", kubeConfig),
		fmt.Sprintf("-repo-root=%s", config.KubeRepoPath),
	}...)
	cmd = exec.Command(asset("ginkgo"), args...)

	// redirect test runner output directly to owning terminal
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	err = cmd.Run()

	return err
}

func createKubeConfig(config *Config) (string, error) {
	const kubeSample = `apiVersion: v1
clusters:
- cluster:
    server: {{.KubeMasterAddr}}
  name: planet
contexts:
- context:
    cluster: planet
    user: ""
  name: planet
current-context: planet
kind: Config
preferences: {}
users: []`
	var f *os.File
	var err error
	var tmpl = template.Must(template.New("kube").Parse(kubeSample))
	var b = new(bytes.Buffer)

	tmpl.Execute(b, config)
	f, err = ioutil.TempFile("", "planet")
	if err != nil {
		return "", trace.Wrap(err, "failed to create temp file")
	}
	defer f.Close()

	if _, err = f.Write(b.Bytes()); err != nil {
		return "", trace.Wrap(err, "failed to write kubeconfig")
	}

	return f.Name(), nil
}
