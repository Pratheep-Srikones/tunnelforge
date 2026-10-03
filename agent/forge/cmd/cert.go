package cmd

import (
	"fmt"
	"os"
	"tunnelforge/internal/certs"

	"github.com/spf13/cobra"
)

// certCmd represents the cert command
var certCmd = &cobra.Command{
	Use:   "cert",
	Short: "Retrieve and manage TunnelForge certificates",
	Long: `Display or export the embedded TunnelForge Root CA certificate.

Examples:
  # Print the embedded CA certificate to stdout:
  forge cert

  # Export the embedded CA certificate to a file:
  forge cert -o ca.crt`,
	RunE: func(cmd *cobra.Command, args []string) error {
		outputFile, _ := cmd.Flags().GetString("out")
		caPEM := certs.GetEmbeddedCACert()

		if len(caPEM) == 0 {
			return fmt.Errorf("no embedded CA certificate found")
		}

		if outputFile != "" {
			if err := os.WriteFile(outputFile, caPEM, 0644); err != nil {
				return fmt.Errorf("failed to write CA certificate to %s: %w", outputFile, err)
			}
			fmt.Printf("CA certificate saved to %s\n", outputFile)
			return nil
		}

		fmt.Print(string(caPEM))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(certCmd)
	certCmd.Flags().StringP("out", "o", "", "File path to save the CA certificate to")
}
