/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"
	"time"
	"tunnelforge/agent/forge/client"
	"tunnelforge/internal/proto"

	"github.com/spf13/cobra"
)

// registerCmd represents the register command
var registerCmd = &cobra.Command{
	Use:   "register",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	RunE: func(cmd *cobra.Command, args []string) error{
		key, _ := cmd.Flags().GetString("enrollment-key")
		serverAddr, _ := cmd.Flags().GetString("server")

		rc := client.NewRESTClient(client.GetHTTPClient(true, 10*time.Second), serverAddr)
		req := proto.RegisterRequest{EnrollmentKey: key}

		var res proto.RegisterResponse
		err := rc.Post(context.Background(),"/register", &req, &res)
		if err != nil {
			return err
		}
		
		fmt.Printf("Token: %s\nAgent ID: %s\n", res.Token, res.AgentID)
		

		return nil
	},
}

func init() {
	rootCmd.AddCommand(registerCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// registerCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// registerCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	registerCmd.Flags().StringP("enrollment-key", "e", "tf_enroll_xyz123", "Enrollmet key for agent registration")
	registerCmd.Flags().StringP("server", "s","http://localhost:8000/forge/internal/auth", "Server address")
}
