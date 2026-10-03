/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"
	"os"
	"time"
	"tunnelforge/agent/forge/client"
	"tunnelforge/agent/forge/config"
	"tunnelforge/internal/proto"

	"github.com/spf13/cobra"
)

// registerCmd represents the register command
var registerCmd = &cobra.Command{
	Use:   "register",
	Short: "Register the tunnelforge agent and get agent id and token",
	Long: `Register the tunnelforge agent with the tunnelforge agent get the 
	authentication token and agent id and save them in the config file`,
	Example: `
	   forge register --server <SERVER_URL_OR_IP_ADDRESS> --enrollment-key <ENROLLMENT_KEY>
	   forge register --server https://tunnel.yourdomain.com --enrollment-key tf_enroll_xyz123
	`,
	RunE: func(cmd *cobra.Command, args []string) error {
		key, _ := cmd.Flags().GetString("enrollment-key")
		if !cmd.Flags().Changed("enrollment-key") {
			if envKey := os.Getenv("FORGE_ENROLLMENT_KEY"); envKey != "" {
				key = envKey
			}
		}
		serverAddr, _ := cmd.Flags().GetString("server")

		rc := client.NewRESTClient(client.GetHTTPClient(true, 10*time.Second), serverAddr)
		req := proto.RegisterRequest{EnrollmentKey: key}

		var res proto.RegisterResponse
		err := rc.Post(context.Background(), "/forge/internal/auth/register", &req, &res)
		if err != nil {
			return err
		}

		if err := config.Set("agent_id", res.AgentID); err != nil {
			return fmt.Errorf("failed to save agent_id to config: %w", err)
		}
		if err := config.Set("token", res.Token); err != nil {
			return fmt.Errorf("failed to save token to config: %w", err)
		}
		if err := config.Set("server", serverAddr); err != nil {
			return fmt.Errorf("failed to save server to config: %w", err)
		}

		fmt.Printf("[Register] Success!\nToken: %s\nAgent ID: %s\nConfig saved.\n", "********", res.AgentID)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(registerCmd)

	registerCmd.Flags().StringP("enrollment-key", "e", "tf_enroll_xyz123", "Enrollmet key for agent registration")
	registerCmd.Flags().StringP("server", "s", "http://localhost:8000", "Server address")
}
