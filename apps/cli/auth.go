package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{Use: "auth", Short: "Manage API authentication"}

var authTokenCreateCmd = &cobra.Command{
	Use:   "create-token",
	Short: "Create a short-lived API bearer token",
	Run: func(cmd *cobra.Command, args []string) {
		name, _ := cmd.Flags().GetString("name")
		expiresIn, _ := cmd.Flags().GetInt("expires-in")
		userID, _ := cmd.Flags().GetString("user-id")
		resp, err := apiClient.CreateAPIToken(name, userID, expiresIn)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			body, _ := json.MarshalIndent(resp, "", "  ")
			fmt.Println(string(body))
			return
		}
		fmt.Printf("Token: %s\n", resp.Token)
		fmt.Printf("Expires: %s\n", resp.ExpiresAt)
		fmt.Println("Store this token securely. It will not be shown again.")
	},
}

func init() {
	authTokenCreateCmd.Flags().String("name", "cli-token", "token label")
	authTokenCreateCmd.Flags().String("user-id", "", "user ID to bind the token to")
	authTokenCreateCmd.Flags().Int("expires-in", 30*24*60*60, "token lifetime in seconds")
	authTokenCreateCmd.Flags().String("output", "table", "output format: table or json")
	authCmd.AddCommand(authTokenCreateCmd)
	rootCmd.AddCommand(authCmd)
}
