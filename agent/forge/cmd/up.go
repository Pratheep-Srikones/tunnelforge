package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"tunnelforge/agent/forge/capture"
	"tunnelforge/agent/forge/client"
	"tunnelforge/agent/forge/config"
	"tunnelforge/agent/forge/replay"
	"tunnelforge/agent/forge/ui"

	"github.com/spf13/cobra"
)

// upCmd represents the up command
var upCmd = &cobra.Command{
	Use:     "up [subdomain]",
	Aliases: []string{"dial"},
	Short:   "Start one or more tunnels to the TunnelForge server",
	Long: `Start one or more reverse tunnels to the TunnelForge server.

Examples:

  # Start tunnels defined in tunnels.yaml (or specified by --config):
  forge up # to use default tunnels.yaml in current directory
  forge up --config path/to/tunnels.yaml`,
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

		tunnels, err := resolveTunnels(cmd)
		if err != nil {
			return err
		}

		ringBuffer := capture.NewRingBuffer(capture.DefaultMaxRequests)
		for sub, entry := range tunnels {
			if entry.CaptureLimit > 0 {
				ringBuffer.SetLimit(sub, entry.CaptureLimit)
			}
		}

		uiServer := ui.NewServer("4040", ringBuffer, tunnels, replay.NewReplayer(client.GetHTTPClient(true, 10*time.Second)))
		go func() {
			if err := uiServer.Start(); err != nil && err != http.ErrServerClosed {
				fmt.Printf("[UI] Server error: %v\n", err)
			}
		}()

		go func() {
			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = uiServer.Stop(shutdownCtx)
		}()

		caCert, _ := cmd.Flags().GetString("ca-cert")
		insecure, _ := cmd.Flags().GetBool("insecure")
		forceWS, _ := cmd.Flags().GetBool("ws")

		c := client.New(
			serverAddr,
			token,
			agentID,
			maxRetryCount,
			tunnels,
			ringBuffer,
		).WithBroadcaster(uiServer.Hub).
			WithCACert(caCert).
			WithInsecureSkipTLS(insecure).
			WithWebSocket(forceWS)

		return c.Run(ctx)
	},
}

// resolveTunnels builds the subdomain → local address mapping from either CLI flags,
// positional arguments, or a YAML configuration file.
func resolveTunnels(cmd *cobra.Command) (map[string]config.TunnelEntry, error) {
	tunnelConfigFile, _ := cmd.Flags().GetString("config")

	if tunnelConfigFile != "" {
		cfg, err := config.LoadTunnelConfig(tunnelConfigFile)
		if err != nil {
			return nil, fmt.Errorf("loading tunnel config %q: %w", tunnelConfigFile, err)
		}

		absPath, err := filepath.Abs(tunnelConfigFile)
		if err == nil {
			_ = config.Set("active_config", absPath)
		}

		tunnelMap := make(map[string]config.TunnelEntry, len(cfg.Tunnels))
		for sub, entry := range cfg.Tunnels {
			entry.Local = normalizeLocalAddr(entry.Local)
			tunnelMap[sub] = entry
		}
		return tunnelMap, nil
	}
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
	upCmd.Flags().StringP("config", "c", "./tunnels.yaml", "Path to tunnel config YAML file")
	upCmd.Flags().IntP("max-retries", "r", 10, "Maximum number of reconnect retries")
	upCmd.Flags().String("token", "", "Agent auth token (defaults to saved config)")
	upCmd.Flags().String("agent-id", "", "Agent ID (defaults to saved config)")
	upCmd.Flags().String("ca-cert", "", "Path to custom CA certificate file (defaults to embedded CA)")
	upCmd.Flags().Bool("insecure", false, "Disable TLS encryption for agent-server communication")
	upCmd.Flags().Bool("ws", false, "Force WebSocket transport fallback (bypasses port 7000)")

	if err := upCmd.MarkFlagRequired("config"); err != nil {
		panic(err)
	}
}
