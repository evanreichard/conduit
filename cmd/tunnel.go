package cmd

import (
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"reichard.io/conduit/client"
	"reichard.io/conduit/config"
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

		// Create Tunnel
		tunnel, err := client.NewTunnel(cfg)
		if err != nil {
			log.Fatal("failed to create tunnel:", err)
		}
		tunnel.Start()
	},
}

func init() {
	configDefs := config.GetConfigDefs[config.ClientConfig]()
	for _, d := range configDefs {
		tunnelCmd.Flags().String(d.Key, d.Default, d.Description)
	}
}
