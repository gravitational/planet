package etcdexport

import (
	"context"
	"time"

	etcdconf "github.com/gravitational/coordinate/config"
	"github.com/gravitational/etcd-backup/lib/etcd"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var restoreCmd = &cobra.Command{
	Use:   "restore [file]",
	Short: "restore etcd datastore",
	Long:  ``,
	Args:  cobra.ExactArgs(1),
	RunE:  restore,
}

var (
	restoreTimeout time.Duration
	restorePrefix  []string
	migratePrefix  []string
	minRestoreTTL  time.Duration
)

func init() {
	restoreCmd.Flags().DurationVarP(&restoreTimeout, "timeout", "", 2*time.Minute, "Cancel the restore if it takes too long")
	restoreCmd.Flags().DurationVarP(&minRestoreTTL, "min-restore-ttl", "", 5*time.Second, "Don't restore key's that are about to expire")
	restoreCmd.Flags().StringSliceVarP(&restorePrefix, "prefix", "", []string{"/"}, "The key prefix to restore")
	restoreCmd.Flags().StringSliceVarP(&migratePrefix, "migrate", "", nil, "The key prefix to migrate from v2 to v3")
	rootCmd.AddCommand(restoreCmd)
}

func restore(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), restoreTimeout)
	defer cancel()

	err := etcd.Restore(ctx, etcd.RestoreConfig{
		EtcdConfig: etcdconf.Config{
			Endpoints: endpoints,
			CAFile:    caFile,
			CertFile:  certFile,
			KeyFile:   keyFile,
		},
		Timeout:       restoreTimeout,
		Prefix:        restorePrefix,
		MigratePrefix: migratePrefix,
		File:          args[0],
		MinRestoreTTL: minRestoreTTL,
		Log:           log.New(),
	})
	if err != nil {
		return trace.Wrap(err)
	}
	return nil

}
