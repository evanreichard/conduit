package cmd

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"reichard.io/conduit/config"
	"reichard.io/conduit/store"
	"reichard.io/conduit/tunnel"
	"reichard.io/conduit/web"
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

		var wg sync.WaitGroup
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Create Tunnel Store
		tunnelStore := store.NewTunnelStore(100)

		// Create Forwarder
		tunnelForwarder, err := tunnel.NewForwarder(cfg.TunnelTarget, tunnelStore)
		if err != nil {
			log.Fatal("failed to create tunnel forwarder:", err)
		}
		wg.Go(func() { tunnelForwarder.Start(ctx) })

		// Create Tunnel
		tunnel, err := tunnel.NewClientTunnel(cfg, tunnelForwarder)
		if err != nil {
			log.Fatal("failed to create tunnel:", err)
		}
		wg.Go(func() { tunnel.Start(ctx) })

		// Create Server
		webServer := web.NewWebServer(tunnelStore)
		wg.Go(func() { webServer.Start(ctx) })

		// Wait Interrupt
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		log.Println("Shutting Down...")
		cancel()
		wg.Wait()
	},
}

func init() {
	configDefs := config.GetConfigDefs[config.ClientConfig]()
	for _, d := range configDefs {
		tunnelCmd.Flags().String(d.Key, d.Default, d.Description)
	}
}
