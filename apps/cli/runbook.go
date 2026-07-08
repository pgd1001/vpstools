package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pgd1001/svrtools/packages/sdk-go/client"
	"github.com/spf13/cobra"
)

var runbookCmd = &cobra.Command{
	Use:   "runbook",
	Short: "Manage runbooks",
}

var runbookListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available runbooks",
	Run: func(cmd *cobra.Command, args []string) {
		output, _ := cmd.Flags().GetString("output")

		resp, err := apiClient.ListRunbooks()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		if output == "json" {
			b, _ := json.MarshalIndent(resp, "", "  ")
			fmt.Println(string(b))
			return
		}

		if len(resp.Runbooks) == 0 {
			fmt.Println("No runbooks available.")
			return
		}

		fmt.Printf("%-14s %-20s %-10s %-8s %s\n", "ID", "NAME", "STATUS", "RISK", "COMMAND")
		for _, rb := range resp.Runbooks {
			perm := ""
			if !rb.Permitted {
				perm = " [restricted]"
			}
			fmt.Printf("%-14s %-20s %-10s %-8s %s%s\n", rb.ID, rb.Name, rb.Status, rb.Risk, rb.Command, perm)
		}
	},
}

var runbookInspectCmd = &cobra.Command{
	Use:   "inspect <runbook>",
	Short: "Show runbook details",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		output, _ := cmd.Flags().GetString("output")

		resp, err := apiClient.GetRunbook(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		if output == "json" {
			b, _ := json.MarshalIndent(resp, "", "  ")
			fmt.Println(string(b))
			return
		}

		rb := resp.Runbook
		fmt.Printf("ID:            %s\n", rb.ID)
		fmt.Printf("Name:          %s\n", rb.Name)
		fmt.Printf("Title:         %s\n", rb.Title)
		fmt.Printf("Description:   %s\n", rb.Description)
		fmt.Printf("Status:        %s\n", rb.Status)
		fmt.Printf("Version:       %d\n", rb.Version)
		fmt.Printf("Risk:          %s\n", rb.Risk)
		fmt.Printf("Command:       %s\n", rb.Command)
		fmt.Printf("Allowed roles: %s\n", rb.AllowedRoles)
		fmt.Printf("Created:       %s\n", rb.CreatedAt)
	},
}

var runbookCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new runbook",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		yamlFile, _ := cmd.Flags().GetString("file")
		title, _ := cmd.Flags().GetString("title")
		cmdStr, _ := cmd.Flags().GetString("command")
		risk, _ := cmd.Flags().GetString("risk")
		env, _ := cmd.Flags().GetString("environment")
		desc, _ := cmd.Flags().GetString("description")
		output, _ := cmd.Flags().GetString("output")

		req := client.CreateRunbookRequest{
			Name:        args[0],
			Title:       title,
			Description: desc,
			Risk:        risk,
			Command:     cmdStr,
			Timeout:     300,
			Environment: env,
		}

		if yamlFile != "" {
			b, err := os.ReadFile(yamlFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error reading file: %v\n", err)
				os.Exit(1)
			}
			req.YAML = string(b)
		}

		resp, err := apiClient.CreateRunbook(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		if output == "json" {
			b, _ := json.Marshal(resp)
			fmt.Println(string(b))
			return
		}
		fmt.Printf("Runbook created: %s (%s) [%s]\n", resp.Name, resp.RunbookID, resp.Status)
	},
}

var runbookPublishCmd = &cobra.Command{
	Use:   "publish <runbook>",
	Short: "Publish a runbook",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		resp, err := apiClient.PublishRunbook(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Runbook %s\n", resp["status"])
	},
}

var runbookRunCmd = &cobra.Command{
	Use:   "run <runbook>",
	Short: "Execute a runbook",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		target, _ := cmd.Flags().GetString("target")
		reason, _ := cmd.Flags().GetString("reason")
		paramsFlag, _ := cmd.Flags().GetString("params")
		output, _ := cmd.Flags().GetString("output")
		wait, _ := cmd.Flags().GetBool("wait")
		timeout, _ := cmd.Flags().GetInt("timeout")

		if target == "" {
			fmt.Fprintln(os.Stderr, "error: --target is required (e.g. server:demo, server:web-01)")
			os.Exit(1)
		}

		params := map[string]string{}
		if paramsFlag != "" {
			for _, p := range strings.Split(paramsFlag, ",") {
				kv := strings.SplitN(p, "=", 2)
				if len(kv) == 2 {
					params[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
				}
			}
		}

		resp, err := apiClient.RunRunbook(args[0], target, reason, params)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		if output == "json" {
			b, _ := json.Marshal(resp)
			fmt.Println(string(b))
			return
		}

		if resp["status"] == "awaiting_approval" {
			fmt.Printf("Approval required: %s\n", resp["message"])
			if aid, ok := resp["approval_id"]; ok {
				fmt.Printf("Approval ID: %s\n", aid)
			}
			return
		}

		execID := ""
		if eid, ok := resp["execution_id"]; ok {
			execID = eid.(string)
		}
		fmt.Printf("Execution ID: %s\n", execID)
		fmt.Printf("Status:       %s\n", resp["status"])
		if tc, ok := resp["target_count"]; ok {
			fmt.Printf("Targets:      %v\n", tc)
		}

		if !wait || execID == "" {
			fmt.Println("\nRun the runner to execute, or use --wait to poll for results.")
			return
		}

		fmt.Println("\nWaiting for completion...")
		if timeout == 0 {
			timeout = 300
		}

		pollInterval := 1 * time.Second
		deadline := time.Now().Add(time.Duration(timeout) * time.Second)

		for time.Now().Before(deadline) {
			time.Sleep(pollInterval)
			exec, err := apiClient.GetExecution(execID)
			if err != nil {
				continue
			}
			e := exec.Execution
			switch e.Status {
			case "succeeded":
				fmt.Printf("\nExecution %s\n\n", e.Status)
				for _, t := range exec.Targets {
					fmt.Printf("  [%s] exit=%d\n", t.Status, t.ExitCode)
					if t.Stdout != "" {
						fmt.Printf("  stdout: %s\n", strings.TrimSpace(t.Stdout))
					}
					if t.Stderr != "" {
						fmt.Printf("  stderr: %s\n", strings.TrimSpace(t.Stderr))
					}
				}
				return
			case "failed":
				fmt.Printf("\nExecution %s\n\n", e.Status)
				for _, t := range exec.Targets {
					fmt.Printf("  [%s] exit=%d\n", t.Status, t.ExitCode)
					if t.Error != "" {
						fmt.Printf("    error: %s\n", t.Error)
					}
				}
				os.Exit(1)
			case "cancelled":
				fmt.Println("\nExecution cancelled")
				os.Exit(1)
			default:
				since := time.Since(time.Now().Add(-pollInterval)).Round(time.Second)
				fmt.Printf("\rStatus: %s (%s elapsed)...", e.Status, since)
			}
			pollInterval = minDuration(pollInterval*2, 5*time.Second)
		}
		fmt.Println("\n\nTimeout reached. Check status with: vps exec status " + execID)
	},
}

func init() {
	runbookListCmd.Flags().String("output", "table", "Output format (table, json)")

	runbookInspectCmd.Flags().String("output", "table", "Output format (table, json)")

	runbookCreateCmd.Flags().String("file", "", "YAML file with runbook definition")
	runbookCreateCmd.Flags().String("title", "", "Runbook title")
	runbookCreateCmd.Flags().String("command", "", "Command to execute")
	runbookCreateCmd.Flags().String("risk", "medium", "Risk level (low, medium, high, critical)")
	runbookCreateCmd.Flags().String("environment", "development", "Allowed environment")
	runbookCreateCmd.Flags().String("description", "", "Description")
	runbookCreateCmd.Flags().String("output", "table", "Output format (table, json)")

	runbookPublishCmd.Flags().String("output", "table", "Output format (table, json)")

	runbookRunCmd.Flags().String("target", "", "Target server (server:id or server:name)")
	runbookRunCmd.Flags().String("reason", "", "Reason for execution")
	runbookRunCmd.Flags().String("params", "", "Parameters as key=value,key=value")
	runbookRunCmd.Flags().String("output", "table", "Output format (table, json)")
	runbookRunCmd.Flags().Bool("wait", false, "Wait for execution to complete")
	runbookRunCmd.Flags().Int("timeout", 300, "Timeout in seconds (with --wait)")

	runbookCmd.AddCommand(runbookListCmd)
	runbookCmd.AddCommand(runbookInspectCmd)
	runbookCmd.AddCommand(runbookCreateCmd)
	runbookCmd.AddCommand(runbookPublishCmd)
	runbookCmd.AddCommand(runbookRunCmd)
}
