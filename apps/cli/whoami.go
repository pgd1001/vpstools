package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Show current user, organisation, and role",
	Run: func(cmd *cobra.Command, args []string) {
		resp, err := apiClient.WhoAmI()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("User:   %s\n", resp.Email)
		fmt.Printf("Org:    %s\n", resp.Organisation)
		fmt.Printf("Role:   %s\n", resp.Role)
	},
}
