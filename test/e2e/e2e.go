package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type Config struct {
	// Directory with test runner and test executable
	ToolDir        string
	KubeMasterAddr string
	KubeRepoPath   string
	KubeConfig     string
	NumNodes       int
}

// RunTests runs e2e tests using ginkgo as a test runner.
// The test executable is hardcoded and expected to be in toolDir.
// extraArgs may specify additional arguments to the test runner.
func RunTests(config *Config, extraArgs []string) error {
	var args []string
	var cmd *exec.Cmd
	var asset = func(app string) string {
		return filepath.Join(config.ToolDir, app)
	}
	var err error

	args = append(args, extraArgs...)
	args = append(args, asset("e2e.test"))
	args = append(args, "--") // pass arguments to test executable
	args = append(args, []string{
		"--provider=planet",
		fmt.Sprintf("--host=%s", config.KubeMasterAddr),
		fmt.Sprintf("--num-nodes=%d", config.NumNodes),
		fmt.Sprintf("--kubeconfig=%s", config.KubeConfig),
		fmt.Sprintf("--repo-root=%s", config.KubeRepoPath),
	}...)
	cmd = exec.Command(asset("ginkgo"), args...)

	// redirect test runner output directly to owning terminal
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	err = cmd.Run()

	return err
}
