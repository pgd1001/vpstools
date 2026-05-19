package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/pgd1001/svrtools/packages/sdk-go/client"
	"github.com/spf13/cobra"
)

var runnerCmd = &cobra.Command{
	Use:   "runner",
	Short: "Manage execution runners",
}

var runnerListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered runners",
	Run: func(cmd *cobra.Command, args []string) {
		output, _ := cmd.Flags().GetString("output")

		resp, err := apiClient.ListRunners()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		if output == "json" {
			b, _ := json.MarshalIndent(resp, "", "  ")
			fmt.Println(string(b))
			return
		}

		if len(resp.Runners) == 0 {
			fmt.Println("No runners registered.")
			return
		}

		fmt.Printf("%-14s %-18s %-8s %-16s %-14s %s\n", "ID", "NAME", "STATUS", "HOSTNAME", "TYPE", "LAST SEEN")
		for _, r := range resp.Runners {
			fmt.Printf("%-14s %-18s %-8s %-16s %-14s %s\n",
				r.ID, r.Name, r.Status, r.Hostname, r.RunnerType, r.LastSeenAt)
		}
	},
}

var runnerRegisterCmd = &cobra.Command{
	Use:   "register <name>",
	Short: "Register a new runner",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		version, _ := cmd.Flags().GetString("version")
		hostname, _ := cmd.Flags().GetString("hostname")
		platform, _ := cmd.Flags().GetString("platform")
		ipAddr, _ := cmd.Flags().GetString("ip-address")
		runnerType, _ := cmd.Flags().GetString("type")
		output, _ := cmd.Flags().GetString("output")

		req := client.RegisterRunnerRequest{
			Name:       args[0],
			Version:    version,
			Hostname:   hostname,
			Platform:   platform,
			IPAddress:  ipAddr,
			RunnerType: runnerType,
		}

		resp, err := apiClient.RegisterRunner(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		if output == "json" {
			b, _ := json.Marshal(resp)
			fmt.Println(string(b))
			return
		}
		fmt.Printf("Runner registered: %s (%s)\n", resp.RunnerID, resp.Status)
	},
}

var runnerTokenCmd = &cobra.Command{
	Use:   "registration-token",
	Short: "Create a runner registration token",
	Run: func(cmd *cobra.Command, args []string) {
		output, _ := cmd.Flags().GetString("output")

		resp, err := apiClient.CreateRegistrationToken()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		if output == "json" {
			b, _ := json.Marshal(resp)
			fmt.Println(string(b))
			return
		}
		fmt.Printf("Registration token: %s (expires in %ss)\n", resp.Token, resp.ExpiresIn)
	},
}

func init() {
	runnerListCmd.Flags().String("output", "table", "Output format (table, json)")

	runnerRegisterCmd.Flags().String("version", "", "Runner version")
	runnerRegisterCmd.Flags().String("hostname", "", "Runner hostname")
	runnerRegisterCmd.Flags().String("platform", "", "Runner platform (linux, darwin, windows)")
	runnerRegisterCmd.Flags().String("ip-address", "", "Runner IP address")
	runnerRegisterCmd.Flags().String("type", "customer_managed", "Runner type")
	runnerRegisterCmd.Flags().String("output", "table", "Output format (table, json)")

	runnerTokenCmd.Flags().String("output", "table", "Output format (table, json)")

	runnerCmd.AddCommand(runnerListCmd)
	runnerCmd.AddCommand(runnerRegisterCmd)
	runnerCmd.AddCommand(runnerTokenCmd)
}
