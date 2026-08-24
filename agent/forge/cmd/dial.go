package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"tunnelforge/agent/forge/client"

	"github.com/spf13/cobra"
)

var dialCmd = &cobra.Command{
	Use:   "dial",
	Short: "Create a tunnel to the TunnelForge server",

	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, stop := signal.NotifyContext(
			context.Background(),
			os.Interrupt,
			syscall.SIGTERM,
		)
		defer stop()

		serverAddr, _ := cmd.Flags().GetString("server")
		token, _ := cmd.Flags().GetString("token")
		subdomain, _ := cmd.Flags().GetString("subdomain")
		localAddr, _ := cmd.Flags().GetString("local-addr")
		maxRetryCount, _ := cmd.Flags().GetInt("max-retries")

		c := client.New(
			serverAddr,
			token,
			subdomain,
			localAddr,
			maxRetryCount,
		)

		return c.Run(ctx)
	},
}

func init() {
	rootCmd.AddCommand(dialCmd)
	dialCmd.Flags().StringP("server", "s", "localhost:7000", "Server address")
	dialCmd.Flags().StringP("token", "t", "abc", "Token")
	dialCmd.Flags().StringP("subdomain", "d", "test-app", "Subdomain")
	dialCmd.Flags().StringP("local-addr", "l", "localhost:3000", "Local address")
	dialCmd.Flags().IntP("max-retries", "r", 10, "Maximum number of retries")
}
