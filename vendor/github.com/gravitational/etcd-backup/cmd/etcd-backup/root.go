package etcdexport

import (
	"fmt"
	"os"

	etcdv2 "github.com/coreos/etcd/client"
	etcdv3 "github.com/coreos/etcd/clientv3"
	etcdconf "github.com/gravitational/coordinate/config"
	"github.com/gravitational/trace"
	"github.com/spf13/cobra"
)

var (
	endpoints []string
	caFile    string
	certFile  string
	keyFile   string
)

var rootCmd = &cobra.Command{
	Use:   "etcd-export",
	Short: "Backup / Restore etcd data",
	Long:  ``,
}

func init() {
	rootCmd.PersistentFlags().StringSliceVarP(&endpoints, "etcd-servers", "", []string{"http://127.0.0.1:5001"}, "List of etcd servers to connect with (scheme://ip:port), comma separated.")
	rootCmd.PersistentFlags().StringVarP(&caFile, "etcd-cafile", "", "/var/state/root.cert", "SSL Certificate Authority file used to secure etcd communication.")
	rootCmd.PersistentFlags().StringVarP(&certFile, "etcd-certfile", "", "/var/state/etcd.cert", "SSL certification file used to secure etcd communication.")
	rootCmd.PersistentFlags().StringVarP(&keyFile, "etcd-keyfile", "", "/var/state/etcd.key", "SSL key file used to secure etcd communication.")
}

// Execute initializes cobra
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func getClients() (etcdv2.KeysAPI, *etcdv3.Client, error) {
	config := etcdconf.Config{
		Endpoints: endpoints,
		CAFile:    caFile,
		CertFile:  certFile,
		KeyFile:   keyFile,
	}
	err := config.CheckAndSetDefaults()
	if err != nil {
		return nil, nil, trace.Wrap(err)
	}

	clientv2, err := config.NewClient()
	if err != nil {
		return nil, nil, trace.Wrap(err)
	}
	keysv2 := etcdv2.NewKeysAPI(clientv2)

	clientv3, err := config.NewClientV3()
	if err != nil {
		return nil, nil, trace.Wrap(err)
	}

	return keysv2, clientv3, nil
}

type etcdBackupNode struct {
	V2 *etcdv2.Node `json:"v2,omitempty"`
	V3 *KeyValue    `json:"v3,omitempty"`
}

// store information in the backup about the version fo the backup
type backupVersion struct {
	Version string `json:"version"`
}

// KeyValue is a clone of the internal KeyValue from etcd which isn't exported
type KeyValue struct {
	// key is the key in bytes. An empty key is not allowed.
	Key []byte `protobuf:"bytes,1,opt,name=key,proto3" json:"key,omitempty"`
	// create_revision is the revision of last creation on this key.
	CreateRevision int64 `protobuf:"varint,2,opt,name=create_revision,json=createRevision,proto3" json:"create_revision,omitempty"`
	// mod_revision is the revision of last modification on this key.
	ModRevision int64 `protobuf:"varint,3,opt,name=mod_revision,json=modRevision,proto3" json:"mod_revision,omitempty"`
	// version is the version of the key. A deletion resets
	// the version to zero and any modification of the key
	// increases its version.
	Version int64 `protobuf:"varint,4,opt,name=version,proto3" json:"version,omitempty"`
	// value is the value held by the key, in bytes.
	Value []byte `protobuf:"bytes,5,opt,name=value,proto3" json:"value,omitempty"`
	// lease is the ID of the lease that attached to key.
	// When the attached lease expires, the key will be deleted.
	// If lease is 0, then no lease is attached to the key.
	Lease int64 `protobuf:"varint,6,opt,name=lease,proto3" json:"lease,omitempty"`
}
