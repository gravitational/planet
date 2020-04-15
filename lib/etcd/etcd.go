package etcd

import (
	"context"
	"time"

	"github.com/gravitational/trace"
	etcd "go.etcd.io/etcd/client"
	"go.etcd.io/etcd/clientv3"
	etcdv3 "go.etcd.io/etcd/clientv3"
	"go.etcd.io/etcd/pkg/transport"
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
	// DialTimeout is dial timeout
	DialTimeout time.Duration
}

// NewClient creates a new instance of an Etcd client
func (r *Config) NewClient() (etcd.Client, error) {
	if err := r.checkAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	info := transport.TLSInfo{
		CertFile:      r.CertFile,
		KeyFile:       r.KeyFile,
		TrustedCAFile: r.CAFile,
	}
	transport, err := transport.NewTransport(info, 30*time.Second)
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

// NewClientV3 creates a new instance of an etcdv3 client
func (r *Config) NewClientV3() (*etcdv3.Client, error) {
	if err := r.checkAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	info := transport.TLSInfo{
		CertFile:      r.CertFile,
		KeyFile:       r.KeyFile,
		TrustedCAFile: r.CAFile,
	}
	tlsConfig, err := info.ClientConfig()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	client, err := etcdv3.New(etcdv3.Config{
		Endpoints:   r.Endpoints,
		TLS:         tlsConfig,
		DialTimeout: r.DialTimeout,
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return client, nil
}

func (r *Config) checkAndSetDefaults() error {
	if len(r.Endpoints) == 0 {
		return trace.BadParameter("need at least one endpoint")
	}
	if r.HeaderTimeoutPerRequest == 0 {
		r.HeaderTimeoutPerRequest = defaultResponseTimeout
	}
	if r.DialTimeout == 0 {
		r.HeaderTimeoutPerRequest = defaultResponseTimeout
	}
	return nil
}

// PutNoExist puts the value at the given key iff the key did not exist
func PutNoExist(ctx context.Context, c *clientv3.Client, key, value string) error {
	op := etcdv3.OpPut(key, value)
	cond := etcdv3.Compare(etcdv3.Version(key), "=", 0)
	_, err := c.Txn(ctx).If(cond).Then(op).Commit()
	if err != nil {
		return ConvertError(err)
	}
	return nil
}

// ConvertError converts the specified etcd error to trace type hierarchy
func ConvertError(err error) error {
	if err == nil {
		return nil
	}
	switch err := err.(type) {
	case *etcd.ClusterError:
		return trace.Wrap(err, err.Detail())
	case etcd.Error:
		switch err.Code {
		case etcd.ErrorCodeKeyNotFound:
			return trace.NotFound(err.Error())
		case etcd.ErrorCodeNotFile:
			return trace.BadParameter(err.Error())
		case etcd.ErrorCodeNodeExist:
			return trace.AlreadyExists(err.Error())
		case etcd.ErrorCodeTestFailed:
			return trace.CompareFailed(err.Error())
		}
	}
	return err
}

const (
	// defaultResponseTimeout specifies the default time limit to wait for response
	// header in a single request made by an etcd client
	defaultResponseTimeout = 1 * time.Second
	// defaultDialTimeout is default TCP connect timeout
	defaultDialTimeout = 30 * time.Second
)
