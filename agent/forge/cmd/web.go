/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

// webCmd represents the web command
var webCmd = &cobra.Command{
	Use:   "web",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	RunE: func(cmd *cobra.Command, args []string) error{
		var port = "4040" 
		if len(args) > 0 {
			port = args[0]
		}else{
			fmt.Println("Using default port "+port)
		}
		const staticDir = "../static"
		fileserver := http.FileServer(http.Dir(staticDir))

		http.Handle("/",fileserver)
		http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				 http.Error(w, "method is not supported", http.StatusBadGateway)
  				 return
			}
			fmt.Fprint(w, "server is running and healthy")
		})

		fmt.Println("Web interface running on port "+port)
		return http.ListenAndServe(":"+port, nil)
	},
}

func init() {
	rootCmd.AddCommand(webCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// webCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// webCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
