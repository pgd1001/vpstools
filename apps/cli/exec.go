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

var execCmd = &cobra.Command{
	Use:   "exec <target> -- <command>",
	Short: "Execute a command on target server(s)",
	Long: `Execute a command on one or more target servers.

Target formats:
  server:<id|name>  Single server (e.g. server:srv_demo, server:web-01)
  tag:<key>=<value> All servers matching tag (e.g. tag:role=web)

Use -- before the command to separate flags from the command.
Use --wait to poll for completion and display results.`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		target := args[0]
		command := args[1:]
		reason, _ := cmd.Flags().GetString("reason")
		idempotencyKey, _ := cmd.Flags().GetString("idempotency-key")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		wait, _ := cmd.Flags().GetBool("wait")
		timeout, _ := cmd.Flags().GetInt("timeout")
		output, _ := cmd.Flags().GetString("output")

		if len(command) == 0 {
			fmt.Fprintln(os.Stderr, "error: no command provided (use -- before command)")
			os.Exit(1)
		}

		cmdStr := strings.Join(command, " ")

		if dryRun {
			fmt.Printf("Target:  %s\n", target)
			fmt.Printf("Command: %s\n", cmdStr)
			if reason != "" {
				fmt.Printf("Reason:  %s\n", reason)
			}
			fmt.Println("\n[dry-run - execution not submitted]")
			return
		}

		resp, err := apiClient.CreateExecutionWithIdempotencyKey(target, cmdStr, reason, idempotencyKey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		if output == "json" {
			b, _ := json.Marshal(map[string]any{
				"execution_id": resp.ExecutionID,
				"status":       resp.Status,
				"target_count": resp.TargetCount,
			})
			fmt.Println(string(b))
			return
		}

		fmt.Printf("Target:       %s\n", target)
		fmt.Printf("Command:      %s\n", cmdStr)
		fmt.Printf("Execution ID: %s\n", resp.ExecutionID)
		fmt.Printf("Status:       %s\n", resp.Status)
		if resp.TargetCount > 0 {
			fmt.Printf("Targets:      %d\n", resp.TargetCount)
		}

		if !wait {
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
			exec, err := apiClient.GetExecution(resp.ExecutionID)
			if err != nil {
				continue
			}
			e := exec.Execution
			switch e.Status {
			case "succeeded":
				fmt.Printf("\nExecution %s\n\n", e.Status)
				for _, t := range exec.Targets {
					fmt.Printf("  [%s] exit=%d\n", t.Status, t.ExitCode)
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
		fmt.Println("\n\nTimeout reached. Check status with: vps exec status " + resp.ExecutionID)
	},
}

var execStatusCmd = &cobra.Command{
	Use:   "status <execution-id>",
	Short: "Check execution status",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		output, _ := cmd.Flags().GetString("output")

		resp, err := apiClient.GetExecution(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		if output == "json" {
			b, _ := json.MarshalIndent(resp, "", "  ")
			fmt.Println(string(b))
			return
		}

		e := resp.Execution
		fmt.Printf("Execution: %s\n", e.ID)
		fmt.Printf("Status:    %s\n", e.Status)
		fmt.Printf("Actor:     %s (%s)\n", e.ActorUserID, e.ActorRole)
		fmt.Printf("Command:   %s\n", e.CommandPreview)
		fmt.Printf("Requested: %s\n", e.RequestedAt)
		if e.StartedAt != "" {
			fmt.Printf("Started:   %s\n", e.StartedAt)
		}
		if e.FinishedAt != "" {
			fmt.Printf("Finished:  %s\n", e.FinishedAt)
		}
		if e.ErrorSummary != "" {
			fmt.Printf("Error:     %s\n", e.ErrorSummary)
		}
		if len(resp.Targets) > 0 {
			fmt.Println("\nTargets:")
			for _, t := range resp.Targets {
				marker := ""
				switch t.Status {
				case "succeeded":
					marker = "[OK]"
				case "failed":
					marker = "[FAIL]"
				case "running":
					marker = "[RUN]"
				case "cancelled":
					marker = "[CANCEL]"
				default:
					marker = "[" + t.Status + "]"
				}
				fmt.Printf("  %s %s exit=%d\n", marker, t.ServerID, t.ExitCode)
				if t.Error != "" {
					fmt.Printf("    error: %s\n", t.Error)
				}
			}
		}
	},
}

var execCancelCmd = &cobra.Command{
	Use:   "cancel <execution-id>",
	Short: "Cancel a pending or queued execution",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		output, _ := cmd.Flags().GetString("output")

		resp, err := apiClient.CancelExecution(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		if output == "json" {
			b, _ := json.Marshal(resp)
			fmt.Println(string(b))
			return
		}
		fmt.Println("Execution cancelled")
	},
}

var execListCmd = &cobra.Command{
	Use:   "list",
	Short: "List recent executions",
	Run: func(cmd *cobra.Command, args []string) {
		output, _ := cmd.Flags().GetString("output")
		status, _ := cmd.Flags().GetString("status")
		limit, _ := cmd.Flags().GetString("limit")
		mine, _ := cmd.Flags().GetBool("mine")
		delegated, _ := cmd.Flags().GetBool("delegated")
		delegatedBy, _ := cmd.Flags().GetString("delegated-by")

		var resp *client.ListExecutionsResponse
		var err error
		if delegated || delegatedBy != "" {
			resp, err = apiClient.ListDelegatedExecutions(status, limit)
		} else if mine {
			resp, err = apiClient.ListMyExecutions(status, limit)
		} else {
			resp, err = apiClient.ListExecutions(status, limit)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		if output == "json" {
			b, _ := json.MarshalIndent(resp, "", "  ")
			fmt.Println(string(b))
			return
		}

		if len(resp.Executions) == 0 {
			fmt.Println("No executions found.")
			return
		}

		fmt.Printf("%-14s %-12s %-10s %s\n", "ID", "STATUS", "TARGETS", "COMMAND")
		for _, e := range resp.Executions {
			summary := fmt.Sprintf("%d/%d/%d", e.SucceededCount, e.FailedCount, e.TargetCount)
			fmt.Printf("%-14s %-12s %-10s %s\n", e.ID, e.Status, summary, truncate(e.CommandPreview, 50))
		}
	},
}

func init() {
	execCmd.Flags().String("reason", "", "Reason for execution")
	execCmd.Flags().String("idempotency-key", "", "Stable retry key for safely resubmitting the same execution")
	execCmd.Flags().Bool("dry-run", false, "Preview without executing")
	execCmd.Flags().Bool("wait", false, "Wait for execution to complete")
	execCmd.Flags().Int("timeout", 300, "Timeout in seconds (with --wait)")
	execCmd.Flags().String("output", "table", "Output format (table, json)")

	execStatusCmd.Flags().String("output", "table", "Output format (table, json)")

	execCancelCmd.Flags().String("output", "table", "Output format (table, json)")

	execListCmd.Flags().String("status", "", "Filter by status")
	execListCmd.Flags().String("limit", "20", "Max results")
	execListCmd.Flags().String("output", "table", "Output format (table, json)")
	execListCmd.Flags().Bool("mine", false, "Show only my executions")
	execListCmd.Flags().Bool("delegated", false, "Show work delegated to others")
	execListCmd.Flags().String("delegated-by", "", "Show work delegated by a specific user")

	execCmd.AddCommand(execStatusCmd)
	execCmd.AddCommand(execCancelCmd)
	execCmd.AddCommand(execListCmd)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
