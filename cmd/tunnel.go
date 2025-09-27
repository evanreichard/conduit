package cmd

import (
	"context"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"reichard.io/conduit/config"
	"reichard.io/conduit/store"
	"reichard.io/conduit/tunnel"
)

var tunnelCmd = &cobra.Command{
	Use:   "tunnel <name> <host:port>",
	Short: "Create a conduit tunnel",
	Run: func(cmd *cobra.Command, args []string) {
		// Get Client Config
		cfg, err := config.GetClientConfig(cmd.Flags())
		if err != nil {
			log.Fatal("failed to get client config:", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Create Forwarder
		tunnelForwarder, err := tunnel.NewForwarder(cfg.TunnelTarget, store.NewTunnelStore(100))
		if err != nil {
			log.Fatal("failed to create tunnel forwarder:", err)
		}
		go tunnelForwarder.Start(ctx)

		// Create Tunnel
		tunnel, err := tunnel.NewClientTunnel(cfg, tunnelForwarder)
		if err != nil {
			log.Fatal("failed to create tunnel:", err)
		}
		tunnel.Start(ctx)
	},
}

func init() {
	configDefs := config.GetConfigDefs[config.ClientConfig]()
	for _, d := range configDefs {
		tunnelCmd.Flags().String(d.Key, d.Default, d.Description)
	}
}
