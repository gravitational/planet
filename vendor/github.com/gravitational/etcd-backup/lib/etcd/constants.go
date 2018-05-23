package etcd

import "time"

const (
	// FileVersion is the version of the backup file we produce.
	FileVersion = "1"

	// DefaultMinRestoreTTL is the minimum TTL to restore to the etcd cluster
	// https://github.com/coreos/etcd/blob/6dcd020d7da9730caf261a46378dce363c296519/lease/lessor.go#L34
	DefaultMinRestoreTTL = 5 * time.Second
)
