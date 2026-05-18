package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Manage VPS server inventory",
}

var serverListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered servers",
	Run: func(cmd *cobra.Command, args []string) {
		output, _ := cmd.Flags().GetString("output")

		resp, err := apiClient.ListServers()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		if output == "json" {
			b, _ := fmt.Printf("%+v", resp)
			_ = b
			return
		}

		fmt.Printf("%-12s %-14s %-14s %-14s %s\n", "ID", "NAME", "HOSTNAME", "ENV", "STATUS")
		for _, s := range resp.Servers {
			fmt.Printf("%-12s %-14s %-14s %-14s %s\n", s.ID, s.Name, s.Hostname, s.Environment, s.Status)
		}
	},
}

func init() {
	serverListCmd.Flags().String("output", "table", "Output format (table, json)")
	serverCmd.AddCommand(serverListCmd)
}
