package cmd

import (
	"reichard.io/conduit/config"
	"reichard.io/conduit/server"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the conduit server",
	Long:  `Start the conduit server to handle incoming tunnel requests`,
	Run: func(cmd *cobra.Command, args []string) {
		// Get Server Config
		cfg, err := config.GetServerConfig(cmd.Flags())
		if err != nil {
			log.Fatal("failed to get server config:", err)
		}

		// Create Server
		srv, err := server.NewServer(cfg)
		if err != nil {
			log.Fatal("failed to create server:", err)
		}

		// Start Server
		if err := srv.Start(); err != nil {
			log.Fatal("failed to start server:", err)
		}
	},
}

func init() {
	configDefs := config.GetConfigDefs[config.ServerConfig]()
	for _, d := range configDefs {
		serveCmd.Flags().String(d.Key, d.Default, d.Description)
	}
}
