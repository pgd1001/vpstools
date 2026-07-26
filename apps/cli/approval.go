package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var approvalCmd = &cobra.Command{
	Use:   "approvals",
	Short: "Manage approval requests",
}

var approvalListCmd = &cobra.Command{
	Use:   "list",
	Short: "List pending approval requests",
	Run: func(cmd *cobra.Command, args []string) {
		output, _ := cmd.Flags().GetString("output")
		status, _ := cmd.Flags().GetString("status")

		resp, err := apiClient.ListApprovals(status)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		if output == "json" {
			b, _ := json.MarshalIndent(resp, "", "  ")
			fmt.Println(string(b))
			return
		}

		if len(resp.Approvals) == 0 {
			fmt.Println("No pending approvals.")
			return
		}

		fmt.Printf("%-14s %-14s %-10s %-10s %s\n", "ID", "REQUESTER", "TYPE", "STATUS", "REASON")
		for _, a := range resp.Approvals {
			fmt.Printf("%-14s %-14s %-10s %-10s %s\n", a.ID, a.RequesterName, a.ActionType, a.Status, a.Reason)
		}
	},
}

var approvalApproveCmd = &cobra.Command{
	Use:   "approve <approval-id>",
	Short: "Approve an approval request",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		output, _ := cmd.Flags().GetString("output")
		note, _ := cmd.Flags().GetString("note")

		resp, err := apiClient.ApproveApproval(args[0], note)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		if output == "json" {
			b, _ := json.Marshal(resp)
			fmt.Println(string(b))
			return
		}
		fmt.Println("Approval granted")
	},
}

var approvalDenyCmd = &cobra.Command{
	Use:   "deny <approval-id>",
	Short: "Deny an approval request",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		output, _ := cmd.Flags().GetString("output")
		note, _ := cmd.Flags().GetString("note")

		if note == "" {
			fmt.Fprintln(os.Stderr, "error: --note is required when denying an approval")
			os.Exit(1)
		}
		resp, err := apiClient.DenyApproval(args[0], note)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		if output == "json" {
			b, _ := json.Marshal(resp)
			fmt.Println(string(b))
			return
		}
		fmt.Println("Approval denied")
	},
}

func init() {
	approvalListCmd.Flags().String("status", "", "Filter by status (pending, approved, denied)")
	approvalListCmd.Flags().String("output", "table", "Output format (table, json)")

	approvalApproveCmd.Flags().String("output", "table", "Output format (table, json)")
	approvalApproveCmd.Flags().String("note", "", "Optional approval note")

	approvalDenyCmd.Flags().String("output", "table", "Output format (table, json)")
	approvalDenyCmd.Flags().String("note", "", "Required reason for denial")

	approvalCmd.AddCommand(approvalListCmd)
	approvalCmd.AddCommand(approvalApproveCmd)
	approvalCmd.AddCommand(approvalDenyCmd)
}
