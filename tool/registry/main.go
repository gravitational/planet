package main

import (
	"context"
	_ "net/http/pprof"
	"os"

	"github.com/gravitational/satellite/lib/ctxgroup"

	"github.com/docker/distribution/registry"
	_ "github.com/docker/distribution/registry/auth/htpasswd"
	_ "github.com/docker/distribution/registry/auth/silly"
	_ "github.com/docker/distribution/registry/auth/token"
	_ "github.com/docker/distribution/registry/proxy"
	_ "github.com/docker/distribution/registry/storage/driver/filesystem"
	_ "github.com/docker/distribution/registry/storage/driver/inmemory"
	_ "github.com/docker/distribution/registry/storage/driver/middleware/redirect"
	_ "github.com/docker/distribution/registry/storage/driver/oss"
	"github.com/spf13/cobra"
)

func main() {
	registry.RootCmd.RemoveCommand(registry.ServeCmd)
	registry.RootCmd.AddCommand(serveCmd)

	if err := registry.RootCmd.Execute(); err != nil {
		os.Exit(255)
	}
}

func init() {
	serveCmd.Flags().StringVarP(&addr, "bind-addr", "", addr, "address to bind on")
	serveCmd.Flags().StringVarP(&config, "config", "", config, "path to the configuration file")
}

// addr specifies the address to bind on
var addr string

// config specifies the path to the configuration file
var config string

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "`serve` stores and distributes Docker images",
	Long:  "`serve` stores and distributes Docker images.",
	RunE: func(cmd *cobra.Command, args []string) error {
		g := ctxgroup.WithContext(context.Background())
		startEndpointsReconciler(addr, &g)

		g.Go(func() error {
			cmd := registry.ServeCmd
			cmd.SetArgs([]string{config})
			return cmd.Execute()
		})
		return g.Wait()
	},
}
