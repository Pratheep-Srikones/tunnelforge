/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"
	"sort"
	"tunnelforge/agent/forge/config"

	"github.com/spf13/cobra"
)

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status and routing details of the TunnelForge agent",
	Long: `Display the current authentication status, connected server details,
agent ID, and active tunnel routing details configured for the TunnelForge agent.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		server := config.GetString("server")
		agent_id := config.GetString("agent_id")
		token := config.GetString("token")
		if token == "" {
			fmt.Println("Not Authenticated\nPlease run 'forge register' first")
			return nil
		}
		fmt.Println("Authenticated")
		fmt.Printf("Connected Server: %s\n", server)
		fmt.Printf("Agent ID: %s\n", agent_id)

		tunnels, err := getRoutingDetails(cmd)
		if err != nil {
			return err
		}

		if len(tunnels) == 0 {
			fmt.Println("Routing Details: None")
		} else {
			fmt.Println("Routing Details:")
			subdomains := make([]string, 0, len(tunnels))
			for sub := range tunnels {
				subdomains = append(subdomains, sub)
			}
			sort.Strings(subdomains)
			for _, sub := range subdomains {
				fmt.Printf("  %s -> %s\n", sub, tunnels[sub])
			}
		}
		return nil
	},
}

// getRoutingDetails reads the tunnel configuration (from either the config
// file or the global config) and returns a map of subdomain to normalized
// local address.
func getRoutingDetails(cmd *cobra.Command) (map[string]string, error) {
	configFile, _ := cmd.Flags().GetString("config")

	targetConfig := configFile
	if targetConfig == "" {
		if _, err := os.Stat("tunnels.yaml"); err == nil {
			targetConfig = "tunnels.yaml"
		}
	}

	if targetConfig != "" {
		cfg, err := config.LoadTunnelConfig(targetConfig)
		if err != nil {
			return nil, fmt.Errorf("loading tunnel config %q: %w", targetConfig, err)
		}
		tunnelMap := make(map[string]string, len(cfg.Tunnels))
		for sub, entry := range cfg.Tunnels {
			tunnelMap[sub] = normalizeLocalAddr(entry.Local)
		}
		return tunnelMap, nil
	}

	// if no config file is provided, try to load from global config.yaml file
	if rawTunnels := config.Get("tunnels"); rawTunnels != nil {
		if m, ok := rawTunnels.(map[string]any); ok {
			tunnelMap := make(map[string]string)
			for sub, val := range m {
				if entryMap, ok := val.(map[string]any); ok {
					if local, ok := entryMap["local"].(string); ok {
						tunnelMap[sub] = normalizeLocalAddr(local)
					}
				} else if strVal, ok := val.(string); ok {
					tunnelMap[sub] = normalizeLocalAddr(strVal)
				}
			}
			if len(tunnelMap) > 0 {
				return tunnelMap, nil
			}
		}
	}

	return nil, nil
}

func init() {
	rootCmd.AddCommand(statusCmd)

	statusCmd.Flags().StringP("config", "c", "", "Path to tunnel config YAML file")
}
