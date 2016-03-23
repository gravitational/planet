package etcd

import (
	"crypto/tls"
	"net"
	"net/http"
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
	transport, err := r.newHttpTransport()
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

// ClientConfig creates a TLS client configuration for an HTTP transport
func (r *Config) ClientConfig() (*tls.Config, error) {
	info := transport.TLSInfo{
		CertFile: r.CertFile,
		KeyFile:  r.KeyFile,
		CAFile:   r.CAFile,
	}
	config, err := info.ClientConfig()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return config, nil
}

func (r *Config) newHttpTransport() (*http.Transport, error) {
	config, err := r.ClientConfig()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		Dial: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 10 * time.Second,
		MaxIdleConnsPerHost: 500,
		TLSClientConfig:     config,
	}

	return transport, nil
}
