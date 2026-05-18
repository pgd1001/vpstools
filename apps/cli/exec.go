package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:   "exec <server|selector> -- <command>",
	Short: "Execute a command on target server(s)",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		target := args[0]
		command := args[1:]
		reason, _ := cmd.Flags().GetString("reason")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		if len(command) == 0 {
			fmt.Fprintln(os.Stderr, "error: no command provided (use -- before command)")
			os.Exit(1)
		}

		cmdStr := strings.Join(command, " ")
		fmt.Printf("Target:  %s\n", target)
		fmt.Printf("Command: %s\n", cmdStr)
		if reason != "" {
			fmt.Printf("Reason:  %s\n", reason)
		}
		if dryRun {
			fmt.Println("[dry-run — execution not submitted]")
			return
		}

		resp, err := apiClient.CreateExecution(target, cmdStr, reason)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("\nExecution ID: %s\n", resp.ExecutionID)
		fmt.Printf("Status:       %s\n", resp.Status)
		fmt.Println("\nRun the runner to execute: make runner && ./bin/runner")
	},
}

func init() {
	execCmd.Flags().String("reason", "", "Reason for execution")
	execCmd.Flags().Bool("dry-run", false, "Preview without executing")
}
