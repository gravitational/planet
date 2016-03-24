package etcdconf

import (
	"time"

	"github.com/gravitational/trace"

	etcd "github.com/coreos/etcd/client"
	"github.com/coreos/etcd/pkg/transport"
)

// Config defines the configuration to access etcd
type Config struct {
	// Endpoints lists etcd server endpoints (http://host:port)
	Endpoints []string
	// CAFile defines the SSL Certificate Authority file to used
	// to secure etcd communication
	CAFile string
	// CertFile defines the SSL certificate file to use to secure
	// etcd communication
	CertFile string
	// KeyFile defines the SSL key file to use to secure etcd communication
	KeyFile string
	// HeaderTimeoutPerRequest specifies the time limit to wait for response
	// header in a single request made by a client
	HeaderTimeoutPerRequest time.Duration
}

// NewClient creates a new instance of an etcd client
func (r *Config) NewClient() (etcd.Client, error) {
	info := transport.TLSInfo{
		CertFile: r.CertFile,
		KeyFile:  r.KeyFile,
		CAFile:   r.CAFile,
	}
	transport, err := transport.NewTransport(info)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	client, err := etcd.New(etcd.Config{
		Endpoints:               r.Endpoints,
		Transport:               transport,
		HeaderTimeoutPerRequest: r.HeaderTimeoutPerRequest,
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return client, nil
}
