package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"tunnelforge/agent/forge/client"
	"tunnelforge/agent/forge/config"

	"github.com/spf13/cobra"
)

var dialCmd = &cobra.Command{
	Use:   "dial",
	Short: "Create a tunnel to the TunnelForge server",
	Long: `Create one or more tunnels to the TunnelForge server.

Use --config to specify a YAML file with multiple tunnel mappings:

  forge dial --config tunnels.yaml

Or use --subdomain and --local-addr for a single tunnel:

  forge dial --subdomain test-app --local-addr localhost:3000`,

	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, stop := signal.NotifyContext(
			context.Background(),
			os.Interrupt,
			syscall.SIGTERM,
		)
		defer stop()

		serverAddr, _ := cmd.Flags().GetString("server")
		maxRetryCount, _ := cmd.Flags().GetInt("max-retries")
		configFile, _ := cmd.Flags().GetString("config")

		token := config.GetString("token")
		agentID := config.GetString("agent_id")

		tunnels, err := loadTunnels(cmd, configFile)
		if err != nil {
			return err
		}

		c := client.New(
			serverAddr,
			token,
			agentID,
			maxRetryCount,
			tunnels,
		)

		return c.Run(ctx)
	},
}

// loadTunnels builds the subdomain → local address map from either
// a YAML config file (--config) or CLI flags (--subdomain + --local-addr).
func loadTunnels(cmd *cobra.Command, configFile string) (map[string]string, error) {
	if configFile != "" {
		cfg, err := config.LoadTunnelConfig(configFile)
		if err != nil {
			return nil, fmt.Errorf("loading tunnel config: %w", err)
		}
		return cfg.TunnelMap(), nil
	}

	// Fallback: single tunnel from CLI flags
	subdomain, _ := cmd.Flags().GetString("subdomain")
	localAddr, _ := cmd.Flags().GetString("local-addr")

	if subdomain == "" || localAddr == "" {
		return nil, fmt.Errorf("either --config or both --subdomain and --local-addr are required")
	}

	return map[string]string{subdomain: localAddr}, nil
}

func init() {
	rootCmd.AddCommand(dialCmd)
	dialCmd.Flags().StringP("server", "s", "localhost:7000", "Server address")
	dialCmd.Flags().StringP("subdomain", "d", "", "Subdomain (single tunnel mode)")
	dialCmd.Flags().StringP("local-addr", "l", "", "Local address (single tunnel mode)")
	dialCmd.Flags().StringP("config", "c", "", "Path to tunnel config YAML file")
	dialCmd.Flags().IntP("max-retries", "r", 10, "Maximum number of retries")
}
