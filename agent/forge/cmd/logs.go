package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"syscall"
	"time"
	"tunnelforge/agent/forge/capture"
	"tunnelforge/agent/forge/ui"

	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
)

// logsCmd represents the logs command
var logsCmd = &cobra.Command{
	Use:   "logs [subdomain]",
	Short: "View or tail request logs captured by the TunnelForge agent",
	Long: `Display captured HTTP request/response logs from a running TunnelForge agent.

Examples:
  # View the last 50 requests across all tunnels:
  forge logs

  # View the last 20 requests for a specific tunnel:
  forge logs myapp -n 20

  # Tail live request logs in real time:
  forge logs -f

  # Stream logs as JSON and pipe to jq:
  forge logs -f -j | jq .`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		follow, _ := cmd.Flags().GetBool("follow")
		jsonOutput, _ := cmd.Flags().GetBool("json")
		limit, _ := cmd.Flags().GetInt("limit")
		port, _ := cmd.Flags().GetString("port")
		if port == "" {
			port = "4040"
		}

		var subdomain string
		if len(args) > 0 {
			subdomain = args[0]
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		return RunLogs(ctx, LogsOptions{
			Port:       port,
			Subdomain:  subdomain,
			Limit:      limit,
			Follow:     follow,
			JSONOutput: jsonOutput,
			Out:        cmd.OutOrStdout(),
			ErrOut:     cmd.ErrOrStderr(),
		})
	},
}

type LogsOptions struct {
	Port       string
	Subdomain  string
	Limit      int
	Follow     bool
	JSONOutput bool
	Out        io.Writer
	ErrOut     io.Writer
}

func RunLogs(ctx context.Context, opts LogsOptions) error {
	baseURL := fmt.Sprintf("http://localhost:%s", opts.Port)

	// Fetch historical logs if limit > 0
	if opts.Limit > 0 {
		entries, err := fetchLogs(ctx, baseURL, opts.Subdomain, opts.Limit)
		if err != nil {
			return fmt.Errorf("could not connect to TunnelForge agent on %s. Is 'forge up' running?\nDetails: %w", baseURL, err)
		}

		for _, entry := range entries {
			if err := printLogEntry(opts.Out, entry, opts.JSONOutput); err != nil {
				return err
			}
		}
	}

	if !opts.Follow {
		return nil
	}

	// Follow live logs over WebSocket
	wsURL := fmt.Sprintf("ws://localhost:%s/ws", opts.Port)
	return followLogs(ctx, wsURL, opts.Subdomain, opts.JSONOutput, opts.Out)
}

func fetchLogs(ctx context.Context, baseURL, subdomain string, limit int) ([]*capture.RequestEntry, error) {
	reqURL := fmt.Sprintf("%s/api/requests?limit=%d", baseURL, limit)
	if subdomain != "" {
		reqURL += fmt.Sprintf("&subdomain=%s", url.QueryEscape(subdomain))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d from API: %s", resp.StatusCode, string(body))
	}

	var entries []*capture.RequestEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("parsing requests JSON: %w", err)
	}

	// Sort chronologically (oldest -> newest) so past logs read naturally before streaming
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})

	return entries, nil
}

func followLogs(ctx context.Context, wsURL, subdomain string, jsonOutput bool, out io.Writer) error {
	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("connecting to live WebSocket stream at %s: %w", wsURL, err)
	}
	defer conn.Close()

	// Ensure connection is closed when context is cancelled
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return nil // Clean exit on Ctrl+C
			}
			return fmt.Errorf("reading live event: %w", err)
		}

		var event ui.UIEvent
		if err := json.Unmarshal(message, &event); err != nil {
			continue
		}

		if event.Type != "request" || event.Entry == nil {
			continue
		}

		if subdomain != "" && event.Subdomain != subdomain {
			continue
		}

		if err := printLogEntry(out, event.Entry, jsonOutput); err != nil {
			return err
		}
	}
}

func printLogEntry(w io.Writer, entry *capture.RequestEntry, jsonOutput bool) error {
	if jsonOutput {
		data, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, string(data))
		return err
	}

	timeStr := entry.Timestamp.Format("15:04:05")
	sub := entry.Subdomain
	if sub == "" {
		sub = "-"
	}

	replayTag := ""
	if entry.Replayed {
		replayTag = " [REPLAY]"
	}

	statusStr := "---"
	if entry.ResponseStatus > 0 {
		statusText := http.StatusText(entry.ResponseStatus)
		if statusText != "" {
			statusStr = strconv.Itoa(entry.ResponseStatus) + " " + statusText
		} else {
			statusStr = strconv.Itoa(entry.ResponseStatus)
		}
	}

	_, err := fmt.Fprintf(w, "%s [%s]%s %s %s %s (%dms)\n",
		timeStr,
		sub,
		replayTag,
		entry.Method,
		entry.URL,
		statusStr,
		entry.DurationMS,
	)
	return err
}

func init() {
	rootCmd.AddCommand(logsCmd)

	logsCmd.Flags().BoolP("follow", "f", false, "Stream live request logs")
	logsCmd.Flags().BoolP("json", "j", false, "Output logs as newline-delimited JSON")
	logsCmd.Flags().IntP("limit", "n", 50, "Number of recent requests to display (0 to skip history)")
	logsCmd.Flags().String("port", "4040", "Port of the local TunnelForge agent UI server")
}
