/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"tunnelforge/internal/proto"

	"github.com/hashicorp/yamux"
	"github.com/spf13/cobra"
)

// dialCmd represents the dial command
var dialCmd = &cobra.Command{
	Use:   "dial",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, err := net.Dial("tcp", "localhost:7000")
		if err != nil {
			return err
		}
		defer conn.Close()
		fmt.Println("Connected to server")

		if err := handShake(conn); err != nil {
			return err
		}

		var response proto.HandshakeResponse
		decoder := json.NewDecoder(conn)

		if err := decoder.Decode(&response); err != nil {
		    fmt.Println("Error decoding handshake response:", err)
		    return err
		}

		if !response.OK {
			return fmt.Errorf("server rejected handshake: %s", response.Message)
		}

		fmt.Println("Server:", response.Message)
		session, err := yamux.Client(conn, nil)
		if err != nil {
			return err
		}

		defer session.Close()

		fmt.Println("Yamux session established")
		for {
			stream, err := session.Accept()
			if err != nil {
				return err
			}	

			go handleStream(stream)
		}
	},
}

func init() {
	rootCmd.AddCommand(dialCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// dialCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// dialCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

func handShake(conn net.Conn) error {
	req := proto.HandshakeRequest{
		Type:      "handshake",
		Token:     "abc",
		Subdomain: "test_app",
	}

	encoder := json.NewEncoder(conn)

	err := encoder.Encode(req)
	if err != nil {
	    return err
	}

	return nil
}

func handleStream(stream net.Conn) {
	defer stream.Close()

	localConn, err := net.Dial("tcp", "localhost:3000")
	if err != nil {
		fmt.Println("Error dialling local connection: " + err.Error())
		return
	}

	defer localConn.Close()
	fmt.Println("Connected to local connection")

	go func() {
		_, err := io.Copy(localConn, stream)
		if err != nil {
			fmt.Println("Error copying stream -> local:", err)
		}
	}()

	_, err = io.Copy(stream, localConn)
	if err != nil {
		fmt.Println("Error copying local -> stream:", err)
	}
}