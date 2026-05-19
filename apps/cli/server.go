package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/pgd1001/svrtools/packages/sdk-go/client"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Manage VPS server inventory",
}

var serverAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add a server to the inventory",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		hostname, _ := cmd.Flags().GetString("hostname")
		publicIP, _ := cmd.Flags().GetString("public-ip")
		privateIP, _ := cmd.Flags().GetString("private-ip")
		sshPort, _ := cmd.Flags().GetInt("ssh-port")
		sshUser, _ := cmd.Flags().GetString("ssh-user")
		environment, _ := cmd.Flags().GetString("environment")
		provider, _ := cmd.Flags().GetString("provider")
		tagsJSON, _ := cmd.Flags().GetString("tags")
		output, _ := cmd.Flags().GetString("output")

		req := client.AddServerRequest{
			Name:        args[0],
			Hostname:    hostname,
			PublicIP:    publicIP,
			PrivateIP:   privateIP,
			SSHPort:     sshPort,
			SSHUsername: sshUser,
			Environment: environment,
			Provider:    provider,
		}

		if tagsJSON != "" {
			json.Unmarshal([]byte(tagsJSON), &req.Tags)
		}

		resp, err := apiClient.AddServer(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		if output == "json" {
			b, _ := json.Marshal(resp)
			fmt.Println(string(b))
			return
		}
		fmt.Printf("Server added: %s (%s)\n", resp.ServerID, resp.Status)
	},
}

var serverListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered servers",
	Run: func(cmd *cobra.Command, args []string) {
		output, _ := cmd.Flags().GetString("output")
		environment, _ := cmd.Flags().GetString("environment")
		tagKey, _ := cmd.Flags().GetString("tag-key")
		tagValue, _ := cmd.Flags().GetString("tag-value")

		resp, err := apiClient.ListServers(environment, tagKey, tagValue)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		if output == "json" {
			b, _ := json.MarshalIndent(resp, "", "  ")
			fmt.Println(string(b))
			return
		}

		if len(resp.Servers) == 0 {
			fmt.Println("No servers found.")
			return
		}

		fmt.Printf("%-14s %-16s %-18s %-14s %-10s %s\n", "ID", "NAME", "HOSTNAME", "ENV", "STATUS", "TAGS")
		for _, s := range resp.Servers {
			tags := ""
			for i, t := range s.Tags {
				if i > 0 {
					tags += ","
				}
				tags += t.Key + "=" + t.Value
			}
			fmt.Printf("%-14s %-16s %-18s %-14s %-10s %s\n", s.ID, s.Name, s.Hostname, s.Environment, s.Status, tags)
		}
	},
}

var serverInspectCmd = &cobra.Command{
	Use:   "inspect <server>",
	Short: "Show server details",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		output, _ := cmd.Flags().GetString("output")

		resp, err := apiClient.GetServer(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		if output == "json" {
			b, _ := json.MarshalIndent(resp, "", "  ")
			fmt.Println(string(b))
			return
		}

		s := resp.Server
		fmt.Printf("ID:           %s\n", s.ID)
		fmt.Printf("Name:         %s\n", s.Name)
		fmt.Printf("Hostname:     %s\n", s.Hostname)
		fmt.Printf("Public IP:    %s\n", s.PublicIP)
		fmt.Printf("Private IP:   %s\n", s.PrivateIP)
		fmt.Printf("SSH:          %s@:%d\n", s.SSHUsername, s.SSHPort)
		fmt.Printf("Environment:  %s\n", s.Environment)
		fmt.Printf("Provider:     %s\n", s.Provider)
		fmt.Printf("OS:           %s %s (%s) %s\n", s.OSName, s.OSVersion, s.Kernel, s.Arch)
		fmt.Printf("Status:       %s\n", s.Status)
		fmt.Printf("Last check:   %s\n", s.LastCheckAt)
		fmt.Printf("Last seen:    %s\n", s.LastSeenAt)
		fmt.Printf("Created:      %s\n", s.CreatedAt)
		fmt.Print("Tags:        ")
		for i, t := range s.Tags {
			if i > 0 {
				fmt.Print(", ")
			}
			fmt.Printf("%s=%s", t.Key, t.Value)
		}
		fmt.Println()
	},
}

var serverCheckCmd = &cobra.Command{
	Use:   "check <server>",
	Short: "Check server health and connectivity",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		output, _ := cmd.Flags().GetString("output")

		resp, err := apiClient.CheckServer(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		if output == "json" {
			b, _ := json.MarshalIndent(resp, "", "  ")
			fmt.Println(string(b))
			return
		}

		s := resp.Server
		fmt.Printf("Server:    %s\n", s["server_id"])
		fmt.Printf("Status:    %s\n", s["status"])
		if h, ok := s["hostname"]; ok {
			fmt.Printf("Hostname:  %s\n", h)
		}
		if os, ok := s["os_name"]; ok {
			fmt.Printf("OS:        %s %s\n", os, s["os_version"])
		}
		if k, ok := s["kernel_version"]; ok {
			fmt.Printf("Kernel:    %s\n", k)
		}
		if a, ok := s["architecture"]; ok {
			fmt.Printf("Arch:      %s\n", a)
		}
		if u, ok := s["uptime"]; ok {
			fmt.Printf("Uptime:    %s\n", u)
		}
		fmt.Printf("Checked:   %s\n", s["checked_at"])
	},
}

func init() {
	serverAddCmd.Flags().String("hostname", "", "Server hostname or address")
	serverAddCmd.Flags().String("public-ip", "", "Public IP address")
	serverAddCmd.Flags().String("private-ip", "", "Private IP address")
	serverAddCmd.Flags().Int("ssh-port", 22, "SSH port")
	serverAddCmd.Flags().String("ssh-user", "root", "SSH username")
	serverAddCmd.Flags().String("environment", "development", "Environment (development, staging, production)")
	serverAddCmd.Flags().String("provider", "", "Hosting provider")
	serverAddCmd.Flags().String("tags", "", "Tags as JSON array [{\"key\":\"x\",\"value\":\"y\"}]")
	serverAddCmd.Flags().String("output", "table", "Output format (table, json)")

	serverListCmd.Flags().String("output", "table", "Output format (table, json)")
	serverListCmd.Flags().String("environment", "", "Filter by environment")
	serverListCmd.Flags().String("tag-key", "", "Filter by tag key")
	serverListCmd.Flags().String("tag-value", "", "Filter by tag value")

	serverInspectCmd.Flags().String("output", "table", "Output format (table, json)")

	serverCheckCmd.Flags().String("output", "table", "Output format (table, json)")

	serverCmd.AddCommand(serverAddCmd)
	serverCmd.AddCommand(serverListCmd)
	serverCmd.AddCommand(serverInspectCmd)
	serverCmd.AddCommand(serverCheckCmd)
}
