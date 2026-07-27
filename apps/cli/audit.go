package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Search and view audit events",
}

var auditSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search audit events",
	Run: func(cmd *cobra.Command, args []string) {
		actor, _ := cmd.Flags().GetString("actor")
		limit, _ := cmd.Flags().GetString("limit")
		if limit == "" {
			limit = "20"
		}

		resp, err := apiClient.ListAudit(limit)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("%-38s %-20s %-14s %-14s %s\n", "EVENT_ID", "ACTION", "TARGET_TYPE", "RESULT", "TIME")
		for _, e := range resp.Events {
			fmt.Printf("%-38s %-20s %-14s %-14s %s\n", e.ID, e.Action, e.TargetType, e.Result, e.CreatedAt)
		}
		if actor != "" {
			fmt.Printf("\n(filtered by actor: %s)\n", actor)
		}
	},
}

var auditVerifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify the organisation audit hash chain",
	Run: func(cmd *cobra.Command, args []string) {
		resp, err := apiClient.VerifyAudit()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if !resp.Valid {
			fmt.Fprintf(os.Stderr, "audit chain invalid after %d events: %s\n", resp.CheckedEvents, resp.Error)
			os.Exit(1)
		}
		fmt.Printf("Audit chain valid (%d events)\n", resp.CheckedEvents)
	},
}

func init() {
	auditSearchCmd.Flags().String("actor", "", "Filter by actor")
	auditSearchCmd.Flags().String("limit", "20", "Max results")
	auditCmd.AddCommand(auditSearchCmd)
	auditCmd.AddCommand(auditVerifyCmd)
}
