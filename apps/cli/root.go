package main

import (
	"fmt"
	"os"

	"github.com/pgd1001/svrtools/packages/sdk-go/client"
	"github.com/spf13/cobra"
)

var (
	apiClient *client.Client
	apiURL    string
)

var rootCmd = &cobra.Command{
	Use:   "vps",
	Short: "Secure CLI control plane for VPS fleets",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if apiURL == "" {
			apiURL = os.Getenv("VPS_API_URL")
		}
		if apiURL == "" {
			apiURL = "http://localhost:8080"
		}
		apiClient = client.New(apiURL)
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&apiURL, "api-url", "", "Control plane API URL (or set VPS_API_URL env)")
	rootCmd.AddCommand(whoamiCmd)
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(execCmd)
	rootCmd.AddCommand(auditCmd)
}
