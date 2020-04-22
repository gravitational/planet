package leadership

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gravitational/planet/lib/leadership"
	"github.com/gravitational/planet/test/internal/etcd"
	"google.golang.org/grpc/grpclog"

	"github.com/gravitational/trace"
	"github.com/sirupsen/logrus"
)

func TestCandidateToleratesClusterFailure(version string) (err error) {
	logrus.SetOutput(os.Stderr)
	logrus.SetLevel(logrus.DebugLevel)

	const (
		etcdPort = "22379"
		term     = 10 * time.Second
	)

	ctx := context.TODO()
	dataDir, err := ioutil.TempDir("", "etcd")
	if err != nil {
		return trace.ConvertSystemError(err)
	}
	defer os.RemoveAll(dataDir)
	// setup
	c := etcd.Config{
		DataDir:       dataDir,
		Port:          etcdPort,
		Version:       version,
		ContainerName: "etcd-leadership-test",
		Image:         "gcr.io/etcd-development/etcd",
	}
	if err := c.Start(ctx); err != nil {
		return trace.Wrap(err)
	}
	client, err := etcd.GetClientV3(etcdPort)
	if err != nil {
		return trace.Wrap(err)
	}
	defer client.Close()
	candidate1, err := leadership.NewCandidate(ctx, leadership.CandidateConfig{
		Term:   term,
		Prefix: "testleadership",
		Name:   "candidate1",
		Client: client,
	})
	if err != nil {
		return trace.Wrap(err)
	}
	candidate1.Start()
	defer candidate1.Stop()
	candidate2, err := leadership.NewCandidate(ctx, leadership.CandidateConfig{
		Term:   term,
		Prefix: "testleadership",
		Name:   "candidate2",
		Client: client,
	})
	if err != nil {
		return trace.Wrap(err)
	}
	candidate2.Start()
	defer candidate2.Stop()

	respChan := make(chan string, 2)
	go func() {
		for leader := range candidate1.LeaderChan() {
			respChan <- leader
		}
	}()
	go drainChan(candidate2.LeaderChan())

	// exercise
	// Await leader election
	select {
	case leader := <-respChan:
		fmt.Println("Elected: ", leader)
	case <-ctx.Done():
		err = ctx.Err()
		return trace.Wrap(err)
	}

	// Shut down etcd to exercise down time
	if err := c.Stop(ctx); err != nil {
		return trace.Wrap(err)
	}
	time.Sleep(20 * time.Second)

	// Restart etcd and capture new leader
	if err := c.Start(ctx); err != nil {
		return trace.Wrap(err)
	}

	select {
	case leader := <-respChan:
		fmt.Println("Reelected: ", leader)
	case <-ctx.Done():
		err = ctx.Err()
		return trace.Wrap(err)
	}
	return nil
}

// InitGRPCLoggerFromEnvironment configures the GRPC logger if any of the related environment variables
// are set.
func InitGRPCLoggerFromEnvironment() {
	const (
		envSeverityLevel  = "GRPC_GO_LOG_SEVERITY_LEVEL"
		envVerbosityLevel = "GRPC_GO_LOG_VERBOSITY_LEVEL"
	)
	severityLevel := os.Getenv(envSeverityLevel)
	verbosityLevel := os.Getenv(envVerbosityLevel)
	var verbosity int
	if verbosityOverride, err := strconv.Atoi(verbosityLevel); err == nil {
		verbosity = verbosityOverride
	}
	if severityLevel == "" && verbosityLevel == "" {
		// Nothing to do
		return
	}
	InitGRPCLogger(severityLevel, verbosity)
}

// InitGRPCLoggerWithDefaults configures the GRPC logger with debug defaults.
func InitGRPCLoggerWithDefaults() {
	InitGRPCLogger("info", 10)
}

// Severity level is one of `info`, `warning` or `error` and defaults to error if unspecified.
// Verbosity is a non-negative integer.
func InitGRPCLogger(severityLevel string, verbosity int) {
	errorW := ioutil.Discard
	warningW := ioutil.Discard
	infoW := ioutil.Discard

	switch strings.ToLower(severityLevel) {
	case "", "error": // If env is unset, set level to `error`.
		errorW = os.Stderr
	case "warning":
		warningW = os.Stderr
	case "info":
		infoW = os.Stderr
	}

	grpclog.SetLoggerV2(grpclog.NewLoggerV2WithVerbosity(infoW, warningW, errorW, verbosity))
}

func drainChan(ch <-chan string) {
	for range ch {
	}
}
