package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/pgd1001/svrtools/packages/runbooks"
	"github.com/pgd1001/svrtools/packages/sdk-go/client"
	"github.com/spf13/cobra"
)

var automationCmd = &cobra.Command{
	Use:   "automation",
	Short: "Pause, resume, and inspect scheduled automation",
}

var automationStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the organisation automation state",
	Run: func(cmd *cobra.Command, args []string) {
		output, _ := cmd.Flags().GetString("output")
		status, err := apiClient.AutomationStatus()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if output == "json" {
			b, _ := json.MarshalIndent(status, "", "  ")
			fmt.Println(string(b))
			return
		}
		if status.Paused {
			fmt.Printf("Automation paused: %s\n", status.Reason)
			return
		}
		fmt.Println("Automation running")
	},
}

var automationListCmd = &cobra.Command{
	Use:   "list",
	Short: "List scheduled automation",
	Run: func(cmd *cobra.Command, args []string) {
		output, _ := cmd.Flags().GetString("output")
		response, err := apiClient.ListSchedules()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if output == "json" {
			b, _ := json.MarshalIndent(response, "", "  ")
			fmt.Println(string(b))
			return
		}
		for _, schedule := range response.Schedules {
			status := "enabled"
			if !schedule.Enabled {
				status = "disabled"
			}
			fmt.Printf("%-24s %-20s %-22s every %ds, %s\n", schedule.ID, schedule.RunbookName, schedule.Target, schedule.IntervalSeconds, status)
		}
	},
}

var automationCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an interval schedule",
	Run: func(cmd *cobra.Command, args []string) {
		name, _ := cmd.Flags().GetString("name")
		runbookName, _ := cmd.Flags().GetString("runbook")
		target, _ := cmd.Flags().GetString("target")
		reason, _ := cmd.Flags().GetString("reason")
		paramsRaw, _ := cmd.Flags().GetString("params")
		interval, _ := cmd.Flags().GetInt("interval")
		nextRunAt, _ := cmd.Flags().GetString("next-run-at")
		params, err := runbooks.ParseParameterValues(paramsRaw)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: invalid params: %v\n", err)
			os.Exit(1)
		}
		response, err := apiClient.CreateSchedule(client.CreateScheduleRequest{
			Name: name, RunbookName: runbookName, Target: target, Reason: reason,
			Params: params, IntervalSeconds: interval, NextRunAt: nextRunAt,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			b, _ := json.MarshalIndent(response, "", "  ")
			fmt.Println(string(b))
			return
		}
		fmt.Printf("Schedule created: %s\n", response["schedule_id"])
	},
}

var automationDisableCmd = &cobra.Command{
	Use:   "disable <schedule-id>",
	Short: "Disable a scheduled automation",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		confirm, _ := cmd.Flags().GetBool("confirm")
		if !confirm {
			fmt.Fprintln(os.Stderr, "refusing to disable a schedule without --confirm")
			os.Exit(1)
		}
		response, err := apiClient.DisableSchedule(strings.TrimSpace(args[0]))
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			b, _ := json.MarshalIndent(response, "", "  ")
			fmt.Println(string(b))
			return
		}
		fmt.Printf("Schedule disabled: %s\n", args[0])
	},
}

var automationPauseCmd = &cobra.Command{
	Use:   "pause",
	Short: "Stop new scheduled automation runs",
	Run: func(cmd *cobra.Command, args []string) {
		reason, _ := cmd.Flags().GetString("reason")
		output, _ := cmd.Flags().GetString("output")
		status, err := apiClient.PauseAutomation(reason)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if output == "json" {
			b, _ := json.Marshal(status)
			fmt.Println(string(b))
			return
		}
		fmt.Println("Automation paused. Existing queued work is not cancelled.")
	},
}

var automationResumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Allow scheduled automation runs again",
	Run: func(cmd *cobra.Command, args []string) {
		output, _ := cmd.Flags().GetString("output")
		status, err := apiClient.ResumeAutomation()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if output == "json" {
			b, _ := json.Marshal(status)
			fmt.Println(string(b))
			return
		}
		fmt.Println("Automation resumed")
	},
}

func init() {
	for _, command := range []*cobra.Command{automationStatusCmd, automationListCmd, automationCreateCmd, automationPauseCmd, automationResumeCmd, automationDisableCmd} {
		command.Flags().String("output", "table", "Output format (table, json)")
	}
	automationPauseCmd.Flags().String("reason", "", "Reason for the emergency pause")
	automationCreateCmd.Flags().String("name", "", "Unique schedule name")
	automationCreateCmd.Flags().String("runbook", "", "Published runbook name")
	automationCreateCmd.Flags().String("target", "", "Execution target")
	automationCreateCmd.Flags().String("reason", "", "Reason for scheduled work")
	automationCreateCmd.Flags().String("params", "", "Parameters as name=value,name2=value2")
	automationCreateCmd.Flags().Int("interval", 3600, "Interval in seconds")
	automationCreateCmd.Flags().String("next-run-at", "", "Optional RFC3339 next run time")
	automationDisableCmd.Flags().Bool("confirm", false, "Confirm disabling the schedule")
	automationCmd.AddCommand(automationStatusCmd, automationListCmd, automationCreateCmd, automationPauseCmd, automationResumeCmd, automationDisableCmd)
}
