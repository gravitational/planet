package monitoring

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
)

const healthzCheckTimeout = 1 * time.Second

var errHealthzCheck = errors.New("failed healthz check")

type checkerFunc func(response io.Reader) error

// monitoring.checker
type httpHealthzChecker struct {
	URL         string
	client      *http.Client
	checkerFunc checkerFunc
}

func (r *httpHealthzChecker) check(reporter reporter) {
	resp, err := r.client.Get(r.URL)
	if err != nil {
		reporter.add(err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		reporter.add(fmt.Errorf("unexpected HTTP status: %s", http.StatusText(resp.StatusCode)))
		return
	}
	err = r.checkerFunc(resp.Body)
	if err != nil {
		reporter.add(err)
	} else {
		reporter.addProbe(&pb.Probe{
			Status: pb.ServiceStatusType_ServiceRunning,
		})
	}
}

func newHTTPHealthzChecker(URL string, checkerFunc checkerFunc) checker {
	client := &http.Client{
		Timeout: healthzCheckTimeout,
	}
	return &httpHealthzChecker{
		URL:         URL,
		client:      client,
		checkerFunc: checkerFunc,
	}
}

func newUnixSocketHealthzChecker(URL, socketPath string, checkerFunc checkerFunc) checker {
	client := &http.Client{
		Timeout: healthzCheckTimeout,
		Transport: &http.Transport{
			Dial: func(network, addr string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
	}
	return &httpHealthzChecker{
		URL:         URL,
		client:      client,
		checkerFunc: checkerFunc,
	}
}
