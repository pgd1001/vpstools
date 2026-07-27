package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

type doctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
	Error  string `json:"error,omitempty"`
}

var doctorJSON bool

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check API reachability, readiness, and authenticated identity",
	Run: func(cmd *cobra.Command, args []string) {
		checks := make([]doctorCheck, 0, 3)

		health, err := apiClient.Health()
		if err != nil {
			checks = append(checks, doctorCheck{Name: "api health", Status: "fail", Error: err.Error()})
		} else {
			checks = append(checks, doctorCheck{Name: "api health", Status: "pass", Detail: fmt.Sprintf("%s, version %s, tier %s", health.Status, valueOrUnknown(health.Version), valueOrUnknown(health.DeploymentTier))})
		}

		ready, err := apiClient.Ready()
		if err != nil {
			checks = append(checks, doctorCheck{Name: "service readiness", Status: "fail", Error: err.Error()})
		} else {
			checks = append(checks, doctorCheck{Name: "service readiness", Status: "pass", Detail: fmt.Sprintf("database %s, artefacts %s", ready.Database, ready.Artifacts)})
		}

		identity, err := apiClient.WhoAmI()
		if err != nil {
			checks = append(checks, doctorCheck{Name: "authenticated identity", Status: "fail", Error: err.Error()})
		} else {
			checks = append(checks, doctorCheck{Name: "authenticated identity", Status: "pass", Detail: fmt.Sprintf("%s, role %s, organisation %s", identity.Email, identity.Role, identity.Organisation)})
		}

		failed := false
		for _, check := range checks {
			if check.Status != "pass" {
				failed = true
			}
		}
		if doctorJSON {
			if err := json.NewEncoder(os.Stdout).Encode(map[string]any{"status": map[bool]string{true: "fail", false: "pass"}[failed], "checks": checks}); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
		} else {
			fmt.Println("VPS Tools production preflight")
			for _, check := range checks {
				marker := "PASS"
				if check.Status != "pass" {
					marker = "FAIL"
				}
				if check.Error != "" {
					fmt.Printf("[%s] %-24s %s\n", marker, check.Name, check.Error)
				} else {
					fmt.Printf("[%s] %-24s %s\n", marker, check.Name, check.Detail)
				}
			}
		}
		if failed {
			os.Exit(1)
		}
	},
}

func valueOrUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorJSON, "json", false, "Print machine-readable JSON output")
}
