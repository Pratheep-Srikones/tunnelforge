package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"tunnelforge/agent/forge/client"
	"tunnelforge/agent/forge/config"
	"tunnelforge/internal/proto"

	"github.com/spf13/cobra"
)

// upCmd represents the up command
var upCmd = &cobra.Command{
	Use:     "up [subdomain]",
	Aliases: []string{"dial"},
	Short:   "Start one or more tunnels to the TunnelForge server",
	Long: `Start one or more reverse tunnels to the TunnelForge server.

Examples:
  # Start a single tunnel with flags:
  forge up test-app --to 3000
  forge up my-api --to localhost:8080

  # Start all tunnels defined in tunnels.yaml (or specified by --config):
  forge up
  forge up --config path/to/tunnels.yaml

  # Start a specific tunnel defined in tunnels.yaml:
  forge up test-app --config tunnels.yaml`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, stop := signal.NotifyContext(
			context.Background(),
			os.Interrupt,
			syscall.SIGTERM,
		)
		defer stop()

		serverAddr, _ := cmd.Flags().GetString("server")
		if serverAddr == "" {
			serverAddr = config.GetString("server")
		}

		if serverAddr == "" {
			return fmt.Errorf("server address not found. Please run 'forge register' first")
		}
		maxRetryCount, _ := cmd.Flags().GetInt("max-retries")

		token, _ := cmd.Flags().GetString("token")
		if token == "" {
			token = config.GetString("token")
		}

		agentID, _ := cmd.Flags().GetString("agent-id")
		if agentID == "" {
			agentID = config.GetString("agent_id")
		}

		if token == "" || agentID == "" {
			return fmt.Errorf("agent credentials not found (token or agent_id is empty). Please run 'forge register' first")
		}

		tunnels, err := resolveTunnels(cmd, args)
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

// resolveTunnels builds the subdomain → local address mapping from either CLI flags,
// positional arguments, or a YAML configuration file.
func resolveTunnels(cmd *cobra.Command, args []string) (map[string]string, error) {
	configFile, _ := cmd.Flags().GetString("config")
	toAddr, _ := cmd.Flags().GetString("to")
	localAddr, _ := cmd.Flags().GetString("local-addr")
	subdomainFlag, _ := cmd.Flags().GetString("subdomain")

	destAddr := toAddr
	if destAddr == "" {
		destAddr = localAddr
	}

	var subdomain string
	if len(args) > 0 && args[0] != "" {
		subdomain = args[0]
	} else if subdomainFlag != "" {
		subdomain = subdomainFlag
	}

	// 1. Single tunnel via CLI (e.g. forge up test-app --to 3000 or --subdomain test-app --local-addr localhost:3000)
	if subdomain != "" && destAddr != "" {
		if err := proto.ValidateSubdomain(subdomain); err != nil {
			return nil, fmt.Errorf("invalid subdomain %q: %w", subdomain, err)
		}
		return map[string]string{subdomain: normalizeLocalAddr(destAddr)}, nil
	}

	// 2. From config file (either --config or default tunnels.yaml)
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

		// 2a. Single named tunnel from config (e.g. forge up test-app)
		if subdomain != "" {
			entry, ok := cfg.Tunnels[subdomain]
			if !ok {
				return nil, fmt.Errorf("tunnel %q not found in %s (available: %v)", subdomain, targetConfig, cfg.Subdomains())
			}
			return map[string]string{subdomain: normalizeLocalAddr(entry.Local)}, nil
		}

		// 2b. All tunnels from config (e.g. forge up)
		tunnelMap := make(map[string]string, len(cfg.Tunnels))
		for sub, entry := range cfg.Tunnels {
			tunnelMap[sub] = normalizeLocalAddr(entry.Local)
		}
		return tunnelMap, nil
	}

	// 3. Subdomain provided but missing destination and no config
	if subdomain != "" {
		return nil, fmt.Errorf("missing destination address for tunnel %q. Specify with '--to <port>' (e.g. 'forge up %s --to 3000') or configure in tunnels.yaml", subdomain, subdomain)
	}

	// 4. No config and no flags
	return nil, fmt.Errorf("no tunnels specified. Provide a tunnel with 'forge up <name> --to <port>' or provide a config file with '--config tunnels.yaml'")
}

// normalizeLocalAddr converts inputs like "3000" or ":3000" to "localhost:3000".
func normalizeLocalAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	if _, err := strconv.Atoi(addr); err == nil {
		return "localhost:" + addr
	}
	if strings.HasPrefix(addr, ":") {
		return "localhost" + addr
	}
	return addr
}

func init() {
	rootCmd.AddCommand(upCmd)

	upCmd.Flags().StringP("server", "s", "", "Server address")
	upCmd.Flags().StringP("to", "t", "", "Local destination port or address (e.g. 3000, localhost:3000)")
	upCmd.Flags().StringP("local-addr", "l", "", "Local address (alias for --to)")
	upCmd.Flags().StringP("subdomain", "d", "", "Subdomain for single tunnel mode")
	upCmd.Flags().StringP("config", "c", "", "Path to tunnel config YAML file")
	upCmd.Flags().IntP("max-retries", "r", 10, "Maximum number of reconnect retries")
	upCmd.Flags().String("token", "", "Agent auth token (defaults to saved config)")
	upCmd.Flags().String("agent-id", "", "Agent ID (defaults to saved config)")
}
